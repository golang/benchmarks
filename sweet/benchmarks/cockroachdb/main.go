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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/par"
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
	output   syncBuffer
}

// syncBuffer is a bytes.Buffer that is safe for concurrent use.
//
// An instance's output is written by more than one goroutine at a time. os/exec
// starts a goroutine per command to copy that command's output into the writer
// it was given, and the node's long-running command shares its writer with the
// short-lived init, sql and node status commands aimed at the same node. The
// deferred report in run then reads the buffer back. bytes.Buffer is not safe
// for any of that.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
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
		"--store", filepath.Join(cfg.tmpDir, inst.name),
		"--log-dir", filepath.Join(cfg.tmpDir, inst.name+"-log"),
	)
	inst.cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOMAXPROCS=%d", cfg.procsPerInst),
	)
	inst.cmd.Stdout = &inst.output
	inst.cmd.Stderr = &inst.output
	if err := inst.cmd.Start(); err != nil {
		return instances, fmt.Errorf("failed to start instance %q: %v", inst.name, err)
	}
	return instances, nil
}

// launchCockroachCluster starts the cluster. It returns the instances it
// created even when it fails, because the caller has to stop the ones that did
// start.
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
			"--store", filepath.Join(cfg.tmpDir, inst.name),
			"--log-dir", filepath.Join(cfg.tmpDir, inst.name+"-log"),
			join,
		)
		inst.cmd.Env = append(os.Environ(),
			fmt.Sprintf("GOMAXPROCS=%d", cfg.procsPerInst),
		)
		inst.cmd.Stdout = &inst.output
		inst.cmd.Stderr = &inst.output
		if err := inst.cmd.Start(); err != nil {
			return instances, fmt.Errorf("failed to start instance %q: %v", inst.name, err)
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
		return instances, fmt.Errorf("failed to init instance %q: %v", inst1.name, err)
	}
	return instances, nil
}

// waitForCluster pings every node in the cluster until all of them respond, or
// we time out. We wait for all of them rather than the first, because even if
// the workload can start before all nodes are ready, that's not true for
// collecting diagnostic data.
func waitForCluster(instances []*cockroachdbInstance, cfg *config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for _, inst := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					// The node will almost certainly not be ready right away, so wait
					// 5 seconds first and between pings. 5 seconds was chosen through
					// trial and error as a time that nodes are *usually* ready by.
					if err := inst.ping(cfg); err == nil {
						return
					}
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		done <- struct{}{}
	}()

	select {
	case <-done:
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
	// Check to see if profiling works. We don't care what the data is, we just want to
	// make sure that it doesn't fail. We've unfortunately seen this fail before.
	// See #56958.
	endpoint := fmt.Sprintf("http://%s:%d/%s", cfg.host, i.httpPort, diagnostics.MemProfile.HTTPEndpoint())
	resp, err := http.Get(endpoint)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (i *cockroachdbInstance) sqlAddr() string {
	return fmt.Sprintf("%s:%d", cliCfg.host, i.sqlPort)
}

func (i *cockroachdbInstance) httpAddr() string {
	return fmt.Sprintf("%s:%d", cliCfg.host, i.httpPort)
}

// clusterStopGrace is how long the cluster as a whole gets to exit on SIGTERM
// before the nodes that have not exited are killed.
const clusterStopGrace = 15 * time.Second

// stopCluster stops every node in the cluster, and reports how many were
// signalled and how many of those had to be killed.
func stopCluster(instances []*cockroachdbInstance) (killed, total int, err error) {
	var signalled []*cockroachdbInstance
	for _, inst := range instances {
		// Only attempt to shut down the process if we got to a point
		// that a command was constructed and started.
		if inst.cmd == nil || inst.cmd.Process == nil {
			continue
		}
		// Send SIGTERM and wait instead of just killing as sending SIGKILL
		// bypasses node shutdown logic and will leave the node in a bad state.
		// Normally not an issue unless you want to restart the cluster i.e.
		// to poke around and see what's wrong.
		if serr := inst.cmd.Process.Signal(syscall.SIGTERM); serr != nil {
			if err == nil {
				err = fmt.Errorf("failed to signal instance %s: %v", inst.name, serr)
			}
			continue
		}
		signalled = append(signalled, inst)
	}

	// Wait for every node concurrently, against one deadline.
	deadline := time.Now().Add(clusterStopGrace)
	var wg sync.WaitGroup
	wasKilled := make([]bool, len(signalled))
	waitErrs := make([]error, len(signalled))
	for i, inst := range signalled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wasKilled[i], waitErrs[i] = inst.waitOrKill(time.Until(deadline))
		}()
	}
	wg.Wait()
	for i := range signalled {
		if waitErrs[i] != nil && err == nil {
			err = waitErrs[i]
		}
		if wasKilled[i] {
			killed++
		}
	}
	return killed, len(signalled), err
}

// waitOrKill waits for the instance to exit, killing it if that takes longer
// than timeout. It reports whether it had to kill it.
func (i *cockroachdbInstance) waitOrKill(timeout time.Duration) (killed bool, err error) {
	done := make(chan struct{})
	go func() {
		// Wait on the Cmd rather than the Process. Only Cmd.Wait joins the
		// goroutines os/exec started to copy this node's output into i.output
		// and closes the pipes it created for them. Waiting on the Process
		// alone leaves the copier running while the deferred report in run
		// reads the buffer, and leaks the pipe for the rest of the benchmark.
		//
		// An ExitError is expected here: a node that drained on SIGTERM, or one
		// we killed below, does not exit zero.
		var exitErr *exec.ExitError
		if werr := i.cmd.Wait(); werr != nil && !errors.As(werr, &exitErr) {
			fmt.Fprintf(os.Stderr, "failed to wait for instance %s: %v\n", i.name, werr)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(max(timeout, 0)):
		// The timer and the process exiting race, and select picks at random
		// between two ready cases, so re-check before reporting a kill: an
		// instance that exited just as the window closed did drain, and saying
		// otherwise would make a clean shutdown look like a timeout.
		select {
		case <-done:
			return false, nil
		default:
		}
		// If it takes this long to shut down, kill the instance and report
		// that we did it. The kill still races with the process exiting on
		// its own: ErrProcessDone means it lost, so the node did drain. Wait
		// for the copiers as usual and do not count it.
		if kerr := i.cmd.Process.Signal(syscall.SIGKILL); kerr != nil {
			if errors.Is(kerr, os.ErrProcessDone) {
				<-done
				return false, nil
			}
			return false, fmt.Errorf("failed to send SIGKILL to instance %s: %v", i.name, kerr)
		}
		killed = true

		// Wait again -- this time it should happen.
		log.Println("sent kill signal to", i.name, "and waiting for exit")
		<-done
	}
	return killed, nil
}

type benchmark struct {
	name        string
	reportName  string
	workload    string
	nodeCount   int
	args        []string
	longArgs    []string // if !config.short
	shortArgs   []string // if config.short
	pingArgs    []string
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
			"--concurrency=5000", // Changed from 10000, see go.dev/issue/73474.
			"--max-rate=30000",
			// Pre-splitting and scattering the ranges should help stabilize results.
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
		// Just to ping whether the workload is ready.
		pingArgs: []string{
			"--ramp=0s",
			"--duration=500ms",
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
	if err := initCmd.Run(); err != nil {
		return err
	}

	// Make sure the server is ready to accept work by pinging it with very short
	// benchmark runs. If they fail, we assume that the server isn't ready.
	log.Println("pinging server with benchmark tool")
	pingArgs := cfg.bench.args
	pingArgs = append(pingArgs, cfg.bench.pingArgs...)
	pingArgs = append(pingArgs, pgurls...)
	var pingOutput []byte
	var pingErr error
	pingStart := time.Now()
	for time.Since(pingStart) < 30*time.Second {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pingCmd := exec.CommandContext(ctx, cfg.cockroachdbBin, pingArgs...)
		pingOutput, pingErr = pingCmd.CombinedOutput()
		cancel()
		if pingErr == nil {
			break
		}
	}
	if pingErr != nil {
		return fmt.Errorf("workload failed to become available within timeout: error: %v: output:\n%s", pingErr, pingOutput)
	}

	args := cfg.bench.args
	if cfg.short {
		args = append(args, cfg.bench.shortArgs...)
	} else {
		args = append(args, cfg.bench.longArgs...)
	}
	args = append(args, pgurls...)

	log.Println("running benchmark tool")
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
		if err := cmd.Run(); err != nil {
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

	metrics := make([]benchmarkMetrics, len(cfg.bench.metricTypes))
	for i, metricType := range cfg.bench.metricTypes {
		metrics[i], err = getMetrics(metricType, output)
		if err != nil {
			return err
		}
	}
	totalOps := float64(0)
	for i, metricType := range cfg.bench.metricTypes {
		reportMetrics(b, metricType, metrics[i])
		totalOps += metrics[i].totalOps
	}
	b.Report("ns/op", uint64(metrics[0].totalSec/totalOps*1e9))

	if err != nil {
		return err
	}
	return nil
}

type benchmarkMetrics struct {
	totalSec       float64
	totalOps       float64
	opsPerSecond   float64
	averageLatency float64
	p50Latency     float64
	p95Latency     float64
	p99Latency     float64
	p100Latency    float64
}

func getMetrics(metricType string, output string) (benchmarkMetrics, error) {
	re := regexp.MustCompile(fmt.Sprintf(`.*(__total)\n.*%s`, metricType))
	match := re.FindString(output)
	if len(match) == 0 {
		return benchmarkMetrics{}, fmt.Errorf("failed to find %s metrics in output", metricType)
	}
	match = strings.Split(match, "\n")[1]
	fields := strings.Fields(match)

	stringToFloat64 := func(field string) (float64, error) {
		number, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return 0, fmt.Errorf("error parsing metrics to uint64: %w", err)
		}
		return number, nil
	}

	float64Fields := make([]float64, len(fields[2:])-1)
	for i := range float64Fields {
		var err error
		float64Fields[i], err = stringToFloat64(fields[2+i])
		if err != nil {
			return benchmarkMetrics{}, err
		}
	}
	// Parse benchmark duration.
	dur, err := time.ParseDuration(fields[0])
	if err != nil {
		return benchmarkMetrics{}, err
	}

	metrics := benchmarkMetrics{
		totalSec:       dur.Seconds(),
		totalOps:       float64Fields[0],
		opsPerSecond:   float64Fields[1],
		averageLatency: float64Fields[2] * 1e6,
		p50Latency:     float64Fields[3] * 1e6,
		p95Latency:     float64Fields[4] * 1e6,
		p99Latency:     float64Fields[5] * 1e6,
		p100Latency:    float64Fields[6] * 1e6,
	}
	return metrics, nil
}

func reportMetrics(b *driver.B, metricType string, metrics benchmarkMetrics) {
	b.Report(fmt.Sprintf("%s-ops/sec", metricType), uint64(metrics.opsPerSecond))
	b.Report(fmt.Sprintf("%s-ops", metricType), uint64(metrics.totalOps))
	b.Report(fmt.Sprintf("%s-avg-latency-ns", metricType), uint64(metrics.averageLatency))
	b.Report(fmt.Sprintf("%s-p50-latency-ns", metricType), uint64(metrics.p50Latency))
	b.Report(fmt.Sprintf("%s-p95-latency-ns", metricType), uint64(metrics.p95Latency))
	b.Report(fmt.Sprintf("%s-p99-latency-ns", metricType), uint64(metrics.p99Latency))
	b.Report(fmt.Sprintf("%s-p100-latency-ns", metricType), uint64(metrics.p100Latency))
}

func run(cfg *config) (err error) {
	log.Println("launching cluster")
	// Launch the server. Take the instances even if this fails: it may have
	// started some of them before it gave up, and nothing else stops them.
	instances, launchErr := launchCockroachCluster(cfg)

	// Clean up the cluster after we're done.
	defer func() {
		if len(instances) == 0 {
			return
		}
		log.Println("shutting down cluster")

		killed, total, r := stopCluster(instances)
		if r != nil {
			if err == nil {
				err = r
			} else {
				fmt.Fprintf(os.Stderr, "failed to shut down cluster: %v\n", r)
			}
		}
		// For a multi-node cluster this is the expected path, not a failure.
		if killed > 0 {
			log.Printf("killed %d of %d instances that had not exited %s after SIGTERM",
				killed, total, clusterStopGrace)
		}
		for _, inst := range instances {
			if inst.output.Len() != 0 {
				fmt.Fprintf(os.Stderr, "=== Instance %q stdout+stderr ===\n", inst.name)
				fmt.Fprintln(os.Stderr, inst.output.String())
			}
			stderrLog, logReadErr := os.ReadFile(filepath.Join(cfg.tmpDir, inst.name+"-log", "cockroach-stderr.log"))
			if logReadErr == nil {
				fmt.Fprintf(os.Stderr, "=== Instance %q original stderr ===\n", inst.name)
				fmt.Fprintln(os.Stderr, string(stderrLog))
			} else {
				fmt.Fprintf(os.Stderr, "=== Instance %q failed to read original stderr: %v\n", inst.name, logReadErr)
			}
		}
	}()

	if launchErr != nil {
		return fmt.Errorf("starting cluster: %v", launchErr)
	}

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
		var stopAll par.Funcs
		diag := driver.NewDiagnostics(cfg.bench.reportName)
		for _, typ := range diagnostics.Types() {
			if typ.HTTPEndpoint() == "" {
				continue
			}
			for i, inst := range instances {
				name := ""
				if !typ.CanMerge() {
					// Create a separate file for each instance.
					name = fmt.Sprintf("inst%d", i)
				}
				stop := server.FetchDiagnostic(inst.httpAddr(), diag, typ, name)
				stopAll.Add(stop)
			}
		}
		defer diag.Commit(d)
		defer stopAll.Run()

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
