// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/zip"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/benchmarks/sweet/cli/bootstrap"
	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
	"golang.org/x/benchmarks/sweet/common/log"
	sprofile "golang.org/x/benchmarks/sweet/common/profile"

	"github.com/BurntSushi/toml"
	"github.com/google/pprof/profile"
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
files. Note: by default, this command expects to run from /path/to/x/benchmarks/sweet.`
	runUsage = `Usage: %s run [flags] <config> [configs...]
`
)

const (
	countDefault       = 10
	pgoCountDefaultMax = 5
)

type runCfg struct {
	count       int
	resultsDir  string
	benchDir    string
	assetsDir   string
	workDir     string
	assetsCache string
	dumpCore    bool
	pgo         bool
	pgoCount    int
	short       bool

	assetsFS fs.FS
}

func (r *runCfg) logCopyDirCommand(fromRelDir, toDir string) {
	if r.assetsDir == "" {
		assetsFile, _ := bootstrap.CachedAssets(r.assetsCache, common.Version)
		log.CommandPrintf("unzip %s '%s/*' -d %s", assetsFile, fromRelDir, toDir)
	} else {
		log.CommandPrintf("cp -r %s/* %s", filepath.Join(r.assetsDir, fromRelDir), toDir)
	}
}

func (r *runCfg) benchmarkResultsDir(b *benchmark) string {
	return filepath.Join(r.resultsDir, b.name)
}

func (r *runCfg) runProfilesDir(b *benchmark, c *common.Config) string {
	return filepath.Join(r.benchmarkResultsDir(b), fmt.Sprintf("%s.debug", c.Name))
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
	f.StringVar(&c.runCfg.assetsDir, "assets-dir", "", "a directory containing uncompressed assets for sweet benchmarks, usually for debugging Sweet (overrides -cache)")
	f.StringVar(&c.runCfg.workDir, "work-dir", "", "work directory for benchmarks (default: temporary directory)")
	f.StringVar(&c.runCfg.assetsCache, "cache", bootstrap.CacheDefault(), "cache location for assets")
	f.BoolVar(&c.runCfg.dumpCore, "dump-core", false, "whether to dump core files for each benchmark process when it completes a benchmark")
	f.BoolVar(&c.pgo, "pgo", false, "perform PGO testing; for each config, collect profiles from a baseline run which are used to feed into a generated PGO config")
	f.IntVar(&c.runCfg.pgoCount, "pgo-count", 0, "the number of times to run profiling runs for -pgo; defaults to the value of -count if <=5, or 5 if higher")
	f.IntVar(&c.runCfg.count, "count", 0, fmt.Sprintf("the number of times to run each benchmark (default %d)", countDefault))

	f.BoolVar(&c.quiet, "quiet", false, "whether to suppress activity output on stderr (no effect on -shell)")
	f.BoolVar(&c.printCmd, "shell", false, "whether to print the commands being executed to stdout")
	f.BoolVar(&c.stopOnError, "stop-on-error", false, "whether to stop running benchmarks if an error occurs or a benchmark fails")
	f.BoolVar(&c.short, "short", false, "whether to run a short version of the benchmarks for testing (changes -count to 1)")
	f.Var(&c.toRun, "run", "benchmark group or comma-separated list of benchmarks to run")
}

func (c *runCmd) Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("at least one configuration is required")
	}
	checkPlatform()

	log.SetCommandTrace(c.printCmd)
	log.SetActivityLog(!c.quiet)

	if c.runCfg.count == 0 {
		if c.short {
			c.runCfg.count = 1
		} else {
			c.runCfg.count = countDefault
		}
	}
	if c.runCfg.pgoCount == 0 {
		c.runCfg.pgoCount = c.runCfg.count
		if c.runCfg.pgoCount > pgoCountDefaultMax {
			c.runCfg.pgoCount = pgoCountDefaultMax
		}
	}

	var err error
	if c.workDir == "" {
		// Create a temporary work tree for running the benchmarks.
		c.workDir, err = os.MkdirTemp("", "gosweet")
		if err != nil {
			return fmt.Errorf("creating work root: %w", err)
		}
	}
	// Ensure all provided directories are absolute paths. This avoids problems with
	// benchmarks potentially changing their current working directory.
	c.workDir, err = filepath.Abs(c.workDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from provided work root (-work-dir): %w", err)
	}
	c.benchDir, err = filepath.Abs(c.benchDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from benchmarks path (-bench-dir): %w", err)
	}
	c.resultsDir, err = filepath.Abs(c.resultsDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from results path (-results): %w", err)
	}
	if c.assetsDir != "" {
		c.assetsDir, err = filepath.Abs(c.assetsDir)
		if err != nil {
			return fmt.Errorf("creating absolute path from assets path (-assets-dir): %w", err)
		}
		if info, err := os.Stat(c.assetsDir); os.IsNotExist(err) {
			return fmt.Errorf("assets not found at %q: did you forget to run `sweet get`?", c.assetsDir)
		} else if err != nil {
			return fmt.Errorf("stat assets %q: %v", c.assetsDir, err)
		} else if info.Mode()&os.ModeDir == 0 {
			return fmt.Errorf("%q is not a directory", c.assetsDir)
		}
		c.assetsFS = os.DirFS(c.assetsDir)
	} else {
		if c.assetsCache == "" {
			return fmt.Errorf("missing assets cache (-cache) and assets directory (-assets-dir): cannot proceed without assets")
		}
		c.assetsCache, err = filepath.Abs(c.assetsCache)
		if err != nil {
			return fmt.Errorf("creating absolute path from assets cache path (-cache): %w", err)
		}
		if info, err := os.Stat(c.assetsCache); os.IsNotExist(err) {
			return fmt.Errorf("assets not found at %q (-assets-dir): did you forget to run `sweet get`?", c.assetsDir)
		} else if err != nil {
			return fmt.Errorf("stat assets %q: %v", c.assetsDir, err)
		} else if info.Mode()&os.ModeDir == 0 {
			return fmt.Errorf("%q (-assets-dir) is not a directory", c.assetsDir)
		}
		assetsFile, err := bootstrap.CachedAssets(c.assetsCache, common.Version)
		if err == bootstrap.ErrNotInCache {
			return fmt.Errorf("assets for version %q not found in %q", common.Version, c.assetsCache)
		} else if err != nil {
			return err
		}
		f, err := os.Open(assetsFile)
		if err != nil {
			return err
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		c.assetsFS, err = zip.NewReader(f, fi.Size())
		if err != nil {
			return err
		}
	}
	// Validate c.benchDir and provide helpful error messages..
	if fi, err := os.Stat(c.benchDir); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("benchmarks directory (-bench-dir) does not exist; did you mean to run this command from x/benchmarks/sweet?")
	} else if err != nil {
		return fmt.Errorf("checking benchmarks directory (-bench-dir): %w", err)
	} else {
		if !fi.IsDir() {
			return fmt.Errorf("-bench-dir is not a directory; did you mean to run this command from x/benchmarks/sweet?")
		}
		var missing []string
		for _, b := range allBenchmarks {
			fi, err := os.Stat(filepath.Join(c.benchDir, b.name))
			if err != nil || !fi.IsDir() {
				missing = append(missing, b.name)
			}
		}
		if len(missing) != 0 {
			return fmt.Errorf("benchmarks directory (-bench-dir) is missing benchmarks (%s); did you mean to run this command from x/benchmarks/sweet?", strings.Join(missing, ", "))
		}
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
		b, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read %q: %v", configFile, err)
		}
		var fconfigs common.ConfigFile
		md, err := toml.Decode(string(b), &fconfigs)
		if err != nil {
			return fmt.Errorf("failed to parse %q: %v", configFile, err)
		}
		if len(md.Undecoded()) != 0 {
			return fmt.Errorf("unexpected keys in %q: %+v", configFile, md.Undecoded())
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
			if config.PGOFiles == nil {
				config.PGOFiles = make(map[string]string)
			}
			for k := range config.PGOFiles {
				if _, ok := allBenchmarksMap[k]; !ok {
					return fmt.Errorf("config %q in %q pgofiles references unknown benchmark %q", config.Name, configFile, k)
				}
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

	// Print an indication of how many runs will be done.
	countString := fmt.Sprintf("%d runs", c.runCfg.count*len(configs))
	if c.pgo {
		countString += fmt.Sprintf(", %d pgo runs", c.runCfg.pgoCount*len(configs))
	}
	log.Printf("Benchmarks: %s (%s)", strings.Join(benchmarkNames(benchmarks), " "), countString)

	// Check prerequisites for each benchmark.
	for _, b := range benchmarks {
		if err := b.harness.CheckPrerequisites(); err != nil {
			return fmt.Errorf("failed to meet prerequisites for %s: %v", b.name, err)
		}
	}

	// Collect profiles from baseline runs and create new PGO'd configs.
	if c.pgo {
		configs, err = c.preparePGO(configs, benchmarks)
		if err != nil {
			return fmt.Errorf("error preparing PGO profiles: %w", err)
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
			log.Error(err)
		}
	}
	if errEncountered {
		return fmt.Errorf("failed to execute benchmarks, see log for details")
	}
	return nil
}

func (c *runCmd) preparePGO(configs []*common.Config, benchmarks []*benchmark) ([]*common.Config, error) {
	profileConfigs := make([]*common.Config, 0, len(configs))
	for _, c := range configs {
		cc := c.Copy()
		cc.Name += ".profile"
		cc.Diagnostics.Set(diagnostics.Config{Type: diagnostics.CPUProfile})
		profileConfigs = append(profileConfigs, cc)
	}

	profileRunCfg := c.runCfg
	profileRunCfg.count = profileRunCfg.pgoCount

	log.Printf("Running profile collection runs")

	// Execute benchmarks to collect profiles.
	var errEncountered bool
	for _, b := range benchmarks {
		if err := b.execute(profileConfigs, &profileRunCfg); err != nil {
			if c.stopOnError {
				return nil, err
			}
			errEncountered = true
			log.Error(err)
		}
	}
	if errEncountered {
		return nil, fmt.Errorf("failed to execute profile collection benchmarks, see log for details")
	}

	// Merge all the profiles and add new PGO configs.
	newConfigs := configs
	for i := range configs {
		origConfig := configs[i]
		profileConfig := profileConfigs[i]
		pgoConfig := origConfig.Copy()
		pgoConfig.Name += ".pgo"
		pgoConfig.PGOFiles = make(map[string]string)

		for _, b := range benchmarks {
			p, err := mergeCPUProfiles(profileRunCfg.runProfilesDir(b, profileConfig))
			if err != nil {
				return nil, fmt.Errorf("error merging profiles for %s/%s: %w", b.name, profileConfig.Name, err)
			}
			pgoConfig.PGOFiles[b.name] = p
		}

		newConfigs = append(newConfigs, pgoConfig)
	}

	return newConfigs, nil
}

var cpuProfileRe = regexp.MustCompile(`^.*\.cpuprofile[0-9]+$`)

func mergeCPUProfiles(dir string) (string, error) {
	profiles, err := sprofile.ReadDirPprof(dir, func(name string) bool {
		return cpuProfileRe.FindString(name) != ""
	})
	if err != nil {
		return "", fmt.Errorf("error reading dir %q: %w", dir, err)
	}
	if len(profiles) == 0 {
		return "", fmt.Errorf("no profiles found in %q", dir)
	}

	p, err := profile.Merge(profiles)
	if err != nil {
		return "", fmt.Errorf("error merging profiles: %w", err)
	}

	out := filepath.Join(dir, "merged.cpu")
	f, err := os.Create(out)
	defer f.Close()

	if err := p.Write(f); err != nil {
		return "", fmt.Errorf("error writing merged profile: %w", err)
	}

	return out, nil
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
