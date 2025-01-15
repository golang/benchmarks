// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// igor is a tool that takes pairwise alignment data as produced by PALS or krishna
// and reports repeat feature family groupings in JSON format.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/third_party/biogo-examples/igor/igor"

	"github.com/biogo/biogo/align/pals"
	"github.com/biogo/biogo/io/featio/gff"
)

const (
	band          = 0.05
	mergeOverlap  = 0
	removeOverlap = 0.95
	requiredCover = 0.95
	strictness    = 0

	pileDiff  = 0.05
	imageDiff = 0.05
)

func main() {
	driver.SetFlags(flag.CommandLine)
	flag.Parse()
	log.SetFlags(0)

	if flag.NArg() != 1 {
		log.Fatal("error: input GFF file required")
	}

	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	err = driver.RunBenchmark("BiogoIgor", func(_ *driver.B) error {
		r := bytes.NewReader(data)
		in := gff.NewReader(r)

		out := bytes.Buffer{}
		out.Grow(1024 * 1024)

		var pf pals.PairFilter
		piles, err := igor.Piles(in, mergeOverlap, pf)
		if err != nil {
			return fmt.Errorf("piling: %v", err)
		}

		_, clusters := igor.Cluster(piles, igor.ClusterConfig{
			BandWidth:         band,
			RequiredCover:     requiredCover,
			OverlapStrictness: strictness,
			OverlapThresh:     removeOverlap,
			Procs:             runtime.GOMAXPROCS(0),
		})
		cc := igor.Group(clusters, igor.GroupConfig{
			PileDiff:  pileDiff,
			ImageDiff: imageDiff,
			Classic:   false,
		})
		err = igor.WriteJSON(cc, &out)
		if err != nil {
			return err
		}
		return nil
	}, driver.InProcessMeasurementOptions...)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
