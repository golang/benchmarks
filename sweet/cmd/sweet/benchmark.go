// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/fileutil"
	"golang.org/x/benchmarks/sweet/common/log"
	"golang.org/x/benchmarks/sweet/generators"
	"golang.org/x/benchmarks/sweet/harnesses"
)

var allBenchmarks = []benchmark{
	{
		name:        "biogo-igor",
		description: "Reports feature family groupings in pairwise alignment data",
		harness:     harnesses.BiogoIgor(),
		generator:   generators.BiogoIgor(),
	},
	{
		name:        "biogo-krishna",
		description: "Performs pairwise alignment of a target sequence against itself",
		harness:     harnesses.BiogoKrishna(),
		generator:   generators.BiogoKrishna(),
	},
	{
		name:        "bleve-index",
		description: "Indexes a subset of Wikipedia into a search index",
		harness:     harnesses.BleveIndex(),
		generator:   generators.BleveIndex(),
	},
	{
		name:        "etcd",
		description: "Distributed key-value store",
		harness:     harnesses.Etcd{},
		generator:   generators.None{},
	},
	{
		name:        "go-build",
		description: "Go build command",
		harness:     harnesses.GoBuild{},
		generator:   generators.None{},
	},
	{
		name:        "gopher-lua",
		description: "Runs a k-nucleotide benchmark written in Lua on a Go-based Lua VM",
		harness:     harnesses.GopherLua(),
		generator:   generators.GopherLua(),
	},
	{
		name:        "gvisor",
		description: "Container runtime sandbox for Linux (requires root)",
		harness:     harnesses.GVisor{},
		generator:   generators.GVisor{},
	},
	{
		name:        "markdown",
		description: "Renders a corpus of markdown documents to XHTML",
		harness:     harnesses.Markdown(),
		generator:   generators.Markdown(),
	},
	{
		name:        "tile38",
		description: "Redis-like geospatial database and geofencing server",
		harness:     harnesses.Tile38{},
		generator:   generators.Tile38{},
	},
}

var allBenchmarksMap = func() map[string]*benchmark {
	m := make(map[string]*benchmark)
	for i := range allBenchmarks {
		m[allBenchmarks[i].name] = &allBenchmarks[i]
	}
	return m
}()

var benchmarkGroups = func() map[string][]*benchmark {
	m := make(map[string][]*benchmark)

	m["default"] = []*benchmark{
		allBenchmarksMap["biogo-igor"],
		allBenchmarksMap["biogo-krishna"],
		allBenchmarksMap["bleve-index"],
		allBenchmarksMap["etcd"],
		allBenchmarksMap["go-build"],
		allBenchmarksMap["gopher-lua"],
		// TODO(go.dev/issue/51445): Enable once gVisor builds with Go 1.19.
		// allBenchmarksMap["gvisor"],
		allBenchmarksMap["markdown"],
		allBenchmarksMap["tile38"],
	}

	for i := range allBenchmarks {
		switch allBenchmarks[i].name {
		case "gvisor":
			// TODO(go.dev/issue/51445): Include in "all"
			// once gVisor builds with Go 1.19.
			continue
		}
		m["all"] = append(m["all"], &allBenchmarks[i])
	}

	return m
}()

func benchmarkNames(b []*benchmark) (s []string) {
	for _, bench := range b {
		s = append(s, bench.name)
	}
	return
}

func mkdirAll(path string) error {
	log.CommandPrintf("mkdir -p %s", path)
	return os.MkdirAll(path, os.ModePerm)
}

func copyDirContents(dst, src string) error {
	log.CommandPrintf("cp -r %s/* %s", src, dst)
	return fileutil.CopyDir(dst, src, nil)
}

func rmDirContents(dir string) error {
	log.CommandPrintf("rm -rf %s/*", dir)
	fs, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, fi := range fs {
		if err := os.RemoveAll(filepath.Join(dir, fi.Name())); err != nil {
			return err
		}
	}
	return nil
}

type benchmark struct {
	name        string
	description string
	harness     common.Harness
	generator   common.Generator
}

func (b *benchmark) execute(cfgs []*common.Config, r *runCfg) error {
	log.Printf("Setting up benchmark: %s", b.name)

	// Compute top-level directories for this benchmark to work in.
	benchDir := filepath.Join(r.benchDir, b.name)
	topDir := filepath.Join(r.workDir, b.name)
	srcDir := filepath.Join(topDir, "src")

	// Check if assets for this benchmark exist. Not all benchmarks have assets!
	var hasAssets bool
	assetsFSDir := b.name
	if f, err := r.assetsFS.Open(assetsFSDir); err == nil {
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		if !fi.IsDir() {
			f.Close()
			return fmt.Errorf("found assets file for %s instead of directory", b.name)
		}
		f.Close()
		hasAssets = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Retrieve the benchmark's source, if needed. If execute is called
	// multiple times, this will already be done.
	_, err := os.Stat(srcDir)
	if os.IsNotExist(err) {
		gcfg := &common.GetConfig{
			SrcDir: srcDir,
			Short:  r.short,
		}
		if err := b.harness.Get(gcfg); err != nil {
			return fmt.Errorf("retrieving source for %s: %v", b.name, err)
		}
	}

	// Create the results directory for the benchmark.
	resultsDir := r.benchmarkResultsDir(b)
	if err := mkdirAll(resultsDir); err != nil {
		return fmt.Errorf("creating results directory for %s: %v", b.name, err)
	}

	// Perform a setup step for each config for the benchmark.
	setups := make([]common.RunConfig, 0, len(cfgs))
	for _, pcfg := range cfgs {
		// Local copy for per-benchmark environment adjustments.
		cfg := pcfg.Copy()

		// Create directory hierarchy for benchmarks.
		workDir := filepath.Join(topDir, cfg.Name)
		binDir := filepath.Join(workDir, "bin")
		tmpDir := filepath.Join(workDir, "tmp")
		assetsDir := filepath.Join(workDir, "assets")
		if err := mkdirAll(binDir); err != nil {
			return fmt.Errorf("create %s bin for %s: %v", b.name, cfg.Name, err)
		}
		if err := mkdirAll(srcDir); err != nil {
			return fmt.Errorf("create %s src for %s: %v", b.name, cfg.Name, err)
		}
		if err := mkdirAll(tmpDir); err != nil {
			return fmt.Errorf("create %s tmp for %s: %v", b.name, cfg.Name, err)
		}
		if hasAssets {
			if err := mkdirAll(assetsDir); err != nil {
				return fmt.Errorf("create %s assets dir for %s: %v", b.name, cfg.Name, err)
			}
		}

		// Add PGO if profile specified for this benchmark.
		if pgo, ok := cfg.PGOFiles[b.name]; ok {
			goflags, ok := cfg.BuildEnv.Lookup("GOFLAGS")
			if ok {
				goflags += " "
			}
			goflags += fmt.Sprintf("-pgo=%s", pgo)
			cfg.BuildEnv.Env = cfg.BuildEnv.MustSet("GOFLAGS=" + goflags)
		}

		// Build the benchmark (application and any other necessary components).
		bcfg := common.BuildConfig{
			BinDir:   binDir,
			SrcDir:   srcDir,
			BenchDir: benchDir,
			Short:    r.short,
		}
		if err := b.harness.Build(cfg, &bcfg); err != nil {
			return fmt.Errorf("build %s for %s: %v", b.name, cfg.Name, err)
		}

		// Generate any args to funnel through to benchmarks.
		args := []string{}
		if r.dumpCore {
			// Create a directory for the core files to live in.
			resultsCoresDir := filepath.Join(resultsDir, "core")
			mkdirAll(resultsCoresDir)
			// We need to pass an argument to the benchmark binary to generate
			// a core file. See benchmarks/internal/driver for details.
			args = append(args, "-dump-cores", resultsCoresDir)
			// Copy the bin directory so that the binaries may be used to analyze
			// the core dump.
			resultsBinDir := filepath.Join(resultsDir, "bin")
			mkdirAll(resultsBinDir)
			copyDirContents(resultsBinDir, binDir)
		}
		if !cfg.Diagnostics.Empty() {
			// Create a directory for any profile files to live in.
			resultsProfilesDir := r.runProfilesDir(b, cfg)
			mkdirAll(resultsProfilesDir)

			// We need to pass arguments to the benchmark binary to generate
			// profiles. See benchmarks/internal/driver for details.
			for _, d := range cfg.Diagnostics.ToSlice() {
				args = append(args, d.DriverArgs(resultsProfilesDir)...)
			}
		}

		results, err := os.Create(filepath.Join(resultsDir, fmt.Sprintf("%s.results", cfg.Name)))
		if err != nil {
			return fmt.Errorf("create %s results file for %s: %v", b.name, cfg.Name, err)
		}
		defer results.Close()
		setups = append(setups, common.RunConfig{
			BinDir:    binDir,
			TmpDir:    tmpDir,
			AssetsDir: assetsDir,
			Args:      args,
			Results:   results,
			Short:     r.short,
		})
	}

	for j := 0; j < r.count; j++ {
		// Execute the benchmark for each configuration.
		for i, setup := range setups {
			if hasAssets {
				// Set up assets directory for test run.
				r.logCopyDirCommand(b.name, setup.AssetsDir)
				if err := fileutil.CopyDir(setup.AssetsDir, assetsFSDir, r.assetsFS); err != nil {
					return err
				}
			}

			log.Printf("Running benchmark %s for %s: run %d", b.name, cfgs[i].Name, j+1)
			// Force a GC now because we're about to turn it off.
			runtime.GC()
			// Hold your breath: we're turning off GC for the duration of the
			// run so that the suite's GC doesn't start blasting on all Ps,
			// introducing undue noise into the experiments.
			gogc := debug.SetGCPercent(-1)
			if err := b.harness.Run(cfgs[i], &setup); err != nil {
				debug.SetGCPercent(gogc)
				setup.Results.Close()
				return fmt.Errorf("run benchmark %s for config %s: %v", b.name, cfgs[i].Name, err)
			}
			debug.SetGCPercent(gogc)

			// Clean up tmp directory so benchmarks may assume it's empty.
			if err := rmDirContents(setup.TmpDir); err != nil {
				return err
			}
			if hasAssets {
				// Clean up assets directory just in case any of the files were written to.
				if err := rmDirContents(setup.AssetsDir); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
