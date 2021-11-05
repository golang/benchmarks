// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.16
// +build go1.16

package main

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type BenchStat struct {
	Name                        string
	RealTime, UserTime, SysTime time.Duration
}

type Benchmark struct {
	Name       string   // Short name for benchmark/test
	Contact    string   // Contact not used, but may be present in description
	Repo       string   // Repo + subdir where test resides, used for "go get -t -d ..."
	Tests      string   // Tests to run (regex for -test.run= )
	Benchmarks string   // Benchmarks to run (regex for -test.bench= )
	GcEnv      []string // Environment variables supplied to 'go test -c' for building, getting
	BuildFlags []string // Flags for building test (e.g., -tags purego)
	RunWrapper []string // (Inner) Command and args to precede whatever the operation is; may fail in the sandbox.
	// e.g. benchmark may run as ConfigWrapper ConfigArg BenchWrapper BenchArg ActualBenchmark
	NotSandboxed bool   // True if this benchmark cannot or should not be run in a container.
	Disabled     bool   // True if this benchmark is temporarily disabled.
	RunDir       string // Parent directory of testdata.
}

type Todo struct {
	Benchmarks     []Benchmark
	Configurations []Configuration
}

// The length of the path to the root of the git repo, inclusive.
// For example, github.com/dr2chase/bent <--- bent is the repo.
var pathLengths = map[string]int{
	"github.com":    3,
	"gitlab.com":    3,
	"zombiezen.com": 3,
	"gonum.org":     3,
	"k8s.io":        2,
	"go.uber.org":   2,
}

var verbose counterFlag

var benchFile = "benchmarks-50.toml"         // default list of benchmarks
var confFile = "configurations.toml"         // default list of configurations
var srcPath = "src/github.com/dr2chase/bent" // Used to find configuration files.
var container = ""
var N = 1
var list = false
var initialize = false
var test = false
var force = false
var requireSandbox = false
var getOnly = false
var runContainer = ""       // if nonempty, skip builds and use existing named container (or binaries if -U )
var wikiTable = false       // emit the tests in a form usable in a wiki table
var explicitAll counterFlag // Include "-a" on "go test -c" test build ; repeating flag causes multiple rebuilds, useful for build benchmarking.
var shuffle = 2             // Dimensionality of (build) shuffling; 0 = none, 1 = per-benchmark, configuration ordering, 2 = bench, config pairs, 3 = across repetitions.
var haveRsync = true

//go:embed scripts/*
var scripts embed.FS

//go:embed configs/*
var configs embed.FS

var copyExes = []string{
	"foo", "memprofile", "cpuprofile", "tmpclr", "benchtime", "benchsize", "benchdwarf", "cronjob.sh", "cmpjob.sh", "cmpcl.sh", "cmpcl-phase.sh", "tweet-results",
}

var copyConfigs = []string{
	"benchmarks-all.toml", "benchmarks-50.toml", "benchmarks-gc.toml", "benchmarks-gcplus.toml", "benchmarks-trial.toml",
	"configurations-sample.toml", "configurations-gollvm.toml", "configurations-cronjob.toml", "configurations-cmpjob.toml",
}

var defaultEnv []string

type pair struct {
	b, c int
}
type triple struct {
	b, c, k int
}

// To disambiguate repeated test runs in the same directory.
var runstamp = strings.Replace(strings.Replace(time.Now().Format("2006-01-02T15:04:05"), "-", "", -1), ":", "", -1)

func cleanup(gopath string) {
	bin := path.Join(gopath, "bin")
	if verbose > 0 {
		fmt.Printf("rm -rf %s\n", bin)
	}
	os.RemoveAll(bin)
}

func main() {

	var benchmarksString, configurationsString, stampLog string

	flag.IntVar(&N, "N", N, "benchmark/test repeat count")

	flag.Var(&explicitAll, "a", "add '-a' flag to 'go test -c' to demand full recompile. Repeat or assign a value for repeat builds for benchmarking")
	flag.IntVar(&shuffle, "s", shuffle, "dimensionality of (build) shuffling (0-3), 0 = none, 1 = per-benchmark, configuration ordering, 2 = bench, config pairs, 3 = across repetitions.")

	flag.StringVar(&benchmarksString, "b", "", "comma-separated list of test/benchmark names (default is all)")
	flag.StringVar(&benchFile, "B", benchFile, "name of file containing benchmarks to run")

	flag.StringVar(&configurationsString, "c", "", "comma-separated list of test/benchmark configurations (default is all)")
	flag.StringVar(&confFile, "C", confFile, "name of file describing configurations")

	flag.BoolVar(&requireSandbox, "S", requireSandbox, "require Docker sandbox to run tests/benchmarks (& exclude unsandboxable tests/benchmarks)")

	flag.BoolVar(&getOnly, "g", getOnly, "get tests/benchmarks and dependencies, do not build or run")
	flag.StringVar(&runContainer, "r", runContainer, "skip get and build, go directly to run, using specified container (any non-empty string will do for unsandboxed execution)")

	flag.StringVar(&stampLog, "L", stampLog, "name of log file to which runstamps are appended")

	flag.BoolVar(&list, "l", list, "list available benchmarks and configurations, then exit")
	flag.BoolVar(&force, "f", force, "force run past some of the consistency checks (gopath/{pkg,bin} in particular)")
	flag.BoolVar(&initialize, "I", initialize, "initialize a directory for running tests ((re)creates Dockerfile, (re)copies in benchmark and configuration files)")
	flag.BoolVar(&test, "T", test, "run tests instead of benchmarks")

	flag.BoolVar(&wikiTable, "W", wikiTable, "print benchmark info for a wiki table")

	flag.Var(&verbose, "v", "print commands and other information (more -v = print more details)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr,
			`
%s obtains the benchmarks/tests listed in %s and compiles
and runs them according to the flags and environment
variables supplied in %s.

Specifying "-a" will pass "-a" to test compilations, but normally this
should not be needed and only slows down builds; -a with a number
that is not 1 can be used for benchmarking builds of the tests
themselves. (Don't forget to specify "all=..." for GCFLAGS if you want
those applied to the entire build.)

Both of these files can be changed with the -B and -C flags; the full
suite of benchmarks in benchmarks-all.toml is somewhat
time-consuming. Suites.toml contains the known benchmarks, their
versions, and certain necessary build or run flags.

Running with the -l flag will list all the available tests and
benchmarks for the given benchmark and configuration files.

By default benchmarks are run, not tests.  -T runs tests instead.

To run tests or benchmnarks in a docker sandbox, specify -S; if the
host OS is not linux this will exclude some benchmarks that cannot be
cross-compiled.

All the test binaries will appear in the subdirectory 'testbin', and
test (benchmark) output will appear in the subdirectory 'bench' with
the suffix '.stdout'.  The test output is grouped by configuration to
allow easy benchmark comparisons with benchstat.  Other benchmarking
results will also appear in 'bench'.
`, os.Args[0], benchFile, confFile)
	}

	flag.Parse()

	_, errRsync := exec.LookPath("rsync")
	if errRsync != nil {
		haveRsync = false
		fmt.Println("Warning: using cp instead of rsync")
	}

	if requireSandbox {
		_, errDocker := exec.LookPath("docker")
		if errDocker != nil {
			fmt.Println("Sandboxing benchmarks requires the docker command")
			os.Exit(1)
		}
	}

	// Make sure our filesystem is in good shape.
	if err := checkAndSetUpFileSystem(initialize); err != nil {
		fmt.Printf("%v", err)
		os.Exit(1)
	}

	var err error
	// Create any directories we need.
	dirs, err = createDirectories(0775)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	todo := &Todo{}
	blobB, err := ioutil.ReadFile(benchFile)
	if err != nil {
		fmt.Printf("There was an error opening or reading file %s: %v\n", benchFile, err)
		os.Exit(1)
	}
	blobC, err := ioutil.ReadFile(confFile)
	if err != nil {
		fmt.Printf("There was an error opening or reading file %s: %v\n", confFile, err)
		os.Exit(1)
	}
	blob := append(blobB, blobC...)
	err = toml.Unmarshal(blob, todo)
	if err != nil {
		fmt.Printf("There was an error unmarshalling %s: %v\n", string(blob), err)
		os.Exit(1)
	}

	var moreArgs []string
	if flag.NArg() > 0 {
		for i, arg := range flag.Args() {
			if i == 0 && (arg == "-" || arg == "--") {
				continue
			}
			moreArgs = append(moreArgs, arg)
		}
	}

	benchmarks := csToSet(benchmarksString)
	configurations := csToSet(configurationsString)

	if wikiTable {
		for _, bench := range todo.Benchmarks {
			s := bench.Benchmarks
			s = strings.Replace(s, "|", "\\|", -1)
			fmt.Printf(" | %s | | `%s` | `%s` | |\n", bench.Name, bench.Repo, s)
		}
		return
	}

	// Normalize configuration goroot names by ensuring they end in '/'
	// Process command-line-specified configurations.
	// Expand environment variables mentioned there.
	duplicates := make(map[string]bool)
	for i, trial := range todo.Configurations {
		trial.Name = os.ExpandEnv(trial.Name)
		todo.Configurations[i].Name = trial.Name
		if duplicates[trial.Name] {
			if trial.Name == todo.Configurations[i].Name {
				fmt.Printf("Saw duplicate configuration %s at index %d\n", trial.Name, i)
			} else {
				fmt.Printf("Saw duplicate configuration %s (originally %s) at index %d\n", trial.Name, todo.Configurations[i].Name, i)
			}
			os.Exit(1)
		}
		duplicates[trial.Name] = true
		if configurations != nil {
			_, present := configurations[trial.Name]
			todo.Configurations[i].Disabled = !present
			if present {
				configurations[trial.Name] = false
			}
		}
		if root := trial.Root; len(root) != 0 {
			// TODO(jfaller): I don't think we need this "/" anymore... investigate.
			todo.Configurations[i].Root = os.ExpandEnv(root) + "/"
		}
		for j, s := range trial.GcEnv {
			trial.GcEnv[j] = os.ExpandEnv(s)
		}
		for j, s := range trial.RunEnv {
			trial.RunEnv[j] = os.ExpandEnv(s)
		}
		todo.Configurations[i].GcFlags = os.ExpandEnv(trial.GcFlags)
		for j, s := range trial.RunFlags {
			trial.RunFlags[j] = os.ExpandEnv(s)
		}
		for j, s := range trial.RunWrapper {
			trial.RunWrapper[j] = os.ExpandEnv(s)
		}
	}
	for b, v := range configurations {
		if v {
			fmt.Printf("Configuration %s listed after -c does not appear in %s\n", b, confFile)
			os.Exit(1)
		}
	}

	// Normalize benchmark names by removing any trailing '/'.
	// Normalize Test and Benchmark specs by replacing missing value with something that won't match anything.
	// Process command-line-specified benchmarks
	duplicates = make(map[string]bool)
	for i, bench := range todo.Benchmarks {

		if duplicates[bench.Name] {
			fmt.Printf("Saw duplicate benchmark %s at index %d\n", bench.Name, i)
			os.Exit(1)
		}
		duplicates[bench.Name] = true

		if benchmarks != nil {
			_, present := benchmarks[bench.Name]
			todo.Benchmarks[i].Disabled = !present
			if present {
				benchmarks[bench.Name] = false
			}
		}
		for j, s := range bench.GcEnv {
			bench.GcEnv[j] = os.ExpandEnv(s)
		}
		// Trim possible trailing slash, do not want
		if '/' == bench.Repo[len(bench.Repo)-1] {
			bench.Repo = bench.Repo[:len(bench.Repo)-1]
			todo.Benchmarks[i].Repo = bench.Repo
		}
		if "" == bench.Tests || !test {
			if !test {
				todo.Benchmarks[i].Tests = "none"
			} else {
				todo.Benchmarks[i].Tests = "Test"
			}
		}
		if "" == bench.Benchmarks || test {
			if !test {
				todo.Benchmarks[i].Benchmarks = "Benchmark"
			} else {
				todo.Benchmarks[i].Benchmarks = "none"
			}
		}
		if !requireSandbox {
			todo.Benchmarks[i].NotSandboxed = true
		}
		if requireSandbox && todo.Benchmarks[i].NotSandboxed {
			if runtime.GOOS == "linux" {
				fmt.Printf("Removing sandbox for %s\n", bench.Name)
				todo.Benchmarks[i].NotSandboxed = false
			} else {
				fmt.Printf("Disabling %s because it requires sandbox\n", bench.Name)
				todo.Benchmarks[i].Disabled = true
			}
		}
	}
	for b, v := range benchmarks {
		if v {
			fmt.Printf("Benchmark %s listed after -b does not appear in %s\n", b, benchFile)
			os.Exit(1)
		}
	}

	// If more verbose, print the normalized configuration.
	if verbose > 1 {
		buf := new(bytes.Buffer)
		if err := toml.NewEncoder(buf).Encode(todo); err != nil {
			fmt.Printf("There was an error encoding %v: %v\n", todo, err)
			os.Exit(1)
		}
		fmt.Println(buf.String())
	}

	if list {
		fmt.Println("Benchmarks:")
		for _, x := range todo.Benchmarks {
			s := x.Name + " (repo=" + x.Repo + ")"
			if x.Disabled {
				s += " (disabled)"
			}
			fmt.Printf("   %s\n", s)
		}
		fmt.Println("Configurations:")
		for _, x := range todo.Configurations {
			s := x.Name
			if x.Root != "" {
				s += " (goroot=" + x.Root + ")"
			}
			if x.Disabled {
				s += " (disabled)"
			}
			fmt.Printf("   %s\n", s)
		}
		return
	}

	if stampLog != "" {
		f, err := os.OpenFile(stampLog, os.O_WRONLY|os.O_APPEND|os.O_CREATE, os.ModePerm)
		if err != nil {
			fmt.Printf("There was an error opening %s for output, error %v\n", stampLog, err)
			os.Exit(2)
		}
		fmt.Fprintf(f, "%s\t%v\n", runstamp, os.Args)
		f.Close()
	}

	defaultEnv = inheritEnv(defaultEnv, "PATH")
	defaultEnv = inheritEnv(defaultEnv, "USER")
	defaultEnv = inheritEnv(defaultEnv, "HOME")
	defaultEnv = inheritEnv(defaultEnv, "SHELL")
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GO") {
			defaultEnv = append(defaultEnv, e)
		}
	}
	defaultEnv = replaceEnv(defaultEnv, "GOPATH", dirs.gopath)
	defaultEnv = replaceEnv(defaultEnv, "GOOS", runtime.GOOS)
	defaultEnv = replaceEnv(defaultEnv, "GOARCH", runtime.GOARCH)
	defaultEnv = ifMissingAddEnv(defaultEnv, "GO111MODULE", "auto")

	var needSandbox bool    // true if any benchmark needs a sandbox
	var needNotSandbox bool // true if any benchmark needs to be not sandboxed

	var getAndBuildFailures []string

	// Ignore the error -- TODO note the difference between exists already and other errors.

	for i, config := range todo.Configurations {
		if !config.Disabled { // Don't overwrite if something was disabled.
			s := config.thingBenchName("stdout")
			f, err := os.OpenFile(s, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
			if err != nil {
				fmt.Printf("There was an error opening %s for output, error %v\n", s, err)
				os.Exit(2)
			}
			todo.Configurations[i].benchWriter = f
		}
	}

	// It is possible to request repeated builds for compiler/linker benchmarking.
	// Normal (non-negative build count) varies configuration most frequently,
	// then benchmark, then repeats the process N times (innerBuildCount = 1).
	// If build count is negative, the configuration varies least frequently,
	// and each benchmark is built buildCount (innerBuildCount) times before
	// moving on to the next. (This tends to focus intermittent benchmarking
	// noise on single configuration-benchmark combos.  This is the "old way".
	buildCount := int(explicitAll)
	if buildCount < 0 {
		buildCount = -buildCount
	}
	if buildCount == 0 {
		buildCount = 1
	}

	if runContainer == "" { // If not reusing binaries/container...
		if verbose == 0 {
			fmt.Print("Go getting")
		}

		goDotMod := path.Join(dirs.build, "go.mod")

		if _, err := os.Stat(goDotMod); err != nil { // if error assume go.mod does not exist
			cmd := exec.Command("go", "mod", "init", "build")
			cmd.Env = defaultEnv
			cmd.Dir = dirs.build

			if verbose > 0 {
				fmt.Println(asCommandLine(dirs.wd, cmd))
			} else {
				fmt.Print(".")
			}
			_, err := cmd.Output()
			if err != nil {
				ee := err.(*exec.ExitError)
				fmt.Printf("There was an error running 'go mod init', stderr = %s", ee.Stderr)
				os.Exit(2)
			}
		}

		// Obtain (go get -d -t -v bench.Repo) all benchmarks, once, populating src
		for i, bench := range todo.Benchmarks {
			if bench.Disabled {
				continue
			}
			getBuildEnv := replaceEnvs(defaultEnv, bench.GcEnv)

			cmd := exec.Command("go", "get", "-d", "-t", "-v", bench.Repo)
			cmd.Env = getBuildEnv
			cmd.Dir = dirs.build

			if !bench.NotSandboxed { // Do this so that OS-dependent dependencies are done correctly.
				cmd.Env = replaceEnv(cmd.Env, "GOOS", "linux")
			}
			if verbose > 0 {
				fmt.Println(asCommandLine(dirs.wd, cmd))
			} else {
				fmt.Print(".")
			}
			_, err := cmd.Output()
			if err != nil {
				ee := err.(*exec.ExitError)
				s := fmt.Sprintf("There was an error running 'go get', stderr = %s", ee.Stderr)
				fmt.Println(s + "DISABLING benchmark " + bench.Name)
				getAndBuildFailures = append(getAndBuildFailures, s+"("+bench.Name+")\n")
				todo.Benchmarks[i].Disabled = true
				continue
			}

			needSandbox = !bench.NotSandboxed || needSandbox
			needNotSandbox = bench.NotSandboxed || needNotSandbox
		}
		if verbose == 0 {
			fmt.Println()
		}

		if getOnly {
			return
		}

		// Create build-related benchmark files
		for ci := range todo.Configurations {
			todo.Configurations[ci].createFilesForLater()
		}

		// Compile tests and move to ./testbin/Bench_Config.
		// If any test needs sandboxing, then one docker container will be created
		// (that contains all the tests).

		if verbose == 0 {
			fmt.Print("Building goroots")
		}

		// First for each configuration, get the compiler and library and install it in its own GOROOT.
		for ci, config := range todo.Configurations {
			if config.Disabled {
				continue
			}

			root := config.Root

			rootCopy := path.Join(dirs.goroots, config.Name)
			if verbose > 0 {
				fmt.Printf("rm -rf %s\n", rootCopy)
			}
			os.RemoveAll(rootCopy)
			config.rootCopy = rootCopy
			todo.Configurations[ci] = config

			docopy := func(from, to string) {
				mkdir := exec.Command("mkdir", "-p", to)
				s, _ := config.runBinary("", mkdir, false)
				if s != "" {
					fmt.Println("Error creating directory, ", to)
					config.Disabled = true
				}

				var cp *exec.Cmd
				if haveRsync {
					cp = exec.Command("rsync", "-a", from+"/", to)
				} else {
					cp = exec.Command("cp", "-a", from+"/.", to)
				}
				s, _ = config.runBinary("", cp, false)
				if s != "" {
					fmt.Println("Error copying directory tree, ", from, to)
					// Not disabling because gollvm uses a different directory structure
				}
			}

			docopy(path.Join(root, "bin"), path.Join(rootCopy, "bin"))
			docopy(path.Join(root, "src"), path.Join(rootCopy, "src"))
			docopy(path.Join(root, "pkg"), path.Join(rootCopy, "pkg"))

			gocmd := config.goCommandCopy()

			buildLibrary := func(withAltOS bool) {
				if withAltOS && runtime.GOOS == "linux" {
					return // The alternate OS is linux
				}
				cmd := exec.Command(gocmd, "install", "-a")
				cmd.Args = append(cmd.Args, config.BuildFlags...)
				if config.GcFlags != "" {
					cmd.Args = append(cmd.Args, "-gcflags="+config.GcFlags)
				}
				cmd.Args = append(cmd.Args, "std")
				cmd.Env = defaultEnv
				if withAltOS {
					cmd.Env = replaceEnv(cmd.Env, "GOOS", "linux")
				}
				if rootCopy != "" {
					cmd.Env = replaceEnv(cmd.Env, "GOROOT", rootCopy)
				}
				cmd.Env = replaceEnvs(cmd.Env, config.GcEnv)

				s, _ := config.runBinary("", cmd, true)
				if s != "" {
					fmt.Println("Error running go install std, ", s)
					config.Disabled = true
				}
			}

			// Prebuild the library for this configuration unless -a=1
			if explicitAll != 1 {
				if needSandbox {
					buildLibrary(true)
				}
				if needNotSandbox {
					buildLibrary(false)
				}
			}
			todo.Configurations[ci] = config
			if config.Disabled {
				continue
			}
		}

		if verbose == 0 {
			fmt.Print("\nCompiling")
		}

		switch shuffle {
		case 0: // N times, for each benchmark, for each configuration, build.
			for yyy := 0; yyy < buildCount; yyy++ {
				for bi, bench := range todo.Benchmarks {
					if bench.Disabled {
						continue
					}
					for ci, config := range todo.Configurations {
						if config.Disabled {
							continue
						}
						s := todo.Configurations[ci].compileOne(&todo.Benchmarks[bi], dirs.wd, yyy)
						if s != "" {
							getAndBuildFailures = append(getAndBuildFailures, s)
						}
					}
				}
			}
		case 1: // N times, for each benchmark, shuffle configurations and build with
			permute := make([]int, len(todo.Configurations))
			for ci, _ := range todo.Configurations {
				permute[ci] = ci
			}

			for yyy := 0; yyy < buildCount; yyy++ {
				for bi, bench := range todo.Benchmarks {
					if bench.Disabled {
						continue
					}

					rand.Shuffle(len(permute), func(i, j int) { permute[i], permute[j] = permute[j], permute[i] })

					for ci := range todo.Configurations {
						config := &todo.Configurations[permute[ci]]
						if config.Disabled {
							continue
						}
						s := config.compileOne(&todo.Benchmarks[bi], dirs.wd, yyy)
						if s != "" {
							getAndBuildFailures = append(getAndBuildFailures, s)
						}
					}
				}
			}
		case 2: // N times, shuffle combination of benchmarks and configuration, build them all
			permute := make([]pair, len(todo.Configurations)*len(todo.Benchmarks))
			i := 0
			for bi := range todo.Benchmarks {
				for ci := range todo.Configurations {
					permute[i] = pair{b: bi, c: ci}
					i++
				}
			}

			for yyy := 0; yyy < buildCount; yyy++ {
				rand.Shuffle(len(permute), func(i, j int) { permute[i], permute[j] = permute[j], permute[i] })
				for _, p := range permute {
					bench := &todo.Benchmarks[p.b]
					config := &todo.Configurations[p.c]
					if bench.Disabled || config.Disabled {
						continue
					}
					s := config.compileOne(bench, dirs.wd, yyy)
					if s != "" {
						getAndBuildFailures = append(getAndBuildFailures, s)
					}
				}
			}

		case 3: // Shuffle all the N copies of all the benchmark and configuration pairs, build them all.
			permute := make([]triple, buildCount*len(todo.Configurations)*len(todo.Benchmarks))
			i := 0
			for k := 0; k < buildCount; k++ {
				for bi := range todo.Benchmarks {
					for ci := range todo.Configurations {
						permute[i] = triple{b: bi, c: ci, k: k}
						i++
					}
				}
			}
			rand.Shuffle(len(permute), func(i, j int) { permute[i], permute[j] = permute[j], permute[i] })

			for _, p := range permute {
				bench := &todo.Benchmarks[p.b]
				config := &todo.Configurations[p.c]
				if bench.Disabled || config.Disabled {
					continue
				}
				s := config.compileOne(bench, dirs.wd, p.k)
				if s != "" {
					getAndBuildFailures = append(getAndBuildFailures, s)
				}
			}
		}

		if verbose == 0 {
			fmt.Println()
		}

		// As needed, create the sandbox.
		if needSandbox {
			if verbose == 0 {
				fmt.Print("Making sandbox")
			}
			cmd := exec.Command("docker", "build", "-q", ".")
			if verbose > 0 {
				fmt.Println(asCommandLine(dirs.wd, cmd))
			}
			// capture standard output to get container name
			output, err := cmd.Output()
			if err != nil {
				ee := err.(*exec.ExitError)
				fmt.Printf("There was an error running 'docker build', stderr = %s\n", ee.Stderr)
				os.Exit(2)
				return
			}
			// Docker prints stuff AFTER the container, thanks, Docker.
			sc := bufio.NewScanner(bytes.NewReader(output))
			if !sc.Scan() {
				fmt.Printf("Could not scan line from '%s'\n", string(output))
				os.Exit(2)
				return
			}
			container = strings.TrimSpace(sc.Text())
			if verbose == 0 {
				fmt.Println()
			}
			fmt.Printf("Container for sandboxed bench/test runs is %s\n", container)
		}
	} else {
		container = runContainer
		if getOnly { // -r -g is a bit of a no-op, but that's what it implies.
			return
		}
	}

	// Initialize RunDir for benchmarks.
	for i, bench := range todo.Benchmarks {
		if bench.Disabled {
			continue
		}
		// Obtain directory containing testdata, if any:
		// Capture output of "go list -f {{.Dir}} $PKG"

		cmd := exec.Command("go", "list", "-f", "{{.Dir}}", bench.Repo)
		cmd.Env = defaultEnv
		cmd.Dir = dirs.build

		if verbose > 0 {
			fmt.Println(asCommandLine(dirs.wd, cmd))
		} else {
			fmt.Print(".")
		}
		out, err := cmd.Output()
		if err != nil {
			s := fmt.Sprintf(`could not go list -f {{.Dir}} %s, err=%v`, bench.Repo, err)
			fmt.Println(s + "DISABLING benchmark " + bench.Name)
			getAndBuildFailures = append(getAndBuildFailures, s+"("+bench.Name+")\n")
			todo.Benchmarks[i].Disabled = true
			continue
		} else if verbose > 0 {
			fmt.Printf("# Rundir=%s\n", string(out))
		}
		rundir := strings.TrimSpace(string(out))
		if !bench.NotSandboxed {
			// if sandboxed, strip cwd from prefix of rundir.
			rundir = rundir[len(dirs.wd):]
		}
		todo.Benchmarks[i].RunDir = rundir
	}

	var failures []string

	// If there's a bad error running one of the benchmarks, report what we've got, please.
	defer func(t *Todo) {
		for _, config := range todo.Configurations {
			if !config.Disabled { // Don't overwrite if something was disabled.
				config.benchWriter.Close()
			}
		}
		if needSandbox {
			// Print this a second time so it doesn't get missed.
			fmt.Printf("Container for sandboxed bench/test runs is %s\n", container)
		}
		if len(failures) > 0 {
			fmt.Println("FAILURES:")
			for _, f := range failures {
				fmt.Println(f)
			}
		}
		if len(getAndBuildFailures) > 0 {
			fmt.Println("Get and build failures:")
			for _, f := range getAndBuildFailures {
				fmt.Println(f)
			}
		}
	}(todo)

	maxrc := 0

	// N repetitions for each configurationm, run all the benchmarks.
	// TODO randomize the benchmarks and configurations, like for builds.
	for i := 0; i < N; i++ {
		// For each configuration, run all the benchmarks.
		for j, config := range todo.Configurations {
			if config.Disabled {
				continue
			}

			for _, b := range todo.Benchmarks {
				if b.Disabled {
					continue
				}

				root := config.Root

				wrapperPrefix := "/"
				if b.NotSandboxed {
					wrapperPrefix = dirs.wd + "/"
				}
				wrapperFor := func(s []string) string {
					x := ""
					if len(s) > 0 {
						// If not an explicit path, then make it an explicit path
						x = s[0]
						if x[0] != '/' {
							x = wrapperPrefix + x
						}
					}
					return x
				}

				configWrapper := wrapperFor(config.RunWrapper)
				benchWrapper := wrapperFor(b.RunWrapper)

				testBinaryName := config.benchName(&b)
				var s string
				var rc int

				var wrappersAndBin []string

				if configWrapper != "" {
					wrappersAndBin = append(wrappersAndBin, configWrapper)
					wrappersAndBin = append(wrappersAndBin, config.RunWrapper[1:]...)
				}
				if benchWrapper != "" {
					wrappersAndBin = append(wrappersAndBin, benchWrapper)
					wrappersAndBin = append(wrappersAndBin, b.RunWrapper[1:]...)
				}

				if b.NotSandboxed {
					bin := path.Join(dirs.wd, dirs.testBinDir, testBinaryName)
					wrappersAndBin = append(wrappersAndBin, bin)

					cmd := exec.Command(wrappersAndBin[0], wrappersAndBin[1:]...)
					cmd.Args = append(cmd.Args, "-test.run="+b.Tests)
					cmd.Args = append(cmd.Args, "-test.bench="+b.Benchmarks)

					cmd.Dir = b.RunDir
					cmd.Env = defaultEnv
					if root != "" {
						cmd.Env = replaceEnv(cmd.Env, "GOROOT", root)
					}
					cmd.Env = replaceEnvs(cmd.Env, config.RunEnv)
					cmd.Env = append(cmd.Env, "BENT_DIR="+dirs.wd)
					cmd.Env = append(cmd.Env, "BENT_PROFILES="+path.Join(dirs.wd, config.thingBenchName("profiles")))
					cmd.Env = append(cmd.Env, "BENT_BINARY="+testBinaryName)
					cmd.Env = append(cmd.Env, "BENT_I="+strconv.FormatInt(int64(i), 10))
					cmd.Args = append(cmd.Args, config.RunFlags...)
					cmd.Args = append(cmd.Args, moreArgs...)

					config.say("shortname: " + b.Name + "\n")
					s, rc = todo.Configurations[j].runBinary(dirs.wd, cmd, false)
				} else {
					// docker run --net=none -e GOROOT=... -w /src/github.com/minio/minio/cmd $D /testbin/cmd_Config.test -test.short -test.run=Nope -test.v -test.bench=Benchmark'(Get|Put|List)'
					// TODO(jfaller): I don't think we need either of these "/" below, investigate...
					bin := "/" + path.Join(dirs.testBinDir, testBinaryName)
					wrappersAndBin = append(wrappersAndBin, bin)

					cmd := exec.Command("docker", "run", "--net=none", "-w", b.RunDir)
					for _, e := range config.RunEnv {
						cmd.Args = append(cmd.Args, "-e", e)
					}
					cmd.Args = append(cmd.Args, "-e", "BENT_DIR=/") // TODO this is not going to work well
					cmd.Args = append(cmd.Args, "-e", "BENT_PROFILES="+path.Join(dirs.wd, config.thingBenchName("profiles")))
					cmd.Args = append(cmd.Args, "-e", "BENT_BINARY="+testBinaryName)
					cmd.Args = append(cmd.Args, "-e", "BENT_I="+strconv.FormatInt(int64(i), 10))
					cmd.Args = append(cmd.Args, container)
					cmd.Args = append(cmd.Args, wrappersAndBin...)
					cmd.Args = append(cmd.Args, "-test.run="+b.Tests)
					cmd.Args = append(cmd.Args, "-test.bench="+b.Benchmarks)
					cmd.Args = append(cmd.Args, config.RunFlags...)
					cmd.Args = append(cmd.Args, moreArgs...)
					config.say("shortname: " + b.Name + "\n")
					s, rc = todo.Configurations[j].runBinary(dirs.wd, cmd, false)
				}
				if s != "" {
					fmt.Println(s)
					failures = append(failures, s)
				}
				if rc > maxrc {
					maxrc = rc
				}
			}
		}
	}
	if maxrc > 0 {
		os.Exit(maxrc)
	}
}

func escape(s string) string {
	s = strings.Replace(s, "\\", "\\\\", -1)
	s = strings.Replace(s, "'", "\\'", -1)
	// Conservative guess at characters that will force quoting
	if strings.ContainsAny(s, "\\ ;#*&$~?!|[]()<>{}`") {
		s = " '" + s + "'"
	} else {
		s = " " + s
	}
	return s
}

// asCommandLine renders cmd as something that could be copy-and-pasted into a command line
func asCommandLine(cwd string, cmd *exec.Cmd) string {
	s := "("
	if cmd.Dir != "" && cmd.Dir != cwd {
		s += "cd" + escape(cmd.Dir) + ";"
	}
	for _, e := range cmd.Env {
		if !strings.HasPrefix(e, "PATH=") &&
			!strings.HasPrefix(e, "HOME=") &&
			!strings.HasPrefix(e, "USER=") &&
			!strings.HasPrefix(e, "SHELL=") {
			s += escape(e)
		}
	}
	for _, a := range cmd.Args {
		s += escape(a)
	}
	s += " )"
	return s
}

// checkAndSetUpFileSystem does a number of tasks to ensure that the tests will
// run properly. It:
//
//   - Makes sure we're not going to accidentally overwrite previous results
//   - if shouldInit is true, we:
//     - Create a Dockerfile.
//     - Create all the configuration files.
//     - Exit
func checkAndSetUpFileSystem(shouldInit bool) error {
	// To avoid bad surprises, look for pkg and bin, if they exist, refuse to run
	_, derr := os.Stat("Dockerfile")
	_, perr := os.Stat(path.Join("gopath", "pkg"))

	if derr != nil && !shouldInit {
		// Missing Dockerfile
		return errors.New("Missing 'Dockerfile', please rerun with -I (initialize) flag if you intend to use this directory.\n")
	}

	if shuffle < 0 || shuffle > 3 {
		return fmt.Errorf("Shuffle value (-s) ought to be between 0 and 3, inclusive, instead is %d\n", shuffle)
	}

	// Initialize the directory, copying in default benchmarks and sample configurations, and creating a Dockerfile
	if shouldInit {
		if perr == nil {
			fmt.Printf("It looks like you've already initialized this directory, remove ./gopath/pkg if you want to reinit.\n")
			os.Exit(1)
		}
		for _, s := range copyExes {
			copyAsset(scripts, "scripts", s)
			os.Chmod(s, 0755)
		}
		for _, s := range copyConfigs {
			copyAsset(configs, "configs", s)
		}

		err := ioutil.WriteFile("Dockerfile",
			[]byte(`
FROM ubuntu
ADD . /
`), 0664)
		if err != nil {
			return err
		}
		fmt.Printf("Created Dockerfile\n")
		os.Exit(0)
	}
	return nil
}

func copyAsset(fs embed.FS, dir, file string) {
	f, err := fs.Open(path.Join(dir, file))
	if err != nil {
		fmt.Printf("Error opening asset %s\n", file)
		os.Exit(1)
	}
	stat, err := f.Stat()
	if err != nil {
		fmt.Printf("Error reading stats %s\n", file)
		os.Exit(1)
	}
	bytes := make([]byte, stat.Size())
	if l, err := f.Read(bytes); err != nil || l != int(stat.Size()) {
		fmt.Printf("Error reading asset %s\n", file)
		os.Exit(1)
	}
	err = ioutil.WriteFile(file, bytes, 0664)
	if err != nil {
		fmt.Printf("Error writing %s\n", file)
		os.Exit(1)
	}
	fmt.Printf("Copied asset %s to current directory\n", file)
}

type directories struct {
	wd, gopath, goroots, build, testBinDir, benchDir string
}

// createDirectories creates all the directories we need.
func createDirectories(mode fs.FileMode) (*directories, error) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Could not get current working directory %v\n", err)
		os.Exit(1)
	}
	dirs := &directories{
		wd:         cwd,
		gopath:     path.Join(cwd, "gopath"),
		goroots:    path.Join(cwd, "goroots"),
		build:      path.Join(cwd, "build"),
		testBinDir: "testbin",
		benchDir:   "bench",
	}
	for _, d := range []string{dirs.gopath, dirs.goroots, dirs.build, path.Join(cwd, dirs.testBinDir), path.Join(cwd, dirs.benchDir)} {
		if err := os.Mkdir(d, mode); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("error creating %v: %v", d, err)
		}
	}
	return dirs, nil
}

// testBinaryName returns the name of the binary produced by "go test -c"
func (b *Benchmark) testBinaryName() string {
	return b.Repo[strings.LastIndex(b.Repo, "/")+1:] + ".test"
}

// inheritEnv extracts ev from the os environment and
// returns env extended with that new environment variable.
// Does not check if ev already exists in env.
func inheritEnv(env []string, ev string) []string {
	evv := os.Getenv(ev)
	if evv != "" {
		env = append(env, ev+"="+evv)
	}
	return env
}

func getenv(env []string, ev string) string {
	evplus := ev + "="
	for _, v := range env {
		if strings.HasPrefix(v, evplus) {
			return v[len(evplus):]
		}
	}
	return ""
}

// replaceEnv returns a new environment derived from env
// by removing any existing definition of ev and adding ev=evv.
func replaceEnv(env []string, ev string, evv string) []string {
	evplus := ev + "="
	var newenv []string
	for _, v := range env {
		if !strings.HasPrefix(v, evplus) {
			newenv = append(newenv, v)
		}
	}
	newenv = append(newenv, evplus+evv)
	return newenv
}

// replaceEnv returns a new environment derived from env
// by replacing or adding all modifiers in newevs.
func replaceEnvs(env, newevs []string) []string {
	for _, e := range newevs {
		var newenv []string
		eq := strings.IndexByte(e, '=')
		if eq == -1 {
			panic("Bad input to replaceEnvs, ought to be slice of e=v")
		}
		evplus := e[0 : eq+1]
		for _, v := range env {
			if !strings.HasPrefix(v, evplus) {
				newenv = append(newenv, v)
			}
		}
		env = append(newenv, e)
	}
	return env
}

// ifMissingAddEnv returns a new environment derived from env
// by adding ev=evv if env does not define env.
func ifMissingAddEnv(env []string, ev string, evv string) []string {
	evplus := ev + "="
	for _, v := range env {
		if strings.HasPrefix(v, evplus) {
			return env
		}
	}
	env = append(env, evplus+evv)
	return env
}

// csToset converts a comma-separated string into the set of strings between the commas.
func csToSet(s string) map[string]bool {
	if s == "" {
		return nil
	}
	m := make(map[string]bool)
	ss := strings.Split(s, ",")
	for _, sss := range ss {
		m[sss] = true
	}
	return m
}

// counterFlag is a flag.Value that is like a flag.Bool and a flag.Int.
// If used as -name, it increments the counterFlag, but -name=x sets the counterFlag.
// Used for verbose flag -v and build-all flag -a
type counterFlag int

func (c *counterFlag) String() string {
	return fmt.Sprint(int(*c))
}

func (c *counterFlag) Set(s string) error {
	switch s {
	case "true":
		*c++
	case "false":
		*c = 0
	default:
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid count %q", s)
		}
		*c = counterFlag(n)
	}
	return nil
}

func (c *counterFlag) IsBoolFlag() bool {
	return true
}
