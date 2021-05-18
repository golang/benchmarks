// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.16
// +build go1.16

package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

const dataDir = "testdata"

var binary, dir string

// We implement TestMain so remove the test binary when all is done.
func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	var err error
	dir, err = os.MkdirTemp("", "bent_test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	binary = filepath.Join(dir, "testbent.exe")
	return m.Run()
}

var (
	built    = false // We have built the binary.
	failed   = false // We have failed to build the binary, don't try again.
	onlyOnce sync.Once
)

func build(t *testing.T) {
	onlyOnce.Do(func() {
		cmd := exec.Command("go", "build", "-o", binary)
		output, err := cmd.CombinedOutput()
		if err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "%s\n", output)
			t.Fatal(err)
		}
		built = true
	})
	if failed {
		t.Skip("cannot run on this environment")
	}
}

func TestBent(t *testing.T) {
	build(t)
	cmd := exec.Command(binary, "-I")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		failed = true
		fmt.Fprintf(os.Stderr, "%s\n", output)
		t.Fatal(err)
	}
	t.Log(string(output))
	Cs := []string{"sample", "cronjob", "cmpjob", "gollvm"}
	Bs := []string{"all", "50", "gc", "gcplus", "trial"}
	for _, c := range Cs {
		for _, b := range Bs {
			cmd = exec.Command(binary, "-l", "-C=configurations-"+c+".toml", "-B=benchmarks-"+b+".toml")
			cmd.Dir = dir
			output, err = cmd.CombinedOutput()
			if err != nil {
				failed = true
				fmt.Fprintf(os.Stderr, "%s\n", output)
				t.Fatal(err)
			}
			t.Log(string(output))
		}
		Bs = Bs[:1] // truncate Bs for remaining configurations
	}

}
