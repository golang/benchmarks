// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/pool"

	"github.com/gomodule/redigo/redis"
)

type config struct {
	host        string
	port        int
	seed        int64
	serverBin   string
	dataPath    string
	tmpDir      string
	serverProcs int
	isProfiling bool
	short       bool
}

func (c *config) profilePath(typ driver.ProfileType) string {
	var fname string
	switch typ {
	case driver.ProfileCPU:
		fname = "cpu.prof"
	case driver.ProfileMem:
		fname = "mem.prof"
	case driver.ProfilePerf:
		fname = "perf.data"
	default:
		panic("unsupported profile type " + string(typ))
	}
	return filepath.Join(c.tmpDir, fname)
}

var cliCfg config

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.StringVar(&cliCfg.host, "host", "", "hostname of tile38 server")
	flag.IntVar(&cliCfg.port, "port", 9851, "port for tile38 server")
	flag.Int64Var(&cliCfg.seed, "seed", 0, "seed for PRNG")
	flag.StringVar(&cliCfg.serverBin, "server", "", "path to tile38 server binary")
	flag.StringVar(&cliCfg.dataPath, "data", "", "path to tile38 server data")
	flag.StringVar(&cliCfg.tmpDir, "tmp", "", "path to temporary directory")
	flag.BoolVar(&cliCfg.short, "short", false, "whether to run a short version of this benchmark")

	// Grab the number of procs we have and give ourselves only 1/4 of those.
	procs := runtime.GOMAXPROCS(-1)
	clientProcs := procs / 4
	if clientProcs == 0 {
		clientProcs = 1
	}
	serverProcs := procs - clientProcs
	if serverProcs == 0 {
		serverProcs = 1
	}
	runtime.GOMAXPROCS(clientProcs)
	cliCfg.serverProcs = serverProcs
}

type requestFunc func(redis.Conn, float64, float64) error

func doWithinCircle(c redis.Conn, lat, lon float64) error {
	_, err := c.Do("WITHIN", "key:bench", "COUNT", "CIRCLE",
		strconv.FormatFloat(lat, 'f', 5, 64),
		strconv.FormatFloat(lon, 'f', 5, 64),
		"100000",
	)
	return err
}

func doIntersectsCircle(c redis.Conn, lat, lon float64) error {
	_, err := c.Do("INTERSECTS", "key:bench", "COUNT", "CIRCLE",
		strconv.FormatFloat(lat, 'f', 5, 64),
		strconv.FormatFloat(lon, 'f', 5, 64),
		"100000",
	)
	return err
}

func doNearby(c redis.Conn, lat, lon float64) error {
	_, err := c.Do("NEARBY", "key:bench", "LIMIT", "100", "COUNT", "POINT",
		strconv.FormatFloat(lat, 'f', 5, 64),
		strconv.FormatFloat(lon, 'f', 5, 64),
	)
	return err
}

func randPoint() (float64, float64) {
	return rand.Float64()*180 - 90, rand.Float64()*360 - 180
}

type worker struct {
	redis.Conn
	runner    requestFunc
	iterCount *int64 // Accessed atomically.
	lat       []time.Duration
}

func newWorker(host string, port int, req requestFunc, iterCount *int64) (*worker, error) {
	conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, err
	}
	return &worker{
		Conn:      conn,
		runner:    req,
		iterCount: iterCount,
		lat:       make([]time.Duration, 0, 100000),
	}, nil
}

func (w *worker) Run(_ context.Context) error {
	lat, lon := randPoint()
	start := time.Now()
	if err := w.runner(w.Conn, lat, lon); err != nil {
		return err
	}
	dur := time.Now().Sub(start)
	w.lat = append(w.lat, dur)
	if atomic.AddInt64(w.iterCount, -1) < 0 {
		return pool.Done
	}
	return nil
}

func (w *worker) Close() error {
	return w.Conn.Close()
}

type durSlice []time.Duration

func (d durSlice) Len() int           { return len(d) }
func (d durSlice) Less(i, j int) bool { return d[i] < d[j] }
func (d durSlice) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

type benchmark struct {
	sname  string
	runner requestFunc
}

func (b *benchmark) name() string {
	return fmt.Sprintf("Tile38%sRequest", b.sname)
}

func (b *benchmark) run(d *driver.B, host string, port, clients int, iters int) error {
	workers := make([]pool.Worker, 0, clients)
	iterCount := int64(iters) // Shared atomic variable.
	for i := 0; i < clients; i++ {
		w, err := newWorker(host, port, b.runner, &iterCount)
		if err != nil {
			return err
		}
		workers = append(workers, w)
	}
	p := pool.New(context.Background(), workers)

	d.ResetTimer()
	if err := p.Run(); err != nil {
		return err
	}
	d.StopTimer()

	// Test is done, bring all latency measurements together.
	latencies := make([]time.Duration, 0, len(workers)*100000)
	for _, w := range workers {
		latencies = append(latencies, w.(*worker).lat...)
	}
	sort.Sort(durSlice(latencies))

	// Sort and report percentiles.
	p50 := latencies[len(latencies)*50/100]
	p90 := latencies[len(latencies)*90/100]
	p99 := latencies[len(latencies)*99/100]
	d.Report("p50-latency-ns", uint64(p50))
	d.Report("p90-latency-ns", uint64(p90))
	d.Report("p99-latency-ns", uint64(p99))

	// Report throughput.
	lengthS := float64(d.Elapsed()) / float64(time.Second)
	reqsPerSec := float64(len(latencies)) / lengthS
	d.Report("ops/s", uint64(reqsPerSec))

	// Report the average request latency.
	d.Ops(len(latencies))
	d.Report(driver.StatTime, uint64((int(d.Elapsed())*clients)/len(latencies)))
	return nil
}

var benchmarks = []benchmark{
	{"WithinCircle100km", doWithinCircle},
	{"IntersectsCircle100km", doIntersectsCircle},
	{"KNearestLimit100", doNearby},
}

func launchServer(cfg *config, out io.Writer) (*exec.Cmd, error) {
	// Set up arguments.
	srvArgs := []string{
		"-d", cfg.dataPath,
		"-h", "127.0.0.1",
		"-p", "9851",
		"-threads", strconv.Itoa(cfg.serverProcs),
	}
	for _, typ := range []driver.ProfileType{driver.ProfileCPU, driver.ProfileMem} {
		if driver.ProfilingEnabled(typ) {
			srvArgs = append(srvArgs, "-"+string(typ)+"profile", cfg.profilePath(typ))
		}
	}

	// Start up the server.
	srvCmd := exec.Command(cfg.serverBin, srvArgs...)
	srvCmd.Env = append(os.Environ(),
		fmt.Sprintf("GOMAXPROCS=%d", cfg.serverProcs),
	)
	srvCmd.Stdout = out
	srvCmd.Stderr = out
	if err := srvCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %v", err)
	}

	testConnection := func() error {
		c, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", cfg.host, cfg.port))
		if err != nil {
			return err
		}
		defer c.Close()

		// Starting in 1.26.1, Tile38 accepts connections before
		// loading data, allowing commands OUTPUT, PING, and ECHO, but
		// returning errors for all other commands until data finishes
		// loading.
		//
		// We test a command that requires loaded data to ensure the
		// server is truly ready.
		_, err = c.Do("SERVER")
		return err
	}

	// Poll until the server is ready to serve, up to 120 seconds.
	var err error
	start := time.Now()
	for time.Now().Sub(start) < 120*time.Second {
		err = testConnection()
		if err == nil {
			return srvCmd, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timeout trying to connect to server: %v", err)
}

func runOne(bench benchmark, cfg *config) (err error) {
	var buf bytes.Buffer

	// Launch the server.
	srvCmd, err := launchServer(cfg, &buf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting server: %v\n%s\n", err, &buf)
		os.Exit(1)
	}

	// Clean up the server process after we're done.
	defer func() {
		if r := srvCmd.Process.Signal(os.Interrupt); r != nil {
			if err == nil {
				err = r
			} else {
				fmt.Fprintf(os.Stderr, "failed to shut down server: %v\n", r)
			}
			return
		}
		if _, r := srvCmd.Process.Wait(); r != nil {
			if err == nil {
				err = r
			} else if r != nil {
				fmt.Fprintf(os.Stderr, "failed to wait for server to exit: %v\n", r)
			}
			return
		}
		if buf.Len() != 0 {
			fmt.Fprintln(os.Stderr, "=== Server stdout+stderr ===")
			fmt.Fprintln(os.Stderr, buf.String())
		}

		// Now that the server is done, the profile should be complete and flushed.
		// Copy it over.
		for _, typ := range []driver.ProfileType{driver.ProfileCPU, driver.ProfileMem} {
			if driver.ProfilingEnabled(typ) {
				p, r := driver.ReadProfile(cfg.profilePath(typ))
				if r != nil {
					err = r
					return
				}
				if r := driver.WriteProfile(p, typ, bench.name()); r != nil {
					err = r
					return
				}
			}
		}
	}()

	rand.Seed(cfg.seed)
	opts := []driver.RunOption{
		driver.DoPeakRSS(true),
		driver.DoPeakVM(true),
		driver.DoDefaultAvgRSS(),
		driver.DoCoreDump(true),
		driver.BenchmarkPID(srvCmd.Process.Pid),
		driver.DoPerf(true),
	}
	iters := 20 * 50000
	if cfg.short {
		iters = 1000
	}
	return driver.RunBenchmark(bench.name(), func(d *driver.B) error {
		return bench.run(d, cfg.host, cfg.port, cfg.serverProcs, iters)
	}, opts...)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected args\n")
		os.Exit(1)
	}
	for _, typ := range driver.ProfileTypes {
		cliCfg.isProfiling = cliCfg.isProfiling || driver.ProfilingEnabled(typ)
	}
	benchmarks := benchmarks
	if cliCfg.short {
		benchmarks = benchmarks[:1]
	}
	for _, bench := range benchmarks {
		if err := runOne(bench, &cliCfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
}
