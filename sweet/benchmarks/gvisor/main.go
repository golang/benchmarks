// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux
// +build linux

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

type config struct {
	runscPath string
	assetsDir string
	tmpDir    string
	short     bool
}

var cliCfg config

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.StringVar(&cliCfg.runscPath, "runsc", "", "path to the runsc binary")
	flag.StringVar(&cliCfg.assetsDir, "assets-dir", "", "path to the directory containing benchmark root filesystems")
	flag.StringVar(&cliCfg.tmpDir, "tmp", "", "path to a temporary working directory")
	flag.BoolVar(&cliCfg.short, "short", false, "whether to run a short version of the benchmarks")
}

type benchmark interface {
	name() string
	run(*config, io.Writer) error
}

func main1() error {
	benchmarks := []benchmark{
		startup{},
		systemCall{500000},
		httpServer{20 * time.Second},
	}
	if cliCfg.short {
		benchmarks = []benchmark{
			startup{},
			systemCall{500},
			httpServer{1 * time.Second},
		}
	}

	// Run each benchmark once.
	for _, bench := range benchmarks {
		// Run the benchmark command under runsc.
		var buf bytes.Buffer
		if err := bench.run(&cliCfg, &buf); err != nil {
			if buf.Len() != 0 {
				fmt.Fprintln(os.Stderr, "=== Benchmark stdout+stderr ===")
				fmt.Fprintln(os.Stderr, buf.String())
			}
			return err
		}
		for _, typ := range diagnostics.Types() {
			if !driver.DiagnosticEnabled(typ) {
				continue
			}
			// runscCmd ensures these are created if necessary.
			if err := driver.CopyDiagnosticData(cliCfg.profilePath(typ), typ, bench.name()); err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	debug.SetTraceback("all")

	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected args\n")
		os.Exit(1)
	}
	if err := main1(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
