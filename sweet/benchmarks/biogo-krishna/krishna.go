// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// krishna is a pure Go implementation of Edgar and Myers PALS tool.
// This version of krishna is modified from its original form and only
// computes alignment for a sequence against itself.
package main

import (
	"bytes"
	"flag"
	"log"
	"runtime"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/third_party/biogo-examples/krishna"

	"github.com/biogo/biogo/align/pals"
)

const (
	minHitLen  = 400
	minId      = 0.94
	tubeOffset = 0
	tmpChunk   = 1e6
)

var (
	alignconc     bool
	tmpDir        string
	tmpConcurrent bool
)

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.BoolVar(&alignconc, "alignconc", false, "whether to perform alignment concurrently (2 threads)")
	flag.StringVar(&tmpDir, "tmp", "", "directory to store temporary files")
	flag.BoolVar(&tmpConcurrent, "tmpconc", false, "whether to process morass concurrently")
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	if flag.NArg() != 1 {
		log.Fatal("error: input FASTA target sequence required")
	}
	k, err := krishna.New(flag.Arg(0), tmpDir, krishna.Params{
		TmpChunkSize: 1e6,
		MinHitLen:    400,
		MinHitId:     0.94,
		TubeOffset:   0,
		AlignConc:    alignconc,
		TmpConc:      tmpConcurrent,
	})
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	defer k.CleanUp()
	err = driver.RunBenchmark("BiogoKrishna", func(d *driver.B) error {
		runtime.GC()

		// Make initial buffer size 1 MiB.
		b := bytes.Buffer{}
		b.Grow(1024 * 1024)
		writer := pals.NewWriter(&b, 2, 60, false)
		d.ResetTimer()

		return k.Run(writer)
	}, driver.InProcessMeasurementOptions...)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
