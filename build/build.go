// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"code.google.com/p/go.benchmarks/driver"
)

func init() {
	driver.Register("build", benchmark)
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
	perf1, perf2 := driver.RunUnderProfiler("go", "install", "-a", "-p", os.Getenv("GOMAXPROCS"), "cmd/go")
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
	t0 := time.Now()
	cmd := exec.Command("go", "install", "-a", "-p", os.Getenv("GOMAXPROCS"), "cmd/go")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start 'go install -a cmd/go': %v", err)
	}
	ss := driver.InitSysStats(1, cmd)
	if err := cmd.Wait(); err != nil {
		log.Fatalf("Failed to run 'go install -a cmd/go': %v\n%v", err, stderr.String())
	}
	res := driver.MakeResult()
	res.RunTime = uint64(time.Since(t0))
	res.Metrics["build-time"] = res.RunTime
	ss.Collect(&res, "build-")

	// go command binary size
	gobin := filepath.Join(os.Getenv("GOROOT"), "bin", "go")
	if runtime.GOOS == "windows" {
		gobin += ".exe"
	}
	gof, err := os.Open(gobin)
	if err != nil {
		log.Fatalf("Failed to open $GOROOT/bin/go: %v\n", err)
	}
	st, err := gof.Stat()
	if err != nil {
		log.Fatalf("Failed to stat $GOROOT/bin/go: %v\n", err)
	}
	res.Metrics["binary-size"] = uint64(st.Size())

	sizef := driver.Size(gobin)
	if sizef != "" {
		res.Files["sections"] = sizef
	}

	return res
}
