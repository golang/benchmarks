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

func goCommand(goroot string, args ...string) *exec.Cmd {
	bin := filepath.Join(goroot, "bin/go")
	cmd := exec.Command(bin, args...)
	return cmd
}

func run(goroot string) error {
	log.Printf("GOROOT under test: %s", goroot)

	pass := true

	if err := goTest(goroot); err != nil {
		pass = false
		log.Printf("Error running Go tests: %v", err)
	}

	if err := bent(goroot); err != nil {
		pass = false
		log.Printf("Error running bent: %v", err)
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

	goroot, err := determineGOROOT()
	if err != nil {
		log.Fatalf("Unable to determine GOROOT: %v", err)
	}

	fmt.Println("toolchain: experiment")

	pass := true
	if err := run(goroot); err != nil {
		pass = false
	}

	baseline := os.Getenv("BENCH_BASELINE_GOROOT")
	if baseline != "" {
		fmt.Println("toolchain: baseline")

		if err := run(baseline); err != nil {
			pass = false
		}
	}

	if !pass {
		log.Printf("FAIL")
		os.Exit(1)
	}
}
