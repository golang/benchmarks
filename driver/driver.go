// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains common benchmarking logic shared between benchmarks.
// It defines the main function which calls one of the benchmarks registered
// with Register function.
// When a benchmark is invoked it has 2 choices:
// 1. Do whatever it wants, fill and return Result object.
// 2. Call Benchmark helper function and provide benchmarking function
// func(N uint64), similar to standard testing benchmarks. The rest is handled
// by the driver.

package driver

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	bench     = flag.String("bench", "", "benchmark to run")
	flake     = flag.Int("flake", 0, "test flakiness of a benchmark")
	benchNum  = flag.Int("benchnum", 5, "number of benchmark runs")
	benchMem  = flag.Int("benchmem", 64, "approx RSS value to aim at in benchmarks, in MB")
	benchTime = flag.Duration("benchtime", 5*time.Second, "run enough iterations of each benchmark to take the specified time")
	affinity  = flag.Int("affinity", 0, "process affinity (passed to an OS-specific function like sched_setaffinity/SetProcessAffinityMask)")
	tmpDir    = flag.String("tmpdir", os.TempDir(), "dir for temporary files")
	genSvg    = flag.Bool("svg", false, "generate svg profiles")

	BenchNum  int
	BenchMem  int
	BenchTime time.Duration
	WorkDir   string

	benchmarks = make(map[string]func() Result)
)

func Register(name string, f func() Result) {
	benchmarks[name] = f
}

func Main() {
	flag.Parse()
	// Copy to public variables, so that benchmarks can access the values.
	BenchNum = *benchNum
	BenchMem = *benchMem
	BenchTime = *benchTime
	WorkDir = *tmpDir

	if *affinity != 0 {
		setProcessAffinity(*affinity)
	}

	if *bench == "" {
		printBenchmarks()
		return
	}
	f := benchmarks[*bench]
	if f == nil {
		fmt.Printf("unknown benchmark '%v'\n", *bench)
		os.Exit(1)
	}

	setupWatchdog()

	if *flake > 0 {
		testFlakiness(f, *flake)
		return
	}

	res := f()

	var metrics []string
	for k := range res.Metrics {
		metrics = append(metrics, k)
	}
	sort.Strings(metrics)
	for _, m := range metrics {
		fmt.Printf("GOPERF-METRIC:%v=%v\n", m, res.Metrics[m])
	}

	var files []string
	for k := range res.Files {
		files = append(files, k)
	}
	sort.Strings(files)
	for _, f := range files {
		fmt.Printf("GOPERF-FILE:%v=%v\n", f, res.Files[f])
	}
}

func printBenchmarks() {
	var bb []string
	for name, _ := range benchmarks {
		bb = append(bb, name)
	}
	sort.Strings(bb)
	for i, name := range bb {
		if i != 0 {
			fmt.Print(",")
		}
		fmt.Print(name)
	}
	fmt.Print("\n")
}

func setupWatchdog() {
	t := *benchTime
	// Be somewhat conservative, and build benchmark does not care about benchTime.
	if t < time.Minute {
		t = time.Minute
	}
	t *= time.Duration(*benchNum)
	t *= 2 // to account for iteration number auto-tuning
	if *flake > 0 {
		t *= time.Duration(*flake + 2)
	}
	go func() {
		time.Sleep(t)
		panic(fmt.Sprintf("timed out after %v", t))
	}()
}

// testFlakiness runs the function N+2 times and prints metrics diffs between
// the second and subsequent runs.
func testFlakiness(f func() Result, N int) {
	res := make([]Result, N+2)
	for i := range res {
		res[i] = f()
	}
	fmt.Printf("\n")
	for k, v := range res[0].Metrics {
		fmt.Printf("%v:\t", k)
		for i := 2; i < len(res); i++ {
			d := 100*float64(v)/float64(res[i].Metrics[k]) - 100
			fmt.Printf(" %+.2f%%", d)
		}
		fmt.Printf("\n")
	}
}

// Result contains all the interesting data about benchmark execution.
type Result struct {
	N        uint64        // number of iterations
	Duration time.Duration // total run duration
	RunTime  uint64        // ns/op
	Metrics  map[string]uint64
	Files    map[string]string
}

func MakeResult() Result {
	return Result{Metrics: make(map[string]uint64), Files: make(map[string]string)}
}

// Benchmark runs f several times, collects stats, chooses the best run
// and creates cpu/mem profiles.
func Benchmark(f func(uint64)) Result {
	res := MakeResult()
	for i := 0; i < *benchNum; i++ {
		res1 := runBenchmark(f)
		if res.N == 0 || res.RunTime > res1.RunTime {
			res = res1
		}
		// Always take RSS and sys memory metrics from last iteration.
		// They only grow, and seem to converge to some eigen value.
		// Variations are smaller if we do this.
		for k, v := range res1.Metrics {
			if k == "rss" || strings.HasPrefix(k, "sys-") {
				res.Metrics[k] = v
			}
		}
	}

	cpuprof := processProfile(os.Args[0], res.Files["cpuprof"])
	delete(res.Files, "cpuprof")
	if cpuprof != "" {
		res.Files["cpuprof"] = cpuprof
	}

	memprof := processProfile("--lines", "--show_bytes", "--alloc_space", "--base", res.Files["memprof0"], os.Args[0], res.Files["memprof"])
	delete(res.Files, "memprof")
	delete(res.Files, "memprof0")
	if memprof != "" {
		res.Files["memprof"] = memprof
	}

	return res
}

// processProfile invokes 'go tool pprof' with the specified args
// and returns name of the resulting file, or an empty string.
func processProfile(args ...string) string {
	fname := "prof.txt"
	typ := "--text"
	if *genSvg {
		fname = "prof.svg"
		typ = "--svg"
	}
	proff, err := os.Create(tempFilename(fname))
	if err != nil {
		log.Printf("Failed to create profile file: %v", err)
		return ""
	}
	defer proff.Close()
	var proflog bytes.Buffer
	cmdargs := append([]string{"tool", "pprof", typ}, args...)
	cmd := exec.Command("go", cmdargs...)
	cmd.Stdout = proff
	cmd.Stderr = &proflog
	err = cmd.Run()
	if err != nil {
		log.Printf("go tool pprof cpuprof failed: %v\n%v", err, proflog.String())
		return "" // Deliberately ignore the error.
	}
	return proff.Name()
}

// runBenchmark runs f several times with increasing number of iterations
// until execution time reaches the requested duration.
func runBenchmark(f func(uint64)) Result {
	res := MakeResult()
	for chooseN(&res) {
		log.Printf("Benchmarking %v iterations\n", res.N)
		res = runBenchmarkOnce(f, res.N)
	}
	log.Printf("Result: %+v\n", res)
	return res
}

// runBenchmarkOnce runs f once and collects all performance metrics and profiles.
func runBenchmarkOnce(f func(uint64), N uint64) Result {
	latencyInit(N)
	runtime.GC()
	mstats0 := new(runtime.MemStats)
	runtime.ReadMemStats(mstats0)
	ss := InitSysStats(N)
	res := MakeResult()
	res.N = N
	res.Files["memprof0"] = tempFilename("memprof")
	memprof0, err := os.Create(res.Files["memprof0"])
	if err != nil {
		log.Fatalf("Failed to create profile file '%v': %v", res.Files["memprof0"], err)
	}
	pprof.WriteHeapProfile(memprof0)
	memprof0.Close()

	res.Files["cpuprof"] = tempFilename("cpuprof")
	cpuprof, err := os.Create(res.Files["cpuprof"])
	if err != nil {
		log.Fatalf("Failed to create profile file '%v': %v", res.Files["cpuprof"], err)
	}
	defer cpuprof.Close()
	pprof.StartCPUProfile(cpuprof)
	t0 := time.Now()
	f(N)
	res.Duration = time.Since(t0)
	res.RunTime = uint64(time.Since(t0)) / N
	res.Metrics["time"] = res.RunTime
	pprof.StopCPUProfile()

	latencyCollect(&res)
	ss.Collect(&res)

	res.Files["memprof"] = tempFilename("memprof")
	memprof, err := os.Create(res.Files["memprof"])
	if err != nil {
		log.Fatalf("Failed to create profile file '%v': %v", res.Files["memprof"], err)
	}
	pprof.WriteHeapProfile(memprof)
	memprof.Close()

	mstats1 := new(runtime.MemStats)
	runtime.ReadMemStats(mstats1)
	res.Metrics["allocated"] = (mstats1.TotalAlloc - mstats0.TotalAlloc) / N
	res.Metrics["allocs"] = (mstats1.Mallocs - mstats0.Mallocs) / N
	res.Metrics["sys-total"] = mstats1.Sys
	res.Metrics["sys-heap"] = mstats1.HeapSys
	res.Metrics["sys-stack"] = mstats1.StackSys
	res.Metrics["gc-pause-total"] = (mstats1.PauseTotalNs - mstats0.PauseTotalNs) / N
	collectGo12MemStats(&res, mstats0, mstats1)
	numGC := uint64(mstats1.NumGC - mstats0.NumGC)
	if numGC == 0 {
		res.Metrics["gc-pause-one"] = 0
	} else {
		res.Metrics["gc-pause-one"] = (mstats1.PauseTotalNs - mstats0.PauseTotalNs) / numGC
	}
	return res
}

// Parallel is a public helper function that runs f N times in P*GOMAXPROCS goroutines.
func Parallel(N uint64, P int, f func()) {
	numProcs := P * runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	wg.Add(numProcs)
	for p := 0; p < numProcs; p++ {
		go func() {
			defer wg.Done()
			for int64(atomic.AddUint64(&N, ^uint64(0))) >= 0 {
				f()
			}
		}()
	}
	wg.Wait()
}

// perfLatency collects and reports information about latencies.
var latency struct {
	data latencyData
	idx  int32
}

type latencyData []uint64

func (p latencyData) Len() int           { return len(p) }
func (p latencyData) Less(i, j int) bool { return p[i] < p[j] }
func (p latencyData) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func latencyInit(N uint64) {
	N = min(N, 1e6) // bound the amount of memory consumed
	latency.data = make(latencyData, N)
	latency.idx = 0
}

func LatencyNote(t time.Time) {
	d := time.Since(t)
	if int(atomic.LoadInt32(&latency.idx)) >= len(latency.data) {
		return
	}
	i := atomic.AddInt32(&latency.idx, 1) - 1
	if int(i) >= len(latency.data) {
		return
	}
	latency.data[i] = uint64(d)
}

func latencyCollect(res *Result) {
	cnt := int(latency.idx)
	if cnt == 0 {
		return
	}
	if cnt > len(latency.data) {
		cnt = len(latency.data)
	}
	sort.Sort(latency.data[:cnt])
	res.Metrics["latency-50"] = latency.data[cnt*50/100]
	res.Metrics["latency-95"] = latency.data[cnt*95/100]
	res.Metrics["latency-99"] = latency.data[cnt*99/100]
}

// chooseN chooses the next number of iterations for benchmark.
func chooseN(res *Result) bool {
	const MaxN = 1e12
	last := res.N
	if last == 0 {
		res.N = 1
		return true
	} else if res.Duration >= *benchTime || last >= MaxN {
		return false
	}
	nsPerOp := max(1, res.RunTime)
	res.N = uint64(*benchTime) / nsPerOp
	res.N = max(min(res.N+res.N/2, 100*last), last+1)
	res.N = roundUp(res.N)
	return true
}

// roundUp rounds the number of iterations to a nice value.
func roundUp(n uint64) uint64 {
	tmp := n
	base := uint64(1)
	for tmp >= 10 {
		tmp /= 10
		base *= 10
	}
	switch {
	case n <= base:
		return base
	case n <= (2 * base):
		return 2 * base
	case n <= (5 * base):
		return 5 * base
	default:
		return 10 * base
	}
	panic("unreachable")
	return 0
}

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

var tmpSeq = 0

func tempFilename(ext string) string {
	tmpSeq++
	return filepath.Join(*tmpDir, fmt.Sprintf("%v.%v", tmpSeq, ext))
}
