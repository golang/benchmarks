// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux
// +build linux

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/common"
)

func workloadsPath(assetsDir, subBenchmark string) string {
	p := common.CurrentPlatform()
	platformDir := fmt.Sprintf("%s-%s", p.GOOS, p.GOARCH)
	return filepath.Join(assetsDir, subBenchmark, "bin", platformDir, "workload")
}

func (c *config) profilePath(typ driver.ProfileType) string {
	return filepath.Join(c.tmpDir, string(typ)+".prof")
}

func (cfg *config) runscCmd(arg ...string) *exec.Cmd {
	var cmd *exec.Cmd
	goProfiling := false
	for _, typ := range []driver.ProfileType{driver.ProfileCPU, driver.ProfileMem} {
		if driver.ProfilingEnabled(typ) {
			goProfiling = true
			break
		}
	}
	if goProfiling {
		arg = append([]string{"-profile"}, arg...)
	}
	if driver.ProfilingEnabled(driver.ProfileCPU) {
		arg = append([]string{"-profile-cpu", cfg.profilePath(driver.ProfileCPU)}, arg...)
	}
	if driver.ProfilingEnabled(driver.ProfileMem) {
		arg = append([]string{"-profile-heap", cfg.profilePath(driver.ProfileMem)}, arg...)
	}
	if driver.ProfilingEnabled(driver.ProfilePerf) {
		cmd = exec.Command("perf", append([]string{"record", "-o", cfg.profilePath(driver.ProfilePerf), cfg.runscPath}, arg...)...)
	} else {
		cmd = exec.Command(cfg.runscPath, arg...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Try to bring down the sandbox if we unexpectedly exit.
		Pdeathsig: syscall.SIGKILL,

		// New process group, so we can kill the entire sub-process
		// tree at once.
		Setpgid: true,
	}
	return cmd
}
