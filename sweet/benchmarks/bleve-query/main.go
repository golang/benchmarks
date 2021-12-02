// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !aix && !plan9
// +build !aix,!plan9

package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"

	"github.com/blevesearch/bleve"
	_ "github.com/blevesearch/bleve/analysis/analyzer/keyword"
)

func parseFlags() error {
	flag.Parse()
	if flag.NArg() != 1 {
		return fmt.Errorf("expected bleve index directory as input")
	}
	return nil
}

func run(idxdir string) error {
	index, err := bleve.Open(idxdir)
	if err != nil {
		return err
	}
	return driver.RunBenchmark("BleveQuery", func(_ *driver.B) error {
		for j := 0; j < 50; j++ {
			for _, term := range terms {
				query := bleve.NewTermQuery(term)
				query.SetField("Text")
				_, err := index.Search(bleve.NewSearchRequest(query))
				if err != nil {
					return err
				}
			}
		}
		return nil
	}, driver.InProcessMeasurementOptions...)
}

func main() {
	driver.SetFlags(flag.CommandLine)
	if err := parseFlags(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
