// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

type Etcd struct{}

func (h Etcd) CheckPrerequisites() error {
	return nil
}

func (h Etcd) Get(gcfg *common.GetConfig) error {
	// Build against the latest alpha.
	//
	// Because of the way etcd is released (as a binary blob),
	// deployed copies tend to be stuck with a specific Go version.
	// Improving performance of these versions doesn't really matter.
	// Instead, try to track something close to HEAD.
	return gitShallowClone(
		gcfg.SrcDir,
		"https://github.com/etcd-io/etcd",
		"v3.6.0-alpha.0",
	)
}

func (h Etcd) Build(cfg *common.Config, bcfg *common.BuildConfig) error {
	env := cfg.BuildEnv.Env

	// Add the Go tool to PATH, since etcd's Makefile doesn't provide enough
	// visibility into how etcd is built to allow us to pass this information
	// directly. Also set the GOROOT explicitly because it might have propagated
	// differently from the environment.
	env = env.Prefix("PATH", filepath.Join(cfg.GoRoot, "bin")+":")
	env = env.MustSet("GOROOT=" + cfg.GoRoot)

	cmd := exec.Command("make", "-C", bcfg.SrcDir, "build")
	cmd.Env = env.Collapse()
	log.TraceCommand(cmd, false)
	// Call Output here to get an *ExitError with a populated Stderr field.
	if _, err := cmd.Output(); err != nil {
		return err
	}
	// Note that no matter what we do, the build script insists on putting the
	// binaries into the source directory, so copy the one we care about into
	// BinDir.
	if err := copyFile(filepath.Join(bcfg.BinDir, "etcd"), filepath.Join(bcfg.SrcDir, "bin", "etcd")); err != nil {
		return err
	}
	// Build etcd's benchmarking tool. Our benchmark is just a wrapper around that.
	benchmarkPkg := filepath.Join(bcfg.SrcDir, "tools", "benchmark")
	if err := cfg.GoTool().BuildPath(benchmarkPkg, filepath.Join(bcfg.BinDir, "benchmark")); err != nil {
		return err
	}
	// Build the benchmark wrapper.
	return cfg.GoTool().BuildPath(bcfg.BenchDir, filepath.Join(bcfg.BinDir, "etcd-bench"))
}

func (h Etcd) Run(cfg *common.Config, rcfg *common.RunConfig) error {
	for _, bench := range []string{"put", "stm"} {
		args := append(rcfg.Args, []string{
			"-bench", bench,
			"-etcd-bin", filepath.Join(rcfg.BinDir, "etcd"),
			"-benchmark-bin", filepath.Join(rcfg.BinDir, "benchmark"),
			"-tmp", rcfg.TmpDir,
		}...)
		if rcfg.Short {
			args = append(args, "-short")
		}
		cmd := exec.Command(
			filepath.Join(rcfg.BinDir, "etcd-bench"),
			args...,
		)
		cmd.Env = cfg.ExecEnv.Collapse()
		cmd.Stdout = rcfg.Results
		cmd.Stderr = rcfg.Results
		log.TraceCommand(cmd, false)
		if err := cmd.Run(); err != nil {
			return err
		}
		// Delete tmp because etcd will have written something there and
		// might attempt to reuse it.
		if err := rmDirContents(rcfg.TmpDir); err != nil {
			return err
		}
	}
	return nil
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
