// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.16

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

var dir string

// TestMain implemented to allow (1) alternate use as bent command itself if BENT_TEST_IS_CMD_BENT is in environment,
// and (2) to create and remove a temporary directory for test initialization.
func TestMain(m *testing.M) {
	if os.Getenv("BENT_TEST_IS_CMD_BENT") != "" {
		main()
		os.Exit(0)
	}
	var err error
	dir, err = os.MkdirTemp("", "bent_test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	m.Run()
}

// bentCmd returns a "bent" command (that is implemented by rerunning the current program after setting
// BENT_TEST_IS_CMD_BENT).  The command is always run in the temporary directory created by TestMain.
func bentCmd(t *testing.T, args ...string) *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BENT_TEST_IS_CMD_BENT=1", "PWD="+dir)
	return cmd
}

func TestBent(t *testing.T) {
	if runtime.GOARCH == "wasm" {
		t.Skipf("skipping test: exec not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	cmd := bentCmd(t, "-I")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", output)
		t.Fatal(err)
	}
	t.Log(string(output))
	Cs := []string{"sample", "cronjob", "cmpjob", "gollvm"}
	Bs := []string{"all", "50", "gc", "gcplus", "trial"}
	for _, c := range Cs {
		for _, b := range Bs {
			cmd = bentCmd(t, "-l", "-C=configurations-"+c+".toml", "-B=benchmarks-"+b+".toml")
			output, err = cmd.CombinedOutput()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", output)
				t.Fatal(err)
			}
			t.Log(string(output))
		}
		Bs = Bs[:1] // truncate Bs for remaining configurations
	}

}
