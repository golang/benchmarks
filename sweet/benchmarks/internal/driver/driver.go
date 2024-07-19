// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package driver

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/pprof/profile"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

var (
	coreDumpDir string
	diag        diagnostics.DriverConfig
)

func SetFlags(f *flag.FlagSet) {
	f.StringVar(&coreDumpDir, "dump-cores", "", "dump a core file to the given directory after every benchmark run")
	diag.AddFlags(f)
}

const (
	StatPeakRSS = "peak-RSS-bytes"
	StatPeakVM  = "peak-VM-bytes"
	StatAvgRSS  = "average-RSS-bytes"
	StatTime    = "ns/op"
)

type RunOption func(*B)

func DoDefaultAvgRSS() RunOption {
	return func(b *B) {
		b.rssFunc = func() (uint64, error) {
			return ReadRSS(b.pid)
		}
	}
}

func DoAvgRSS(f func() (uint64, error)) RunOption {
	return func(b *B) {
		b.rssFunc = f
	}
}

func DoTime(v bool) RunOption {
	return func(b *B) {
		b.doTime = v
	}
}

func DoPeakRSS(v bool) RunOption {
	return func(b *B) {
		b.doPeakRSS = v
	}
}

func DoPeakVM(v bool) RunOption {
	return func(b *B) {
		b.doPeakVM = v
	}
}

func DoCoreDump(v bool) RunOption {
	return func(b *B) {
		b.doCoreDump = v
	}
}

func DoCPUProfile(v bool) RunOption {
	return func(b *B) {
		b.collectDiag[diagnostics.CPUProfile] = v
	}
}

func DoMemProfile(v bool) RunOption {
	return func(b *B) {
		b.collectDiag[diagnostics.MemProfile] = v
	}
}

func DoPerf(v bool) RunOption {
	return func(b *B) {
		b.collectDiag[diagnostics.Perf] = v
	}
}

func DoTrace(v bool) RunOption {
	return func(b *B) {
		b.collectDiag[diagnostics.Trace] = v
	}
}

func BenchmarkPID(pid int) RunOption {
	return func(b *B) {
		b.pid = pid
		if pid != os.Getpid() {
			b.collectDiag[diagnostics.CPUProfile] = false
			b.collectDiag[diagnostics.MemProfile] = false
			b.collectDiag[diagnostics.Perf] = false
			b.collectDiag[diagnostics.Trace] = false
		}
	}
}

func WithContext(ctx context.Context) RunOption {
	return func(b *B) {
		b.ctx = ctx
	}
}

func WriteResultsTo(wr io.Writer) RunOption {
	return func(b *B) {
		b.resultsWriter = wr
	}
}

func WithGOMAXPROCS(procs int) RunOption {
	return func(b *B) {
		b.gomaxprocs = procs
	}
}

var InProcessMeasurementOptions = []RunOption{
	DoTime(true),
	DoPeakRSS(true),
	DoDefaultAvgRSS(),
	DoPeakVM(true),
	DoCoreDump(true),
	DoCPUProfile(true),
	DoMemProfile(true),
	DoPerf(true),
	DoTrace(true),
}

type B struct {
	ctx           context.Context
	pid           int
	name          string
	start         time.Time
	dur           time.Duration
	doTime        bool
	doPeakRSS     bool
	doPeakVM      bool
	doCoreDump    bool
	gomaxprocs    int
	collectDiag   map[diagnostics.Type]bool
	rssFunc       func() (uint64, error)
	statsMu       sync.Mutex
	stats         map[string]uint64
	ops           int
	wg            sync.WaitGroup
	diagnostics   map[diagnostics.Type]*os.File
	resultsWriter io.Writer
	perfProcess   *os.Process
}

func newB(name string) *B {
	b := &B{
		pid:  os.Getpid(),
		name: name,
		collectDiag: map[diagnostics.Type]bool{
			diagnostics.CPUProfile: false,
			diagnostics.MemProfile: false,
		},
		stats:       make(map[string]uint64),
		ops:         1,
		diagnostics: make(map[diagnostics.Type]*os.File),
	}
	return b
}

func (b *B) setStat(name string, value uint64) {
	b.statsMu.Lock()
	defer b.statsMu.Unlock()
	b.stats[name] = value
}

func (b *B) shouldCollectDiag(typ diagnostics.Type) bool {
	return b.collectDiag[typ] && DiagnosticEnabled(typ)
}

func (b *B) Name() string {
	return b.name
}

func (b *B) StartTimer() {
	if b.shouldCollectDiag(diagnostics.CPUProfile) {
		pprof.StartCPUProfile(b.diagnostics[diagnostics.CPUProfile])
	}
	if b.shouldCollectDiag(diagnostics.Perf) {
		if err := b.startPerf(); err != nil {
			warningf("failed to start perf: %v", err)
		}
	}
	b.start = time.Now()
}

func (b *B) ResetTimer() {
	if b.shouldCollectDiag(diagnostics.CPUProfile) {
		pprof.StopCPUProfile()
		if err := b.truncateDiagnosticData(diagnostics.CPUProfile); err != nil {
			warningf("failed to truncate CPU profile: %v", err)
		}
		pprof.StartCPUProfile(b.diagnostics[diagnostics.CPUProfile])
	}
	if b.shouldCollectDiag(diagnostics.Perf) {
		if err := b.stopPerf(); err != nil {
			warningf("failed to stop perf: %v", err)
		}
		if err := b.truncateDiagnosticData(diagnostics.Perf); err != nil {
			warningf("failed to truncate perf data file: %v", err)
		}
		if err := b.startPerf(); err != nil {
			warningf("failed to start perf: %v", err)
		}
	}
	if !b.start.IsZero() {
		b.start = time.Now()
	}
	b.dur = 0
}

func (b *B) truncateDiagnosticData(typ diagnostics.Type) error {
	f := b.diagnostics[typ]
	_, err := f.Seek(0, 0)
	if err != nil {
		return err
	}
	return f.Truncate(0)
}

func (b *B) StopTimer() {
	end := time.Now()
	if b.start.IsZero() {
		panic("stopping unstarted timer")
	}
	b.dur += end.Sub(b.start)
	b.start = time.Time{}

	if b.shouldCollectDiag(diagnostics.CPUProfile) {
		pprof.StopCPUProfile()
	}
	if b.shouldCollectDiag(diagnostics.Perf) {
		if err := b.stopPerf(); err != nil {
			warningf("failed to stop perf: %v", err)
		}
	}
}

func (b *B) TimerRunning() bool {
	return !b.start.IsZero()
}

func (b *B) Elapsed() time.Duration {
	return b.dur
}

func (b *B) Report(name string, value uint64) {
	b.stats[name] = value
}

func (b *B) Ops(ops int) {
	b.ops = ops
}

func (b *B) Context() context.Context {
	if b.ctx != nil {
		return b.ctx
	}
	return context.Background()
}

func (b *B) startRSSSampler() chan<- struct{} {
	if b.rssFunc == nil {
		return nil
	}
	stop := make(chan struct{})
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()

		rssSamples := make([]uint64, 0, 1024)
		for {
			select {
			case <-stop:
				b.setStat(StatAvgRSS, avg(rssSamples))
				return
			case <-time.After(100 * time.Millisecond):
				r, err := b.rssFunc()
				if err != nil {
					warningf("failed to read RSS: %v", err)
					continue
				}
				if r == 0 {
					continue
				}
				rssSamples = append(rssSamples, r)
			}
		}
	}()
	return stop
}

func splitName(s string) []string {
	var comps []string
	last := 0
	for i, r := range s {
		if r == '-' || r == '*' || r == '/' {
			comps = append(comps, s[last:i])
			last = i + 1
		}
	}
	if len(comps) == 0 {
		comps = []string{s}
	}
	return comps
}

func (b *B) report() {
	b.statsMu.Lock()
	defer b.statsMu.Unlock()

	// Collect all names of non-zero stats.
	names := make([]string, 0, len(b.stats))
	for name, value := range b.stats {
		if value != 0 {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "# No benchmark results found for this run.")
		return
	}
	namesToComps := make(map[string][]string)
	for _, n := range names {
		namesToComps[n] = splitName(n)
	}
	sort.Slice(names, func(i, j int) bool {
		// Let's make sure StatTime always ends up first.
		if names[i] == StatTime {
			return true
		} else if names[j] == StatTime {
			return false
		}
		ci := namesToComps[names[i]]
		cj := namesToComps[names[j]]
		min := len(ci)
		if len(ci) > len(cj) {
			min = len(cj)
		}
		for i := 0; i < min; i++ {
			k := strings.Compare(ci[len(ci)-1-i], cj[len(cj)-1-i])
			if k < 0 {
				return true
			} else if k > 0 {
				return false
			}
		}
		return len(ci) < len(cj)
	})

	// Write out stats.
	var out io.Writer = os.Stderr
	if b.resultsWriter != nil {
		out = b.resultsWriter
	}
	suffix := ""
	if b.gomaxprocs > 1 {
		suffix = fmt.Sprintf("-%d", b.gomaxprocs)
	}
	fmt.Fprintf(out, "Benchmark%s%s %d", b.name, suffix, b.ops)
	for _, name := range names {
		value := b.stats[name]
		if value != 0 {
			fmt.Fprintf(out, " %d %s", value, name)
		}
	}
	fmt.Fprintln(out)
}

func warningf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	s = strings.Join(strings.Split(s, "\n"), "\n# ")
	fmt.Fprintf(os.Stderr, "# warning: %s\n", s)
}

func avg(s []uint64) uint64 {
	avg := uint64(0)
	lo := uint64(0)
	l := uint64(len(s))
	for i := 0; i < len(s); i++ {
		avg += s[i] / l
		mod := s[i] % l
		if lo >= l-mod {
			avg += 1
			lo -= l - mod
		} else {
			lo += mod
		}
	}
	return avg
}

func (b *B) startPerf() error {
	if b.perfProcess != nil {
		panic("perf process already started")
	}
	args := []string{"record", "-o", b.diagnostics[diagnostics.Perf].Name(), "-p", strconv.Itoa(b.pid)}
	args = append(args, PerfFlags()...)
	cmd := exec.Command("perf", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	b.perfProcess = cmd.Process
	return nil
}

func (b *B) stopPerf() error {
	if b.perfProcess == nil {
		panic("perf process not started")
	}
	proc := b.perfProcess
	b.perfProcess = nil

	if err := proc.Signal(os.Interrupt); err != nil {
		return err
	}
	_, err := proc.Wait()
	return err
}

func RunBenchmark(name string, f func(*B) error, opts ...RunOption) error {
	// Create a B and populate it with options.
	b := newB(name)
	for _, opt := range opts {
		opt(b)
	}

	// Make sure gomaxprocs is set.
	if b.gomaxprocs == 0 {
		b.gomaxprocs = runtime.GOMAXPROCS(-1)
	}

	// Start the RSS sampler and start the timer.
	stop := b.startRSSSampler()

	// Make sure profile file(s) are created if necessary.
	for _, typ := range diagnostics.Types() {
		if b.shouldCollectDiag(typ) {
			f, err := newDiagnosticDataFile(typ, b.name)
			if err != nil {
				return err
			}
			b.diagnostics[typ] = f
		}
	}

	if b.shouldCollectDiag(diagnostics.Trace) {
		if err := trace.Start(b.diagnostics[diagnostics.Trace]); err != nil {
			return err
		}
	}

	b.StartTimer()

	// Run the benchmark itself.
	if err := f(b); err != nil {
		return err
	}
	if b.TimerRunning() {
		b.StopTimer()
	}

	// Stop the RSS sampler.
	if stop != nil {
		stop <- struct{}{}
	}

	if b.doPeakRSS {
		v, err := ReadPeakRSS(b.pid)
		if err != nil {
			warningf("failed to read RSS peak: %v", err)
		} else if v != 0 {
			b.setStat(StatPeakRSS, v)
		}
	}
	if b.doPeakVM {
		v, err := ReadPeakVM(b.pid)
		if err != nil {
			warningf("failed to read VM peak: %v", err)
		} else if v != 0 {
			b.setStat(StatPeakVM, v)
		}
	}
	if b.doTime {
		if b.dur == 0 {
			panic("timer never stopped")
		} else if b.dur < 0 {
			panic("negative duration encountered")
		}
		if b.ops == 0 {
			panic("zero ops reported")
		} else if b.ops < 0 {
			panic("negative ops encountered")
		}
		b.setStat(StatTime, uint64(b.dur.Nanoseconds())/uint64(b.ops))
	}
	if b.doCoreDump && coreDumpDir != "" {
		// Use gcore to dump the core of the benchmark process.
		cmd := exec.Command(
			"gcore", "-o", filepath.Join(coreDumpDir, name), strconv.Itoa(b.pid),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			// Just print a warning; this isn't a fatal error.
			warningf("failed to dump core: %v\n%s", err, string(out))
		}
	}

	b.wg.Wait()

	// Finalize all the profile files we're handling ourselves.
	for typ, f := range b.diagnostics {
		if typ == diagnostics.MemProfile {
			if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
				return err
			}
		}
		if typ == diagnostics.Trace {
			trace.Stop()
		}
		f.Close()
	}

	// Report the results.
	b.report()
	return nil
}

func DiagnosticEnabled(typ diagnostics.Type) bool {
	_, ok := diag.ConfigSet.Get(typ)
	return ok
}

func WritePprofProfile(prof *profile.Profile, typ diagnostics.Type, pattern string) error {
	if !typ.IsPprof() {
		return fmt.Errorf("this type of diagnostic doesn't use the pprof format")
	}
	if !DiagnosticEnabled(typ) {
		return fmt.Errorf("this type of diagnostic is not currently enabled")
	}
	f, err := newDiagnosticDataFile(typ, pattern)
	if err != nil {
		return err
	}
	defer f.Close()
	return prof.Write(f)
}

func CopyDiagnosticData(diagPath string, typ diagnostics.Type, pattern string) error {
	inF, err := os.Open(diagPath)
	if err != nil {
		return err
	}
	defer inF.Close()
	outF, err := newDiagnosticDataFile(typ, pattern)
	if err != nil {
		return err
	}
	defer outF.Close()
	_, err = io.Copy(outF, inF)
	return err
}

func PerfFlags() []string {
	cfg, ok := diag.ConfigSet.Get(diagnostics.Perf)
	if !ok {
		panic("perf not enabled")
	}
	return strings.Split(cfg.Flags, " ")
}

func newDiagnosticDataFile(typ diagnostics.Type, pattern string) (*os.File, error) {
	if !DiagnosticEnabled(typ) {
		return nil, fmt.Errorf("this type of profile is not currently enabled")
	}
	return os.CreateTemp(diag.ResultsDir, pattern+"."+string(typ))
}
