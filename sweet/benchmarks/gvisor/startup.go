// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux
// +build linux

package main

import (
	"io"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
)

type startup struct{}

func (b startup) name() string {
	return "GVisorStartup"
}

func (b startup) run(cfg *config, out io.Writer) error {
	cmd := cfg.runscCmd("-rootless", "-network=none", "run", "bench")
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Dir = filepath.Join(cfg.assetsDir, "startup")
	return driver.RunBenchmark(b.name(), func(d *driver.B) error {
		return cmd.Run()
	}, driver.DoTime(true))
}
