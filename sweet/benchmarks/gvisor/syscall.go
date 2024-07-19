// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux
// +build linux

package main

import (
	"io"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/cgroups"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
)

type systemCall struct {
	ops int
}

func (b systemCall) name() string {
	return "GVisorSyscall"
}

func (b systemCall) run(cfg *config, out io.Writer) error {
	baseCmd, postExit := cfg.runscCmd("-rootless", "do", workloadsPath(cfg.assetsDir, "syscall"))
	baseCmd.Stdout = out
	baseCmd.Stderr = out
	cmd, err := cgroups.WrapCommand(baseCmd, "test-syscall.scope")
	if err != nil {
		return err
	}
	defer func() {
		for _, fn := range postExit {
			fn()
		}
	}()
	return driver.RunBenchmark(b.name(), func(d *driver.B) error {
		d.Ops(b.ops)
		d.ResetTimer()
		return cmd.Run()
	}, driver.DoTime(true), driver.DoAvgRSS(cmd.RSSFunc()))
}
