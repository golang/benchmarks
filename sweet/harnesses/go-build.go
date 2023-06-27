// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/fileutil"
	"golang.org/x/benchmarks/sweet/common/log"
)

type buildBenchmark struct {
	name  string
	pkg   string
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
	// Clone the sources that we're going to build.
	for _, bench := range goBuildBenchmarks(gcfg.Short) {
		if err := bench.clone(filepath.Join(gcfg.SrcDir, bench.name)); err != nil {
			return err
		}
	}
	return nil
}

func (h GoBuild) Build(pcfg *common.Config, bcfg *common.BuildConfig) error {
	// Local copy of config for updating GOROOT.
	cfg := pcfg.Copy()

	// Get the benchmarks we're going to build.
	benchmarks := goBuildBenchmarks(bcfg.Short)

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

	benchmarks := goBuildBenchmarks(rcfg.Short)
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
		cmd.Stderr = rcfg.Results
		log.TraceCommand(cmd, false)
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func goBuildBenchmarks(short bool) []*buildBenchmark {
	if short {
		return buildBenchmarksShort
	}
	return buildBenchmarks
}
