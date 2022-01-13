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
	animatebench "golang.org/x/benchmarks/third_party/fogleman-fauxgl"
)

var im image.Image

var imagesPerRotation int

func init() {
	flag.IntVar(&imagesPerRotation, "images-per-rotation", 72, "number of images per rotation to generate")
}

func main() {
	driver.SetFlags(flag.CommandLine)
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "expected input STL file")
		os.Exit(1)
	}
	inc := 360 / imagesPerRotation

	// Load mesh into animation structure.
	anim, err := animatebench.Load(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = driver.RunBenchmark("FoglemanFauxGLRenderRotateBoat", func(b *driver.B) error {
		runtime.GC()
		b.ResetTimer()
		for i := 0; i < 360; i += inc {
			im = anim.RenderNext()
		}
		return nil
	}, driver.InProcessMeasurementOptions...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
