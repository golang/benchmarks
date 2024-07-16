// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !wasm

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/server"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

const (
	// Arbitrarily chosen to match the cockroachdb default.
	basePort = 26257
	// The percentage of memory to allocate to the pebble cache.
	cacheSize = "0.25"
)

type config struct {
	host           string
	cockroachdbBin string
	tmpDir         string
	benchName      string
	isProfiling    bool
	short          bool
	procsPerInst   int
	bench          *benchmark
}

var cliCfg config

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.StringVar(&cliCfg.host, "host", "localhost", "hostname of cockroachdb server")
	flag.StringVar(&cliCfg.cockroachdbBin, "cockroachdb-bin", "", "path to cockroachdb binary")
	flag.StringVar(&cliCfg.tmpDir, "tmp", "", "path to temporary directory")
	flag.StringVar(&cliCfg.benchName, "bench", "", "name of the benchmark to run")
	flag.BoolVar(&cliCfg.short, "short", false, "whether to run a short version of this benchmark")
}

type cockroachdbInstance struct {
	name     string
	sqlPort  int // Used for intra-cluster communication.
	httpPort int // Used to scrape for metrics.
	cmd      *exec.Cmd
	output   bytes.Buffer
}

func clusterAddresses(instances []*cockroachdbInstance) string {
	var s []string
	for _, inst := range instances {
		s = append(s, inst.sqlAddr())
	}
	return strings.Join(s, ",")
}

func launchSingleNodeCluster(cfg *config) ([]*cockroachdbInstance, error) {
	var instances []*cockroachdbInstance
	instances = append(instances, &cockroachdbInstance{
		name:     "roach-node",
		sqlPort:  basePort,
		httpPort: basePort + 1,
	})
	inst := instances[0]

	// `cockroach start-single-node` handles both creation of the node
	// and initialization.
	inst.cmd = exec.Command(cfg.cockroachdbBin,
		"start-single-node",
		"--insecure",
		"--listen-addr", inst.sqlAddr(),
		"--http-addr", inst.httpAddr(),
		"--cache", cacheSize,
		"--store", fmt.Sprintf("%s/%s", cfg.tmpDir, inst.name),
		"--logtostderr",
	)
	inst.cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOMAXPROCS=%d", cfg.procsPerInst),
	)
	inst.cmd.Stdout = &inst.output
	inst.cmd.Stderr = &inst.output
	if err := inst.cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start instance %q: %v", inst.name, err)
	}
	return instances, nil
}

func launchCockroachCluster(cfg *config) ([]*cockroachdbInstance, error) {
	if cfg.bench.nodeCount == 1 {
		// Use `cockroach start-single-node` instead for single node clusters.
		return launchSingleNodeCluster(cfg)
	}
	var instances []*cockroachdbInstance
	for i := 0; i < cfg.bench.nodeCount; i++ {
		instances = append(instances, &cockroachdbInstance{
			name:     fmt.Sprintf("roach-node-%d", i+1),
			sqlPort:  basePort + 2*i,
			httpPort: basePort + 2*i + 1,
		})
	}

	// Start the instances with `cockroach start`.
	for n, inst := range instances {
		allOtherInstances := append(instances[:n:n], instances[n+1:]...)
		join := fmt.Sprintf("--join=%s", clusterAddresses(allOtherInstances))

		inst.cmd = exec.Command(cfg.cockroachdbBin,
			"start",
			"--insecure",
			"--listen-addr", inst.sqlAddr(),
			"--http-addr", inst.httpAddr(),
			"--cache", cacheSize,
			"--store", fmt.Sprintf("%s/%s", cfg.tmpDir, inst.name),
			"--logtostderr",
			join,
		)
		inst.cmd.Env = append(os.Environ(),
			fmt.Sprintf("GOMAXPROCS=%d", cfg.procsPerInst),
		)
		inst.cmd.Stdout = &inst.output
		inst.cmd.Stderr = &inst.output
		if err := inst.cmd.Start(); err != nil {
			return nil, fmt.Errorf("failed to start instance %q: %v", inst.name, err)
		}
	}

	// Initialize the cluster with `cockroach init`.
	inst1 := instances[0]
	initCmd := exec.Command(cfg.cockroachdbBin,
		"init",
		"--insecure",
		fmt.Sprintf("--host=%s", cfg.host),
		fmt.Sprintf("--port=%d", inst1.sqlPort),
	)
	initCmd.Env = append(os.Environ(),
		fmt.Sprintf("GOMAXPROCS=%d", cfg.procsPerInst),
	)
	initCmd.Stdout = &inst1.output
	initCmd.Stderr = &inst1.output
	if err := initCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to init instance %q: %v", inst1.name, err)
	}
	return instances, nil
}

// waitForCluster pings nodes in the cluster until one responds, or
// we time out. We only care to wait for one node to respond as the
// workload will work as long as it can connect to one node initially.
// The --ramp flag will take care of startup noise.
func waitForCluster(instances []*cockroachdbInstance, cfg *config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, inst := range instances {
		inst := inst
		go func(ctx context.Context) {
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					// The node will almost certainly not be ready right away, so wait
					// 5 seconds first and between pings. 5 seconds was chosen through
					// trial and error as a time that nodes are *usually* ready by.
					if err := inst.ping(cfg); err == nil {
						cancel()
						return
					}
				}
			}
		}(ctx)
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Minute):
		return errors.New("benchmark timed out waiting for cluster to be ready")
	}

	return nil
}

func (i *cockroachdbInstance) setClusterSettings(cfg *config) error {
	settings := []string{
		// Disable admission control.
		// Cockroach normally patches the go runtime to track cpu nanos per goroutine,
		// which is used by admission control. However, these benchmarks are run without
		// said observability change, and it is unsupported + untested if admission control
		// works in this case. It might be fine, but to be safe, we disable it here.
		"admission.kv.enabled = false",
		"admission.sql_kv_response.enabled = false",
		"admission.sql_sql_response.enabled = false",
	}

	// Multi-line cluster setting changes aren't allowed.
	for _, setting := range settings {
		cmd := exec.Command(cfg.cockroachdbBin,
			"sql",
			"--insecure",
			fmt.Sprintf("--host=%s", cfg.host),
			fmt.Sprintf("--port=%d", i.sqlPort),
			"--execute", fmt.Sprintf("SET CLUSTER SETTING %s;", setting),
		)
		cmd.Stdout = &i.output
		cmd.Stderr = &i.output
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}

func (i *cockroachdbInstance) ping(cfg *config) error {
	// Ping the node to see if it is live. The actual command of
	// `node status` is a bit arbitrary, if it responds at all
	// we know the node is live. Generally it is used to see if
	// *other* nodes are live.
	cmd := exec.Command(cfg.cockroachdbBin,
		"node",
		"status",
		"--insecure",
		fmt.Sprintf("--host=%s", cfg.host),
		fmt.Sprintf("--port=%d", i.sqlPort),
	)
	cmd.Stdout = &i.output
	cmd.Stderr = &i.output
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (i *cockroachdbInstance) sqlAddr() string {
	return fmt.Sprintf("%s:%d", cliCfg.host, i.sqlPort)
}

func (i *cockroachdbInstance) httpAddr() string {
	return fmt.Sprintf("%s:%d", cliCfg.host, i.httpPort)
}

func (i *cockroachdbInstance) shutdown() (killed bool, err error) {
	// Only attempt to shut down the process if we got to a point
	// that a command was constructed and started.
	if i.cmd == nil || i.cmd.Process == nil {
		return false, nil
	}
	// Send SIGTERM and wait instead of just killing as sending SIGKILL
	// bypasses node shutdown logic and will leave the node in a bad state.
	// Normally not an issue unless you want to restart the cluster i.e.
	// to poke around and see what's wrong.
	if err := i.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return false, err
	}
	done := make(chan struct{})
	go func() {
		if _, err := i.cmd.Process.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to wait for instance %s: %v\n", i.name, err)
		}
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Minute):
		// If it takes a full minute to shut down, just kill the instance
		// and report that we did it. We *probably* won't need it again.
		if err := i.cmd.Process.Signal(syscall.SIGKILL); err != nil {
			return false, fmt.Errorf("failed to send SIGKILL to instance %s: %v", i.name, err)
		}
		killed = true

		// Wait again -- this time it should happen.
		log.Println("sent kill signal to", i.name, "and waiting for exit")
		<-done
	}
	return killed, nil
}

func (i *cockroachdbInstance) kill() error {
	if i.cmd == nil || i.cmd.Process == nil {
		// Nothing to kill.
		return nil
	}
	if err := i.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	if _, err := i.cmd.Process.Wait(); err != nil {
		return fmt.Errorf("failed to wait for instance %s: %v", i.name, err)
	}
	return nil
}

type benchmark struct {
	name        string
	reportName  string
	workload    string
	nodeCount   int
	args        []string
	longArgs    []string // if !config.short
	shortArgs   []string // if config.short
	metricTypes []string
	timeout     time.Duration
}

const (
	readMetric  = "read"
	writeMetric = "write"
)

func kvBenchmark(readPercent int, nodeCount int) benchmark {
	metricTypes := []string{writeMetric}
	if readPercent > 0 {
		metricTypes = append(metricTypes, readMetric)
	}

	return benchmark{
		name:        fmt.Sprintf("kv%d/nodes=%d", readPercent, nodeCount),
		reportName:  fmt.Sprintf("CockroachDBkv%d/nodes=%d", readPercent, nodeCount),
		workload:    "kv",
		nodeCount:   1,
		metricTypes: metricTypes,
		// Very generous timeout, we don't expect to ever hit this, but just in case.
		timeout: 5 * time.Minute,
		args: []string{
			"workload", "run", "kv",
			fmt.Sprintf("--read-percent=%d", readPercent),
			"--min-block-bytes=1024",
			"--max-block-bytes=1024",
			"--concurrency=10000",
			"--max-rate=30000",
			//Pre-splitting and scattering the ranges should help stabilize results.
			"--scatter",
			"--splits=5",
		},
		longArgs: []string{
			"--ramp=15s",
			"--duration=1m",
		},
		// Chosen through trial and error as the shortest time that doesn't
		// give extremely fluctuating results.
		shortArgs: []string{
			"--ramp=5s",
			"--duration=30s",
		},
	}
}

var benchmarks = []benchmark{
	kvBenchmark(0 /* readPercent */, 1 /* nodeCount */),
	kvBenchmark(0 /* readPercent */, 3 /* nodeCount */),
	kvBenchmark(50 /* readPercent */, 1 /* nodeCount */),
	kvBenchmark(50 /* readPercent */, 3 /* nodeCount */),
	kvBenchmark(95 /* readPercent */, 1 /* nodeCount */),
	kvBenchmark(95 /* readPercent */, 3 /* nodeCount */),
}

func runBenchmark(b *driver.B, cfg *config, instances []*cockroachdbInstance) (err error) {
	var pgurls []string
	for _, inst := range instances {
		host := inst.sqlAddr()
		pgurls = append(pgurls, fmt.Sprintf(`postgres://root@%s?sslmode=disable`, host))
	}
	// Load in the schema needed for the workload via `workload init`
	log.Println("loading the schema")
	initArgs := []string{"workload", "init", cfg.bench.workload}
	initArgs = append(initArgs, pgurls...)
	initCmd := exec.Command(cfg.cockroachdbBin, initArgs...)
	var stdout, stderr bytes.Buffer
	initCmd.Stdout = &stdout
	initCmd.Stderr = &stderr
	if err = initCmd.Run(); err != nil {
		return err
	}

	log.Println("sleeping")

	// If we try and start the workload right after loading in the schema
	// it will spam us with database does not exist errors. We could repeatedly
	// retry until the database exists by parsing the output, or we can just
	// wait 5 seconds.
	time.Sleep(5 * time.Second)

	args := cfg.bench.args
	if cfg.short {
		args = append(args, cfg.bench.shortArgs...)
	} else {
		args = append(args, cfg.bench.longArgs...)
	}
	args = append(args, pgurls...)

	log.Println("running benchmark timeout")
	cmd := exec.Command(cfg.cockroachdbBin, args...)
	fmt.Fprintln(os.Stderr, cmd.String())

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), fmt.Sprintf("GOMAXPROCS=%d", cfg.procsPerInst))

	defer func() {
		if err != nil && stderr.Len() != 0 {
			fmt.Fprintln(os.Stderr, "=== Benchmarking tool stderr ===")
			fmt.Fprintln(os.Stderr, stderr.String())
		}
	}()

	finished := make(chan bool, 1)
	var benchmarkErr error
	go func() {
		b.ResetTimer()
		if err = cmd.Run(); err != nil {
			benchmarkErr = err
		}
		b.StopTimer()
		finished <- true
	}()

	select {
	case <-finished:
	case <-time.After(cfg.bench.timeout):
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("error killing benchmark process, benchmark timed out: %w", err)
		}
		return errors.New("benchmark timed out")
	}

	if benchmarkErr != nil {
		return benchmarkErr
	}

	return reportFromBenchmarkOutput(b, cfg, stdout.String())
}

func reportFromBenchmarkOutput(b *driver.B, cfg *config, output string) (err error) {
	defer func() {
		if err != nil {
			fmt.Fprintln(os.Stderr, "=== Benchmarking tool output ===")
			fmt.Fprintln(os.Stderr, output)
		}
	}()

	for _, metricType := range cfg.bench.metricTypes {
		err = getAndReportMetrics(b, metricType, output)
		if err != nil {
			return err
		}
	}

	if err != nil {
		return err
	}
	return nil
}

type benchmarkMetrics struct {
	totalOps       uint64
	opsPerSecond   uint64
	averageLatency uint64
	p50Latency     uint64
	p95Latency     uint64
	p99Latency     uint64
	p100Latency    uint64
}

func getAndReportMetrics(b *driver.B, metricType string, output string) error {
	metrics, err := getMetrics(metricType, output)
	if err != nil {
		return err
	}
	reportMetrics(b, metricType, metrics)
	return nil
}

func getMetrics(metricType string, output string) (benchmarkMetrics, error) {
	re := regexp.MustCompile(fmt.Sprintf(`.*(__total)\n.*%s`, metricType))
	match := re.FindString(output)
	if len(match) == 0 {
		return benchmarkMetrics{}, fmt.Errorf("failed to find %s metrics in output", metricType)
	}
	match = strings.Split(match, "\n")[1]
	fields := strings.Fields(match)

	stringToUint64 := func(field string) (uint64, error) {
		number, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return 0, fmt.Errorf("error parsing metrics to uint64: %w", err)
		}
		return uint64(number), nil
	}

	uint64Fields := make([]uint64, len(fields[2:])-1)
	for i := range uint64Fields {
		var err error
		uint64Fields[i], err = stringToUint64(fields[2+i])
		if err != nil {
			return benchmarkMetrics{}, err
		}
	}

	metrics := benchmarkMetrics{
		totalOps:       uint64Fields[0],
		opsPerSecond:   uint64Fields[1],
		averageLatency: uint64Fields[2] * 1e6,
		p50Latency:     uint64Fields[3] * 1e6,
		p95Latency:     uint64Fields[4] * 1e6,
		p99Latency:     uint64Fields[5] * 1e6,
		p100Latency:    uint64Fields[6] * 1e6,
	}
	return metrics, nil
}

func reportMetrics(b *driver.B, metricType string, metrics benchmarkMetrics) {
	b.Report(fmt.Sprintf("%s-ops/sec", metricType), metrics.opsPerSecond)
	b.Report(fmt.Sprintf("%s-ops", metricType), metrics.totalOps)
	b.Report(fmt.Sprintf("%s-ns/op", metricType), metrics.averageLatency)
	b.Report(fmt.Sprintf("%s-p50-latency-ns", metricType), metrics.p50Latency)
	b.Report(fmt.Sprintf("%s-p95-latency-ns", metricType), metrics.p95Latency)
	b.Report(fmt.Sprintf("%s-p99-latency-ns", metricType), metrics.p99Latency)
	b.Report(fmt.Sprintf("%s-p100-latency-ns", metricType), metrics.p100Latency)
}

func run(cfg *config) (err error) {
	log.Println("launching cluster")
	var instances []*cockroachdbInstance
	// Launch the server.
	instances, err = launchCockroachCluster(cfg)

	if err != nil {
		return fmt.Errorf("starting cluster: %v\n", err)
	}

	// Clean up the cluster after we're done.
	defer func() {
		log.Println("shutting down cluster")

		// We only need send a shutdown signal to one instance, attempting to
		// send it again will cause it to hang.
		inst := instances[0]
		killed, r := inst.shutdown()
		if r != nil {
			if err == nil {
				err = r
			} else {
				fmt.Fprintf(os.Stderr, "failed to shutdown %s: %v", inst.name, r)
			}
		}
		if killed {
			log.Println("killed instance", inst.name, "and killing others")

			// If we ended up killing an instance, try to kill the other instances too.
			for _, inst := range instances[1:] {
				if err := inst.kill(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to kill %s: %v", inst.name, err)
				}
			}
		}
		if inst.output.Len() != 0 {
			fmt.Fprintf(os.Stderr, "=== Instance %q stdout+stderr ===\n", inst.name)
			fmt.Fprintln(os.Stderr, inst.output.String())
		}
	}()

	log.Println("waiting for cluster")
	if err = waitForCluster(instances, cfg); err != nil {
		return err
	}

	log.Println("setting cluster settings")
	if err = instances[0].setClusterSettings(cfg); err != nil {
		return err
	}

	opts := []driver.RunOption{
		driver.DoPeakRSS(true),
		driver.DoPeakVM(true),
		driver.DoDefaultAvgRSS(),
		driver.DoCoreDump(true),
		driver.BenchmarkPID(instances[0].cmd.Process.Pid),
		driver.DoPerf(true),
	}
	return driver.RunBenchmark(cfg.bench.reportName, func(d *driver.B) error {
		// Set up diagnostics.
		var finishers []func() uint64
		if driver.DiagnosticEnabled(diagnostics.CPUProfile) {
			for _, inst := range instances {
				finishers = append(finishers, server.PollDiagnostic(
					inst.httpAddr(),
					cfg.tmpDir,
					cfg.bench.reportName,
					diagnostics.CPUProfile,
				))
			}
		}
		if driver.DiagnosticEnabled(diagnostics.Trace) {
			var sum atomic.Uint64
			for _, inst := range instances {
				stopTrace := server.PollDiagnostic(
					inst.httpAddr(),
					cfg.tmpDir,
					cfg.bench.reportName,
					diagnostics.Trace,
				)
				finishers = append(finishers, func() uint64 {
					n := stopTrace()
					sum.Add(n)
					return n
				})
			}
			defer func() {
				d.Report("trace-bytes", sum.Load())
			}()
		}
		if driver.DiagnosticEnabled(diagnostics.MemProfile) {
			for _, inst := range instances {
				inst := inst
				finishers = append(finishers, func() uint64 {
					n, err := server.CollectDiagnostic(
						inst.httpAddr(),
						cfg.tmpDir,
						cfg.bench.reportName,
						diagnostics.MemProfile,
					)
					if err != nil {
						fmt.Fprintf(os.Stderr, "failed to read memprofile: %v", err)
					}
					return uint64(n)
				})
			}
		}
		if len(finishers) != 0 {
			// Finish all the diagnostic collections in concurrently. Otherwise we could be waiting a while.
			defer func() {
				log.Println("running finishers")
				var wg sync.WaitGroup
				for _, finish := range finishers {
					finish := finish
					wg.Add(1)
					go func() {
						defer wg.Done()
						finish()
					}()
				}
				wg.Wait()
			}()
		}
		// Actually run the benchmark.
		log.Println("running benchmark")
		return runBenchmark(d, cfg, instances)
	}, opts...)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected args\n")
		os.Exit(1)
	}
	for _, typ := range diagnostics.Types() {
		cliCfg.isProfiling = cliCfg.isProfiling || driver.DiagnosticEnabled(typ)
	}
	for i := range benchmarks {
		if benchmarks[i].name == cliCfg.benchName {
			cliCfg.bench = &benchmarks[i]
			break
		}
	}
	if cliCfg.bench == nil {
		fmt.Fprintf(os.Stderr, "error: unknown benchmark %q\n", cliCfg.benchName)
		os.Exit(1)
	}

	// We're going to launch a bunch of cockroachdb instances. Distribute
	// GOMAXPROCS between those and ourselves equally.
	procs := runtime.GOMAXPROCS(-1)
	procsPerInst := procs / (cliCfg.bench.nodeCount + 1)
	if procsPerInst == 0 {
		procsPerInst = 1
	}
	runtime.GOMAXPROCS(procsPerInst)
	cliCfg.procsPerInst = procsPerInst

	if err := run(&cliCfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
