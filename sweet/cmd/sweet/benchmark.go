// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
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
		name:        "bleve-query",
		description: "Queries a pre-built search index with keywords",
		harness:     harnesses.BleveQuery(),
		generator:   generators.BleveQuery{},
	},
	{
		name:        "fogleman-fauxgl",
		description: "Renders a rotating boat via an OpenGL-like software rendering pipeline",
		harness:     harnesses.FoglemanFauxGL(),
		generator:   generators.FoglemanFauxGL(),
	},
	{
		name:        "fogleman-pt",
		description: "Renders a Go gopher via path tracing",
		harness:     harnesses.FoglemanPT(),
		generator:   generators.FoglemanPT(),
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

var benchmarkGroups = map[string][]*benchmark{
	"default": {
		allBenchmarksMap["biogo-igor"],
		allBenchmarksMap["biogo-krishna"],
		allBenchmarksMap["bleve-index"],
		allBenchmarksMap["bleve-query"],
		allBenchmarksMap["fogleman-fauxgl"],
		allBenchmarksMap["fogleman-pt"],
		allBenchmarksMap["go-build"],
		allBenchmarksMap["gopher-lua"],
		allBenchmarksMap["gvisor"],
		allBenchmarksMap["markdown"],
		allBenchmarksMap["tile38"],
	},
	"all": func() (b []*benchmark) {
		for i := range allBenchmarks {
			b = append(b, &allBenchmarks[i])
		}
		return
	}(),
}

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
	return fileutil.CopyDir(dst, src)
}

func rmDirContents(dir string) error {
	log.CommandPrintf("rm -rf %s/*", dir)
	fs, err := ioutil.ReadDir(dir)
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
	topAssetsDir := filepath.Join(r.assetsDir, b.name)
	benchDir := filepath.Join(r.benchDir, b.name)
	topDir := filepath.Join(r.workDir, b.name)
	srcDir := filepath.Join(topDir, "src")

	hasAssets, err := fileutil.FileExists(topAssetsDir)
	if err != nil {
		return err
	}

	// Retrieve the benchmark's source.
	if err := b.harness.Get(srcDir); err != nil {
		return fmt.Errorf("retrieving source for %s: %v", b.name, err)
	}

	// Create the results directory for the benchmark.
	resultsDir := filepath.Join(r.resultsDir, b.name)
	if err := mkdirAll(resultsDir); err != nil {
		return fmt.Errorf("creating results directory for %s: %v", b.name, err)
	}

	// Perform a setup step for each config for the benchmark.
	setups := make([]common.RunConfig, 0, len(cfgs))
	for _, cfg := range cfgs {
		// Create directory heirarchy for benchmarks.
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

		// Build the benchmark (application and any other necessary components).
		bcfg := common.BuildConfig{
			BinDir:   binDir,
			SrcDir:   srcDir,
			BenchDir: benchDir,
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
		if r.cpuProfile || r.memProfile || r.perf {
			// Create a directory for any profile files to live in.
			resultsProfilesDir := filepath.Join(resultsDir, fmt.Sprintf("%s.debug", cfg.Name))
			mkdirAll(resultsProfilesDir)

			// We need to pass arguments to the benchmark binary to generate
			// profiles. See benchmarks/internal/driver for details.
			if r.cpuProfile {
				args = append(args, "-cpuprofile", resultsProfilesDir)
			}
			if r.memProfile {
				args = append(args, "-memprofile", resultsProfilesDir)
			}
			if r.perf {
				args = append(args, "-perf", resultsProfilesDir)
				if r.perfFlags != "" {
					args = append(args, "-perf-flags", r.perfFlags)
				}
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
		})
	}

	for j := 0; j < r.count; j++ {
		// Execute the benchmark for each configuration.
		for i, setup := range setups {
			if hasAssets {
				// Set up assets directory for test run.
				if err := copyDirContents(setup.AssetsDir, topAssetsDir); err != nil {
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
