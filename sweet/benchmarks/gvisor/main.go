// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

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
)

type config struct {
	runscPath string
	assetsDir string
	tmpDir    string
	short     bool

	diag *driver.Diagnostics
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
		// TODO(go.dev/issue/67508): Disable the startup benchmark because it doesn't work
		// on the builders.
		// startup{},
		systemCall{500000},
		httpServer{20 * time.Second},
	}
	if cliCfg.short {
		benchmarks = []benchmark{
			// TODO(go.dev/issue/67508): Disable the startup benchmark because it doesn't work
			// on the builders.
			// startup{},
			systemCall{500},
			httpServer{1 * time.Second},
		}
	}

	// Run each benchmark once.
	for _, bench := range benchmarks {
		cfg := cliCfg
		cfg.diag = driver.NewDiagnostics(bench.name())

		// Run the benchmark command under runsc.
		var buf bytes.Buffer
		if err := bench.run(&cfg, &buf); err != nil {
			if buf.Len() != 0 {
				fmt.Fprintf(os.Stderr, "=== Benchmark %s stdout+stderr ===", bench.name())
				fmt.Fprintf(os.Stderr, "%s\n", buf.String())
			}
			return err
		}

		cfg.diag.Commit(nil)
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
