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
	"time"

	"golang.org/x/benchmarks/sweet/common"
)

var (
	wait              = flag.Bool("wait", true, "wait for system idle before starting benchmarking")
	gorootExperiment  = flag.String("goroot", "", "GOROOT to test (default $GOROOT or 'go env GOROOT')")
	gorootBaseline    = flag.String("goroot-baseline", "", "baseline GOROOT to test against (optional) (default $BENCH_BASELINE_GOROOT)")
	branch            = flag.String("branch", "", "branch of the commits we're testing against (default $BENCH_BRANCH or unknown)")
	repository        = flag.String("repository", "", "repository name of the commits we're testing against (default $BENCH_REPOSITORY or 'go')")
	subRepoExperiment = flag.String("subrepo", "", "Sub-repo dir to test (default $BENCH_SUBREPO_PATH)")
	subRepoBaseline   = flag.String("subrepo-baseline", "", "Sub-repo baseline to test against (default $BENCH_SUBREPO_BASELINE_PATH)")
)

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
	fmt.Printf("runstamp: %s\n", time.Now().In(time.UTC).Format(time.RFC3339Nano))

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
	gorootExperiment := *gorootExperiment
	if gorootExperiment == "" {
		var err error
		gorootExperiment, err = determineGOROOT()
		if err != nil {
			log.Fatalf("Unable to determine GOROOT: %v", err)
		}
	}
	toolchains := []*toolchain{toolchainFromGOROOT("experiment", gorootExperiment)}

	// Find the baseline toolchain, if applicable.
	gorootBaseline := *gorootBaseline
	if gorootBaseline == "" {
		gorootBaseline = os.Getenv("BENCH_BASELINE_GOROOT")
	}
	if gorootBaseline != "" {
		toolchains = append(toolchains, toolchainFromGOROOT("baseline", gorootBaseline))
	}

	// Determine the repository we are testing. Defaults to 'go' because
	// old versions of the coordinator don't specify the repository, but
	// also only test go.
	repository := *repository
	if repository == "" {
		repository = os.Getenv("BENCH_REPOSITORY")
	}
	if repository == "" {
		repository = "go"
	}
	fmt.Printf("repository: %s\n", repository)

	// Try to identify the branch. If we can't, just make sure we say so
	// explicitly.
	branch := *branch
	if branch == "" {
		branch = os.Getenv("BENCH_BRANCH")
	}
	if branch == "" {
		branch = "unknown"
	}
	fmt.Printf("branch: %s\n", branch)

	subRepoExperiment := *subRepoExperiment
	if subRepoExperiment == "" {
		subRepoExperiment = os.Getenv("BENCH_SUBREPO_PATH")
	}
	subRepoBaseline := *subRepoBaseline
	if subRepoBaseline == "" {
		subRepoBaseline = os.Getenv("BENCH_SUBREPO_BASELINE_PATH")
	}

	if repository != "go" {
		toolchain := toolchainFromGOROOT("baseline", gorootBaseline)
		if err := goTestSubrepo(toolchain, repository, subRepoBaseline, subRepoExperiment); err != nil {
			log.Print("FAIL")
			os.Exit(1)
		}
		return
	}
	// Run benchmarks against the toolchains.
	if err := run(toolchains); err != nil {
		log.Print("FAIL")
		os.Exit(1)
	}
}
