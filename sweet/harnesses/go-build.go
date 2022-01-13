// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

type buildBenchmark struct {
	name  string
	pkg   string
	clone func(outDir string) error
}

var buildBenchmarks = []*buildBenchmark{
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

type GoBuild struct{}

func (h GoBuild) CheckPrerequisites() error {
	return nil
}

func (h GoBuild) Get(srcDir string) error {
	// Clone the sources that we're going to build.
	for _, bench := range buildBenchmarks {
		if err := bench.clone(filepath.Join(srcDir, bench.name)); err != nil {
			return err
		}
	}
	return nil
}

func (h GoBuild) Build(cfg *common.Config, bcfg *common.BuildConfig) error {
	benchmarks := buildBenchmarks
	if bcfg.Short {
		// Do only the pkgsite benchmark.
		benchmarks = []*buildBenchmark{buildBenchmarks[2]}
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

		// Build the benchmark once, pulling in any requisite packages.
		cmd := exec.Command(cfg.GoTool().Tool, "build")
		cmd.Dir = filepath.Join(bcfg.BinDir, bench.name, bench.pkg)
		log.TraceCommand(cmd, false)
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return cfg.GoTool().BuildPath(bcfg.BenchDir, filepath.Join(bcfg.BinDir, "go-build-bench"))
}

func (h GoBuild) Run(cfg *common.Config, rcfg *common.RunConfig) error {
	benchmarks := buildBenchmarks
	if rcfg.Short {
		// Do only the pkgsite benchmark.
		benchmarks = []*buildBenchmark{buildBenchmarks[2]}
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
		cmd.Stderr = rcfg.Results
		log.TraceCommand(cmd, false)
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
