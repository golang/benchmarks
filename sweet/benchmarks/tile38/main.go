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
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/pool"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/server"
	"golang.org/x/benchmarks/sweet/common/diagnostics"

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
	gomaxprocs  int
	short       bool
}

var cliCfg config

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.StringVar(&cliCfg.host, "host", "127.0.0.1", "hostname of tile38 server")
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
	cliCfg.gomaxprocs = procs
}

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

type requestFunc func(redis.Conn, float64, float64) error

var requestFuncs = []requestFunc{
	doWithinCircle,
	doIntersectsCircle,
	doNearby,
}

func randPoint() (float64, float64) {
	return rand.Float64()*180 - 90, rand.Float64()*360 - 180
}

type worker struct {
	redis.Conn
	iterCount *int64 // Accessed atomically.
	lat       []time.Duration
}

func newWorker(host string, port int, iterCount *int64) (*worker, error) {
	conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, err
	}
	return &worker{
		Conn:      conn,
		iterCount: iterCount,
		lat:       make([]time.Duration, 0, 100000),
	}, nil
}

func (w *worker) Run(_ context.Context) error {
	count := atomic.AddInt64(w.iterCount, -1)
	if count < 0 {
		return pool.Done
	}
	lat, lon := randPoint()
	start := time.Now()
	if err := requestFuncs[count%3](w.Conn, lat, lon); err != nil {
		return err
	}
	dur := time.Now().Sub(start)
	w.lat = append(w.lat, dur)
	return nil
}

func (w *worker) Close() error {
	return w.Conn.Close()
}

type durSlice []time.Duration

func (d durSlice) Len() int           { return len(d) }
func (d durSlice) Less(i, j int) bool { return d[i] < d[j] }
func (d durSlice) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

func runBenchmark(d *driver.B, host string, port, clients int, iters int) error {
	workers := make([]pool.Worker, 0, clients)
	iterCount := int64(iters) // Shared atomic variable.
	for i := 0; i < clients; i++ {
		w, err := newWorker(host, port, &iterCount)
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

func launchServer(cfg *config, diag *driver.Diagnostics, out io.Writer) (*exec.Cmd, []func(), error) {
	// Set up arguments.
	srvArgs := []string{
		"-d", cfg.dataPath,
		"-h", cfg.host,
		"-p", strconv.Itoa(cfg.port),
		"-pprofport", strconv.Itoa(pprofPort),
	}

	// Set up diagnostics that the server can gather on its own
	var postExit []func()
	for _, typ := range []diagnostics.Type{diagnostics.CPUProfile, diagnostics.MemProfile} {
		if df, err := diag.Create(typ); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s diagnostics: %s\n", typ, err)
		} else if df != nil {
			srvArgs = append(srvArgs, "-"+string(typ), df.Name())
			// We don't need the file, so close it immediately.
			df.Close()
			postExit = append(postExit, df.Commit)
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
		return nil, nil, fmt.Errorf("failed to start server: %v", err)
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
			return srvCmd, postExit, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, nil, fmt.Errorf("timeout trying to connect to server: %v", err)
}

const pprofPort = 12345

const benchName = "Tile38QueryLoad"

func run(cfg *config) (err error) {
	var buf bytes.Buffer

	diag := driver.NewDiagnostics("tile38")
	defer diag.Commit(nil)

	// Launch the server.
	srvCmd, postExit, err := launchServer(cfg, diag, &buf)
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

		// Run post-exit functions
		for _, fn := range postExit {
			fn()
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
		driver.WithGOMAXPROCS(cfg.gomaxprocs),
	}
	iters := 40 * 50000
	if cfg.short {
		iters = 100
	}
	return driver.RunBenchmark(benchName, func(d *driver.B) error {
		// Collect a trace only during the run. (Also, Tile38 doesn't have a
		// flag to collect its own trace, so we couldn't collect it another way
		// anyway.)
		stop := server.FetchDiagnostic(fmt.Sprintf("%s:%d", cfg.host, pprofPort), diag, diagnostics.Trace, benchName)
		defer stop()

		return runBenchmark(d, cfg.host, cfg.port, cfg.serverProcs, iters)
	}, opts...)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected args\n")
		os.Exit(1)
	}
	if err := run(&cliCfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
