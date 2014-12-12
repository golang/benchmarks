// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Bench contains benchmarks for the Go performance dashboard:
	http://build.golang.org/perf
Run without flags to get list of available benchmarks:
	./bench
	build,garbage,http,json
To run a benchmark execute:
	./bench -bench=json
See the README file for more details.
*/
package main // import "golang.org/x/benchmarks/bench"

import (
	"golang.org/x/benchmarks/driver"

	_ "golang.org/x/benchmarks/build"
	_ "golang.org/x/benchmarks/garbage"
	_ "golang.org/x/benchmarks/http"
	_ "golang.org/x/benchmarks/json"
)

func main() {
	driver.Main()
}
