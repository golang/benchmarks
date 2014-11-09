// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

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
