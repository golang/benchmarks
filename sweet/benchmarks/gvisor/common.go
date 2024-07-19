// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux
// +build linux

package main

import (
	"fmt"
	"os"
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

func (cfg *config) runscCmd(arg ...string) (*exec.Cmd, []func()) {
	var cmd *exec.Cmd

	cmdArgs := []string{cfg.runscPath}

	goProfiling := false
	var postExit []func()
	addDiagnostic := func(typ diagnostics.Type, flag string) {
		if df, err := cfg.diag.Create(typ); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s diagnostics: %s\n", typ, err)
		} else if df != nil {
			df.Close()
			cmdArgs = append(cmdArgs, flag, df.Name())
			goProfiling = true
			postExit = append(postExit, df.Commit)
		}
	}
	addDiagnostic(diagnostics.CPUProfile, "-profile-cpu")
	addDiagnostic(diagnostics.MemProfile, "-profile-heap")
	addDiagnostic(diagnostics.Trace, "-trace")
	if goProfiling {
		cmdArgs = append(cmdArgs, "-profile")
	}

	if df, err := cfg.diag.Create(diagnostics.Perf); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create %s diagnostics: %s\n", diagnostics.Perf, err)
	} else if df != nil {
		df.Close()
		postExit = append(postExit, df.Commit)

		perfArgs := []string{"perf", "record", "-o", df.Name()}
		perfArgs = append(perfArgs, driver.PerfFlags()...)
		perfArgs = append(perfArgs, cmdArgs...)
		cmdArgs = perfArgs
	}

	cmdArgs = append(cmdArgs, arg...)
	cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Try to bring down the sandbox if we unexpectedly exit.
		Pdeathsig: syscall.SIGKILL,

		// New process group, so we can kill the entire sub-process
		// tree at once.
		Setpgid: true,
	}
	return cmd, postExit
}
