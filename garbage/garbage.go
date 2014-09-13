// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package garbage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"code.google.com/p/go.benchmarks/driver"
)

func init() {
	driver.Register("garbage", benchmark)
}

type ParsedPackage map[string]*ast.Package

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
	pkgname := "http"
	dirpath := filepath.Join(runtime.GOROOT(), "/src/pkg/net/", pkgname)
	if !exists(dirpath) {
		// As of 8th Sept 2014 the "pkg" prefix was removed from the std lib
		// http://golang.org/s/go14nopkg
		dirpath = filepath.Join(runtime.GOROOT(), "/src/net/", pkgname)
	}
	// filter function to select the desired .go files
	filter := func(d os.FileInfo) bool {
		if isPkgFile(d) {
			// Some directories contain main packages: Only accept
			// files that belong to the expected package so that
			// parser.ParsePackage doesn't return "multiple packages
			// found" errors.
			// Additionally, accept the special package name
			// fakePkgName if we are looking at cmd documentation.
			name := pkgName(dirpath + "/" + d.Name())
			return name == pkgname
		}
		return false
	}

	// get package AST
	pkgs, err := parser.ParseDir(token.NewFileSet(), dirpath, filter, parser.ParseComments)
	if err != nil {
		println("parse", dirpath, err.Error())
		panic("fail")
	}
	return pkgs
}

func isGoFile(dir os.FileInfo) bool {
	return !dir.IsDir() &&
		!strings.HasPrefix(dir.Name(), ".") && // ignore .files
		path.Ext(dir.Name()) == ".go"
}

func isPkgFile(dir os.FileInfo) bool {
	return isGoFile(dir) &&
		!strings.HasSuffix(dir.Name(), "_test.go") // ignore test files
}

func pkgName(filename string) string {
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.PackageClauseOnly)
	if err != nil || file == nil {
		return ""
	}
	return file.Name.Name
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
