// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"runtime"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	ptbench "golang.org/x/benchmarks/third_party/fogleman-pt"
)

var (
	iter int
	im   image.Image
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] <gopher.obj>\n", os.Args[0])
		flag.PrintDefaults()
	}
	driver.SetFlags(flag.CommandLine)
	flag.IntVar(&iter, "iter", 2, "number of iterations to run renderer for")
}

func main() {
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "expected one argument: gopher.obj")
		os.Exit(1)
	}

	// load and transform gopher mesh
	gopher, err := ptbench.Load(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	name := fmt.Sprintf("FoglemanPathTraceRenderGopherIter%d", iter)
	err = driver.RunBenchmark(name, func(b *driver.B) error {
		runtime.GC()
		b.ResetTimer()
		im = gopher.Render(iter)
		return nil
	}, driver.InProcessMeasurementOptions...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
