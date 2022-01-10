// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"os"
	"strings"
)

func goTest(goroot string) error {
	log.Printf("Running Go test benchmarks for GOROOT %s", goroot)

	cmd := goCommand(goroot, "test", "-v", "-run=none", "-bench=.", "-count=5", "golang.org/x/benchmarks/...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	env := os.Environ()
	needGOROOT := true
	for i := range env {
		if strings.HasPrefix(env[i], "GOROOT=") {
			env[i] = "GOROOT=" + goroot
			needGOROOT = false
		}
	}
	if needGOROOT {
		env = append(env, "GOROOT="+goroot)
	}
	cmd.Env = env

	return cmd.Run()
}
