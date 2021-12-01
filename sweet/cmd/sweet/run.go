// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"

	"github.com/BurntSushi/toml"
)

type csvFlag []string

func (c *csvFlag) String() string {
	return strings.Join([]string(*c), ",")
}

func (c *csvFlag) Set(input string) error {
	*c = strings.Split(input, ",")
	return nil
}

const (
	runLongDesc = `Execute benchmarks in the suite against GOROOTs provided in TOML configuration
files.`
	runUsage = `Usage: %s run [flags] <config> [configs...]
`
)

type runCfg struct {
	count      int
	resultsDir string
	benchDir   string
	assetsDir  string
	workDir    string
	dumpCore   bool
	cpuProfile bool
	memProfile bool
	perf       bool
	perfFlags  string
}

type runCmd struct {
	runCfg
	quiet       bool
	printCmd    bool
	stopOnError bool
	toRun       csvFlag
}

func (*runCmd) Name() string     { return "run" }
func (*runCmd) Synopsis() string { return "Executes benchmarks in the suite." }
func (*runCmd) PrintUsage(w io.Writer, base string) {
	// Print header.
	fmt.Fprintln(w, runLongDesc)

	// Print supported benchmarks.
	fmt.Fprintln(w, "\nSupported benchmarks:")
	maxBenchNameLen := 0
	for _, b := range allBenchmarks {
		l := utf8.RuneCountInString(b.name)
		if l > maxBenchNameLen {
			maxBenchNameLen = l
		}
	}
	for _, b := range allBenchmarks {
		fmt.Fprintf(w, fmt.Sprintf("  %%%ds: %%s\n", maxBenchNameLen), b.name, b.description)
	}

	// Print benchmark groups.
	fmt.Fprintln(w, "\nBenchmark groups:")
	maxGroupNameLen := 0
	var groups []string
	for groupName := range benchmarkGroups {
		l := utf8.RuneCountInString(groupName)
		if l > maxGroupNameLen {
			maxGroupNameLen = l
		}
		groups = append(groups, groupName)
	}
	sort.Strings(groups)
	for _, group := range groups {
		var groupBenchNames []string
		if group == "all" {
			groupBenchNames = []string{"all supported benchmarks"}
		} else {
			groupBenchNames = benchmarkNames(benchmarkGroups[group])
		}
		fmt.Fprintf(w, fmt.Sprintf("  %%%ds: %%s\n", maxGroupNameLen), group, strings.Join(groupBenchNames, " "))
	}

	// Print configuration format information.
	fmt.Fprintf(w, common.ConfigHelp)
	fmt.Fprintln(w)

	// Print usage line. Flags will automatically be added after.
	fmt.Fprintf(w, runUsage, base)
}

func (c *runCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.runCfg.resultsDir, "results", "./results", "location to write benchmark results to")
	f.StringVar(&c.runCfg.benchDir, "bench-dir", "./benchmarks", "the benchmarks directory in the sweet source")
	f.StringVar(&c.runCfg.assetsDir, "assets-dir", "./assets", "the directory containing assets for sweet benchmarks")
	f.StringVar(&c.runCfg.workDir, "work-dir", "", "work directory for benchmarks (default: temporary directory)")
	f.BoolVar(&c.runCfg.dumpCore, "dump-core", false, "whether to dump core files for each benchmark process when it completes a benchmark")
	f.BoolVar(&c.runCfg.cpuProfile, "cpuprofile", false, "whether to dump a CPU profile for each benchmark (ensures all benchmarks do the same amount of work)")
	f.BoolVar(&c.runCfg.memProfile, "memprofile", false, "whether to dump a memory profile for each benchmark (ensures all executions do the same amount of work")
	f.BoolVar(&c.runCfg.perf, "perf", false, "whether to run each benchmark under Linux perf and dump the results")
	f.StringVar(&c.runCfg.perfFlags, "perf-flags", "", "the flags to pass to Linux perf if -perf is set")
	f.IntVar(&c.runCfg.count, "count", 25, "the number of times to run each benchmark")

	f.BoolVar(&c.quiet, "quiet", false, "whether to suppress activity output on stderr (no effect on -shell)")
	f.BoolVar(&c.printCmd, "shell", false, "whether to print the commands being executed to stdout")
	f.BoolVar(&c.stopOnError, "stop-on-error", false, "whether to stop running benchmarks if an error occurs or a benchmark fails")
	f.Var(&c.toRun, "run", "benchmark group or comma-separated list of benchmarks to run")
}

func (c *runCmd) Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("at least one configuration is required")
	}
	checkPlatform()

	log.SetCommandTrace(c.printCmd)
	log.SetActivityLog(!c.quiet)

	var err error
	if c.workDir == "" {
		// Create a temporary work tree for running the benchmarks.
		c.workDir, err = ioutil.TempDir("", "gosweet")
		if err != nil {
			return fmt.Errorf("creating work root: %v", err)
		}
	}
	// Ensure all provided directories are absolute paths. This avoids problems with
	// benchmarks potentially changing their current working directory.
	c.workDir, err = filepath.Abs(c.workDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from provided work root: %v", err)
	}
	c.assetsDir, err = filepath.Abs(c.assetsDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from assets path: %v", err)
	}
	c.benchDir, err = filepath.Abs(c.benchDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from benchmarks path: %v", err)
	}
	c.resultsDir, err = filepath.Abs(c.resultsDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from results path: %v", err)
	}

	// Make sure the assets directory is there.
	if info, err := os.Stat(c.assetsDir); os.IsNotExist(err) {
		return fmt.Errorf("assets not found at %q: forgot to run `sweet get`?", c.assetsDir)
	} else if err != nil {
		return fmt.Errorf("stat assets %q: %v", c.assetsDir, err)
	} else if info.Mode()&os.ModeDir == 0 {
		return fmt.Errorf("%q is not a directory", c.assetsDir)
	}
	log.Printf("Work directory: %s", c.workDir)

	// Parse and validate all input TOML configs.
	configs := make([]*common.Config, 0, len(args))
	names := make(map[string]struct{})
	for _, configFile := range args {
		// Make the configuration file path absolute relative to the CWD.
		configFile, err := filepath.Abs(configFile)
		if err != nil {
			return fmt.Errorf("failed to absolutize %q: %v", configFile, err)
		}
		configDir := filepath.Dir(configFile)

		// Read and parse the configuration file.
		b, err := ioutil.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read %q: %v", configFile, err)
		}
		var fconfigs common.ConfigFile
		if err := toml.Unmarshal(b, &fconfigs); err != nil {
			return fmt.Errorf("failed to parse %q: %v", configFile, err)
		}
		// Validate each config and append to central list.
		for _, config := range fconfigs.Configs {
			if config.Name == "" {
				return fmt.Errorf("config in %q is missing a name", configFile)
			}
			if _, ok := names[config.Name]; ok {
				return fmt.Errorf("name of config in %q is not unique: %s", configFile, config.Name)
			}
			names[config.Name] = struct{}{}
			if config.GoRoot == "" {
				return fmt.Errorf("config %q in %q is missing a goroot", config.Name, configFile)
			}
			if strings.Contains(config.GoRoot, "~") {
				return fmt.Errorf("path containing ~ found in config %q; feature not supported since v0.1.0", config.Name)
			}
			config.GoRoot = canonicalizePath(config.GoRoot, configDir)
			if config.BuildEnv.Env == nil {
				config.BuildEnv.Env = common.NewEnvFromEnviron()
			}
			if config.ExecEnv.Env == nil {
				config.ExecEnv.Env = common.NewEnvFromEnviron()
			}
			configs = append(configs, config)
		}
	}

	// Decide which benchmarks to run, based on the -run flag.
	var benchmarks []*benchmark
	var unknown []string
	switch len(c.toRun) {
	case 0:
		benchmarks = benchmarkGroups["default"]
	case 1:
		if grp, ok := benchmarkGroups[c.toRun[0]]; ok {
			benchmarks = grp
			break
		}
		fallthrough
	default:
		for _, name := range c.toRun {
			if benchmark, ok := allBenchmarksMap[name]; ok {
				benchmarks = append(benchmarks, benchmark)
			} else {
				unknown = append(unknown, name)
			}
		}
	}
	if len(unknown) != 0 {
		return fmt.Errorf("unknown benchmarks: %s", strings.Join(unknown, ", "))
	}
	log.Printf("Benchmarks: %s", strings.Join(benchmarkNames(benchmarks), " "))

	// Check prerequisites for each benchmark.
	for _, b := range benchmarks {
		if err := b.harness.CheckPrerequisites(); err != nil {
			return fmt.Errorf("failed to meet prerequisites for %s: %v", b.name, err)
		}
	}

	// Execute each benchmark for all configs.
	var errEncountered bool
	for _, b := range benchmarks {
		if err := b.execute(configs, &c.runCfg); err != nil {
			if c.stopOnError {
				return err
			}
			errEncountered = true
			log.Printf("error: %v\n", err)
		}
	}
	if errEncountered {
		return fmt.Errorf("failed to execute benchmarks, see log for details")
	}
	return nil
}

func canonicalizePath(path, base string) string {
	if filepath.IsAbs(path) {
		return path
	}
	path = filepath.Join(base, path)
	return filepath.Clean(path)
}

func checkPlatform() {
	currentPlatform := common.CurrentPlatform()
	platformOK := false
	for _, platform := range common.SupportedPlatforms {
		if currentPlatform == platform {
			platformOK = true
			break
		}
	}
	if !platformOK {
		log.Printf("warning: %s is an unsupported platform, use at your own risk!", currentPlatform)
	}
}
