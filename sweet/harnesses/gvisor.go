// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

type GVisor struct{}

func (h GVisor) CheckPrerequisites() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("requires Linux")
	}
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("requires amd64")
	}
	return nil
}

func (h GVisor) Get(gcfg *common.GetConfig) error {
	return gitCloneToCommit(
		gcfg.SrcDir,
		"https://github.com/google/gvisor",
		"go",
		"b75aeea", // release-20240513.0-37-g4f08fc481
	)
}

func (h GVisor) Build(cfg *common.Config, bcfg *common.BuildConfig) error {
	// Build benchmarking client which will handle a bunch of coordination.
	if err := cfg.GoTool(bcfg.BuildLog).BuildPath(filepath.Join(bcfg.BenchDir), filepath.Join(bcfg.BinDir, "gvisor-bench")); err != nil {
		return err
	}

	// Build the runsc package in the repository. CGO_ENABLED must be 0.
	// See https://github.com/google/gvisor#using-go-get.
	cfg.BuildEnv.Env = cfg.BuildEnv.MustSet("CGO_ENABLED=0")
	bin := filepath.Join(bcfg.BinDir, "runsc")
	if err := cfg.GoTool(bcfg.BuildLog).BuildPath(filepath.Join(bcfg.SrcDir, "runsc"), bin); err != nil {
		return err
	}

	// Make sure the binary has all the right permissions set.
	// See https://gvisor.dev/docs/user_guide/install/#install-directly
	log.CommandPrintf("chmod 755 %s", bin)
	if err := os.Chmod(bin, 0755); err != nil {
		return fmt.Errorf("failed to set permissions on runsc: %w", err)
	}
	return nil
}

func (h GVisor) Run(cfg *common.Config, rcfg *common.RunConfig) error {
	args := append(rcfg.Args, []string{
		"-runsc", filepath.Join(rcfg.BinDir, "runsc"),
		"-assets-dir", rcfg.AssetsDir,
		"-tmp", rcfg.TmpDir,
	}...)
	if rcfg.Short {
		args = append(args, "-short")
	}
	cmd := exec.Command(
		filepath.Join(rcfg.BinDir, "gvisor-bench"),
		args...,
	)
	cmd.Env = cfg.ExecEnv.Collapse()
	cmd.Stdout = rcfg.Results
	cmd.Stderr = rcfg.Log
	log.TraceCommand(cmd, false)
	return cmd.Run()
}
