// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Garbage is a benchmark that stresses garbage collector.
// It repeatedly parses net/http package with go/parser and then discards results.
package main

// The source of net/http was captured at git tag go1.5.2 by
//go:generate sh -c "(echo 'package garbage'; echo 'var src = `'; bundle net/http http '' | sed 's/`/`+\"`\"+`/g'; echo '`') > nethttp.go"

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"runtime"
	"sync"
	"sync/atomic"

	"golang.org/x/benchmarks/driver"
)

func main() {
	driver.Main("Garbage", benchmark)
}

// func init() {
// 	driver.Register("garbage", benchmark)
// }

type ParsedPackage *ast.File

var (
	parsed []ParsedPackage
)

func benchmark() driver.Result {
	if parsed == nil {
		mem := packageMemConsumption()
		avail := (driver.BenchMem << 20) * 4 / 5 // 4/5 to account for non-heap memory
		npkg := avail / mem / 2                  // 2 to account for GOGC=100
		parsed = make([]ParsedPackage, npkg)
		for n := 0; n < 2; n++ { // warmup GC
			for i := range parsed {
				parsed[i] = parsePackage()
			}
		}
		fmt.Printf("consumption=%vKB npkg=%d\n", mem>>10, npkg)
	}
	return driver.Benchmark(benchmarkN)
}

func benchmarkN(N uint64) {
	P := runtime.GOMAXPROCS(0)
	// Create G goroutines, but only 2*P of them parse at the same time.
	G := 1024
	gate := make(chan bool, 2*P)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(G)
	remain := int64(N)
	pos := 0
	for g := 0; g < G; g++ {
		go func() {
			defer wg.Done()
			for atomic.AddInt64(&remain, -1) >= 0 {
				gate <- true
				p := parsePackage()
				mu.Lock()
				// Overwrite only half of the array,
				// the other part represents "old" generation.
				parsed[pos%(len(parsed)/2)] = p
				pos++
				mu.Unlock()
				<-gate
			}
		}()
	}
	wg.Wait()
}

// packageMemConsumption returns memory consumption of a single parsed package.
func packageMemConsumption() int {
	// One GC does not give precise results,
	// because concurrent sweep may be still in progress.
	runtime.GC()
	runtime.GC()
	ms0 := new(runtime.MemStats)
	runtime.ReadMemStats(ms0)
	const N = 10
	var parsed [N]ParsedPackage
	for i := range parsed {
		parsed[i] = parsePackage()
	}
	runtime.GC()
	runtime.GC()
	// Keep it alive.
	if parsed[0] == nil {
		fmt.Println(&parsed)
	}
	ms1 := new(runtime.MemStats)
	runtime.ReadMemStats(ms1)
	mem := int(ms1.Alloc-ms0.Alloc) / N
	if mem < 1<<16 {
		mem = 1 << 16
	}
	return mem
}

// parsePackage parses and returns net/http package.
func parsePackage() ParsedPackage {
	pkgs, err := parser.ParseFile(token.NewFileSet(), "net/http", src, parser.ParseComments)
	if err != nil {
		println("parse", err.Error())
		panic("fail")
	}
	return pkgs
}
