// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"debug/buildinfo"
	"fmt"
	"go/version"
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/fileutil"
	"golang.org/x/benchmarks/sweet/common/log"
)

type buildBenchmark struct {
	name  string
	pkg   string
	minGo string // skip if toolchain is older than this
	clone func(outDir string) error
}

var (
	buildBenchmarks = []*buildBenchmark{
		{
			name: "kubernetes",
			pkg:  "cmd/kubelet",
			clone: func(outDir string) error {
				return gitShallowClone(
					outDir,
					"https://github.com/kubernetes/kubernetes",
					"v1.22.1",
				)
			},
		},
		{
			name: "istio",
			pkg:  "istioctl/cmd/istioctl",
			clone: func(outDir string) error {
				return gitShallowClone(
					outDir,
					"https://github.com/istio/istio",
					"1.11.1",
				)
			},
		},
		{
			name: "pkgsite",
			pkg:  "cmd/frontend",
			clone: func(outDir string) error {
				return gitCloneToCommit(
					outDir,
					"https://go.googlesource.com/pkgsite",
					"master",
					"0a8194a898a1ceff6a0b29e3419650daf43d8567",
				)
			},
		},
		{
			// Added for #72815. This codebase has at least a few packages
			// that are difficult for the Go compiler to handle, performance-wise,
			// as of Mar. 13 2025.
			name:  "tsgo",
			pkg:   "cmd/tsgo",
			minGo: "go1.24",
			clone: func(outDir string) error {
				return gitCloneToCommit(
					outDir,
					"https://github.com/microsoft/typescript-go",
					"main",
					"1fffa1c05909adddbf2db7e14afeb8f63ed1e12c",
				)
			},
		},
	}
	// For short mode, only build pkgsite. It's the smallest of
	// the set, and it's hosted on go.googlesource.com, so fetching
	// source is less likely to be rate-limited causing CI failures.
	buildBenchmarksShort = []*buildBenchmark{buildBenchmarks[2]}
)

type GoBuild struct{}

func (h GoBuild) CheckPrerequisites() error {
	return nil
}

func (h GoBuild) Get(gcfg *common.GetConfig) error {
	benchmarks, err := goBuildBenchmarks(nil, gcfg.Short)
	if err != nil {
		return fmt.Errorf("error getting benchmark list: %v", err)
	}

	// Clone the sources that we're going to build.
	for _, bench := range benchmarks {
		if err := bench.clone(filepath.Join(gcfg.SrcDir, bench.name)); err != nil {
			return err
		}
	}
	return nil
}

func (h GoBuild) Build(pcfg *common.Config, bcfg *common.BuildConfig) error {
	// Local copy of config for updating GOROOT.
	cfg := pcfg.Copy()

	// cfg.GoRoot is our source toolchain. We need to rebuild cmd/compile
	// and cmd/link with cfg.BuildEnv to apply any configured build options
	// (e.g., PGO).
	//
	// Do so by `go install`ing them into a copied GOROOT.
	goroot := filepath.Join(bcfg.BinDir, "goroot")
	if err := fileutil.CopyDir(goroot, cfg.GoRoot, nil); err != nil {
		return fmt.Errorf("error copying GOROOT: %v", err)
	}
	cfg.GoRoot = goroot
	if err := cfg.GoTool().Do("", "install", "cmd/compile", "cmd/link"); err != nil {
		return fmt.Errorf("error building cmd/compile and cmd/link: %v", err)
	}

	// Get the benchmarks we're going to build.
	benchmarks, err := goBuildBenchmarks(cfg, bcfg.Short)
	if err != nil {
		return fmt.Errorf("error getting benchmark list: %v", err)
	}

	for _, bench := range benchmarks {
		// Generate a symlink to the repository and put it in bin.
		// It's not a binary, but it's the only place we can put it
		// and still access it in Run.
		link := filepath.Join(bcfg.BinDir, bench.name)
		err := symlink(link, filepath.Join(bcfg.SrcDir, bench.name))
		if err != nil {
			return err
		}
		if bcfg.Short {
			// Short mode isn't intended to produce good benchmark results;
			// it's meant for testing and debugging. Skip the additional build step.
			continue
		}
		// Build the benchmark once, pulling in any requisite packages.
		//
		// Run the go tool with ExecEnv, as that is what we will use
		// when benchmarking.
		pkgPath := filepath.Join(bcfg.BinDir, bench.name, bench.pkg)
		dummyBin := filepath.Join(bcfg.BinDir, "dummy")
		goTool := cfg.GoTool()
		goTool.Env = cfg.ExecEnv.MustSet("GOROOT=" + cfg.GoRoot)
		if err := goTool.BuildPath(pkgPath, dummyBin); err != nil {
			return fmt.Errorf("error building %s %s: %w", bench.name, bench.pkg, err)
		}
	}

	if err := cfg.GoTool().BuildPath(bcfg.BenchDir, filepath.Join(bcfg.BinDir, "go-build-bench")); err != nil {
		return fmt.Errorf("error building go-build tool: %w", err)
	}
	return nil
}

func (h GoBuild) Run(pcfg *common.Config, rcfg *common.RunConfig) error {
	// Local copy of config for updating GOROOT.
	cfg := pcfg.Copy()
	cfg.GoRoot = filepath.Join(rcfg.BinDir, "goroot") // see Build, above.

	benchmarks, err := goBuildBenchmarks(cfg, rcfg.Short)
	if err != nil {
		return fmt.Errorf("error getting benchmark list: %v", err)
	}

	for _, bench := range benchmarks {
		cmd := exec.Command(
			filepath.Join(rcfg.BinDir, "go-build-bench"),
			append(rcfg.Args, []string{
				"-go", cfg.GoTool().Tool,
				"-tmp", rcfg.TmpDir,
				filepath.Join(rcfg.BinDir, bench.name, bench.pkg),
			}...)...,
		)
		cmd.Env = cfg.ExecEnv.Collapse()
		cmd.Stdout = rcfg.Results
		cmd.Stderr = rcfg.Log
		log.TraceCommand(cmd, false)
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

// goBuildBenchmarks returns the set of benchmarks to run for this
// configuration.
//
// cfg may be nil if the build configuration isn't known yet. In that case, it
// returns the set of benchmarks that may run.
func goBuildBenchmarks(cfg *common.Config, short bool) ([]*buildBenchmark, error) {
	var bi *buildinfo.BuildInfo
	if cfg != nil {
		var err error
		bi, err = buildinfo.ReadFile(cfg.GoTool().Tool)
		if err != nil {
			return nil, fmt.Errorf("error reading build info from Go toolchain: %v", err)
		}
	}

	base := buildBenchmarks
	if short {
		base = buildBenchmarksShort
	}

	var out []*buildBenchmark
	for _, bench := range base {
		if bench.minGo != "" && bi != nil && version.Compare(bi.GoVersion, bench.minGo) < 0 {
			log.Printf("Skipping go-build on %s: toolchain version %s less than required %s", bench.name, bi.GoVersion, bench.minGo)
			continue
		}
		out = append(out, bench)
	}

	return out, nil
}
