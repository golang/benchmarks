// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

type ESBuild struct {
	haveYarn bool
}

func (h *ESBuild) CheckPrerequisites() error {
	// Check if we have the yarn command.
	if _, err := exec.LookPath("yarn"); err == nil {
		h.haveYarn = true
	} else if !errors.Is(err, exec.ErrNotFound) {
		return err
	}
	return nil
}

func (h *ESBuild) Get(gcfg *common.GetConfig) error {
	err := gitShallowClone(
		gcfg.SrcDir,
		"https://github.com/evanw/esbuild",
		"v0.23.1",
	)
	if err != nil {
		return err
	}
	runMake := func(rules ...string) error {
		cmd := exec.Command("make", append([]string{"-C", gcfg.SrcDir}, rules...)...)
		log.TraceCommand(cmd, false)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to run make: %v: output:\n%s", err, out)
		}
		return nil
	}
	// Fetch downstream benchmark dependencies.
	if err := runMake("bench/three"); err != nil {
		return err
	}
	if h.haveYarn {
		if err := runMake("bench/readmin"); err != nil {
			return err
		}
	}
	// Run the command but ignore errors. There's something weird going on with
	// a sed command in this make rule, but we don't actually need it to succeed
	// to run the benchmarks.
	_ = runMake("bench/rome")
	return nil
}

func (h *ESBuild) Build(cfg *common.Config, bcfg *common.BuildConfig) error {
	// Generate a symlink to the repository and put it in bin.
	// It's not a binary, but it's the only place we can put it
	// and still access it in Run.
	link := filepath.Join(bcfg.BinDir, "esbuild-src")
	err := symlink(link, bcfg.SrcDir)
	if err != nil {
		return err
	}
	// Build driver.
	if err := cfg.GoTool().BuildPath(bcfg.BenchDir, filepath.Join(bcfg.BinDir, "esbuild-bench")); err != nil {
		return err
	}
	// Build esbuild.
	return cfg.GoTool().BuildPath(filepath.Join(bcfg.SrcDir, "cmd", "esbuild"), filepath.Join(bcfg.BinDir, "esbuild"))
}

func (h *ESBuild) Run(cfg *common.Config, rcfg *common.RunConfig) error {
	for _, b := range esbuildBenchmarks {
		if b.needYarn && !h.haveYarn {
			continue
		}
		cmd := exec.Command(
			filepath.Join(rcfg.BinDir, "esbuild-bench"),
			"-bin", filepath.Join(rcfg.BinDir, "esbuild"),
			"-src", filepath.Join(rcfg.BinDir, "esbuild-src", b.src),
			"-tmp", rcfg.TmpDir,
			"-bench", b.name,
		)
		cmd.Args = append(cmd.Args, rcfg.Args...)
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

type esbuildBenchmark struct {
	name     string
	src      string
	needYarn bool
}

var esbuildBenchmarks = []esbuildBenchmark{
	{"ThreeJS", filepath.Join("bench", "three"), false},
	{"RomeTS", filepath.Join("bench", "rome"), false},
	{"ReactAdminJS", filepath.Join("bench", "readmin"), true},
}
