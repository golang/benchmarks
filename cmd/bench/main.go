// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Binary bench provides a unified wrapper around the different types of
// benchmarks in x/benchmarks.
//
// Benchmarks are run against the toolchain in GOROOT, and optionally an
// additional baseline toolchain in BENCH_BASELINE_GOROOT.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/benchmarks/sweet/common"
)

var wait = flag.Bool("wait", true, "wait for system idle before starting benchmarking")

func determineGOROOT() (string, error) {
	g, ok := os.LookupEnv("GOROOT")
	if ok {
		return g, nil
	}

	cmd := exec.Command("go", "env", "GOROOT")
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

type toolchain struct {
	*common.Go
	Name string
}

func toolchainFromGOROOT(name, goroot string) *toolchain {
	return &toolchain{
		Go: &common.Go{
			Tool: filepath.Join(goroot, "bin", "go"),
			// Update the GOROOT so the wrong one doesn't propagate from
			// the environment.
			Env:        common.NewEnvFromEnviron().MustSet("GOROOT=" + goroot),
			PassOutput: true,
		},
		Name: name,
	}
}

func run(tcs []*toolchain) error {
	// Because each of the functions below is responsible for running
	// benchmarks under each toolchain itself, it is also responsible
	// for ensuring that the benchmark tag "toolchain" is printed.
	pass := true
	if err := goTest(tcs); err != nil {
		pass = false
		log.Printf("Error running Go tests: %v", err)
	}
	if err := bent(tcs); err != nil {
		pass = false
		log.Printf("Error running bent: %v", err)
	}
	if err := sweet(tcs); err != nil {
		pass = false
		log.Printf("Error running sweet: %v", err)
	}
	if !pass {
		return fmt.Errorf("benchmarks failed")
	}
	return nil
}

func main() {
	flag.Parse()

	if *wait {
		// We may be on a freshly booted VM. Wait for boot tasks to
		// complete before continuing.
		if err := waitForIdle(); err != nil {
			log.Fatalf("Failed to wait for idle: %v", err)
		}
	}

	// Find the toolchain under test.
	gorootExperiment, err := determineGOROOT()
	if err != nil {
		log.Fatalf("Unable to determine GOROOT: %v", err)
	}
	toolchains := []*toolchain{toolchainFromGOROOT("experiment", gorootExperiment)}

	// Find the baseline toolchain, if applicable.
	gorootBaseline := os.Getenv("BENCH_BASELINE_GOROOT")
	if gorootBaseline != "" {
		toolchains = append(toolchains, toolchainFromGOROOT("baseline", gorootBaseline))
	}

	// Try to identify the Go branch. If we can't, just make sure we say so explicitly.
	branch := os.Getenv("BENCH_BRANCH")
	if branch == "" {
		branch = "unknown"
	}
	fmt.Printf("branch: %s\n", branch)

	// Run benchmarks against the toolchains.
	if err := run(toolchains); err != nil {
		log.Print("FAIL")
		os.Exit(1)
	}
}
