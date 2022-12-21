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
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

func workloadsPath(assetsDir, subBenchmark string) string {
	p := common.CurrentPlatform()
	platformDir := fmt.Sprintf("%s-%s", p.GOOS, p.GOARCH)
	return filepath.Join(assetsDir, subBenchmark, "bin", platformDir, "workload")
}

func (c *config) profilePath(typ diagnostics.Type) string {
	return filepath.Join(c.tmpDir, string(typ)+".prof")
}

func (cfg *config) runscCmd(arg ...string) *exec.Cmd {
	var cmd *exec.Cmd
	goProfiling := false
	for _, typ := range []diagnostics.Type{diagnostics.CPUProfile, diagnostics.MemProfile, diagnostics.Trace} {
		if driver.DiagnosticEnabled(typ) {
			goProfiling = true
			break
		}
	}
	if goProfiling {
		arg = append([]string{"-profile"}, arg...)
	}
	if driver.DiagnosticEnabled(diagnostics.CPUProfile) {
		arg = append([]string{"-profile-cpu", cfg.profilePath(diagnostics.CPUProfile)}, arg...)
	}
	if driver.DiagnosticEnabled(diagnostics.MemProfile) {
		arg = append([]string{"-profile-heap", cfg.profilePath(diagnostics.MemProfile)}, arg...)
	}
	if driver.DiagnosticEnabled(diagnostics.Trace) {
		arg = append([]string{"-trace", cfg.profilePath(diagnostics.Trace)}, arg...)
	}
	if driver.DiagnosticEnabled(diagnostics.Perf) {
		perfArgs := []string{"record", "-o", cfg.profilePath(diagnostics.Perf)}
		perfArgs = append(perfArgs, driver.PerfFlags()...)
		perfArgs = append(perfArgs, cfg.runscPath)
		perfArgs = append(perfArgs, arg...)
		cmd = exec.Command("perf", perfArgs...)
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
