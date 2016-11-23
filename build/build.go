// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Build is a benchmark that examines compiler and linker performance.
// It executes 'go build -a cmd/go'.
package main

import (
	"log"
	"os"
	"os/exec"

	"golang.org/x/benchmarks/driver"
)

func main() {
	driver.Main(benchmark)
}

func benchmark() driver.Result {
	if os.Getenv("GOMAXPROCS") == "" {
		os.Setenv("GOMAXPROCS", "1")
	}
	res := driver.MakeResult()
	for i := 0; i < driver.BenchNum; i++ {
		res1 := benchmarkOnce()
		if res.RunTime == 0 || res.RunTime > res1.RunTime {
			res = res1
		}
		log.Printf("Run %v: %+v\n", i, res)
	}
	perf1, perf2 := driver.RunUnderProfiler("go", "build", "-o", "goperf", "-a", "-p", os.Getenv("GOMAXPROCS"), "cmd/go")
	if perf1 != "" {
		res.Files["processes"] = perf1
	}
	if perf2 != "" {
		res.Files["cpuprof"] = perf2
	}
	return res
}

func benchmarkOnce() driver.Result {
	// run 'go build -a'
	res := driver.MakeResult()
	cmd := exec.Command("go", "build", "-o", "gobuild", "-a", "-p", os.Getenv("GOMAXPROCS"), "cmd/go")
	out, err := driver.RunAndCollectSysStats(cmd, &res, 1, "build-")
	if err != nil {
		log.Fatalf("Failed to run 'go build -a cmd/go': %v\n%v", err, out)
	}

	// go command binary size
	gof, err := os.Open("gobuild")
	if err != nil {
		log.Fatalf("Failed to open $GOROOT/bin/go: %v\n", err)
	}
	st, err := gof.Stat()
	if err != nil {
		log.Fatalf("Failed to stat $GOROOT/bin/go: %v\n", err)
	}
	res.Metrics["binary-size"] = uint64(st.Size())

	sizef := driver.Size("gobuild")
	if sizef != "" {
		res.Files["sections"] = sizef
	}

	return res
}
