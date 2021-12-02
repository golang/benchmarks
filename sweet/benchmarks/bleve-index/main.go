// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !aix && !plan9
// +build !aix,!plan9

package main

import (
	"compress/bzip2"
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	blevebench "golang.org/x/benchmarks/third_party/bleve-bench"

	"github.com/blevesearch/bleve"
	_ "github.com/blevesearch/bleve/analysis/analyzer/keyword"
	wikiparse "github.com/dustin/go-wikiparse"
)

var (
	batchSize int
	documents int
)

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.IntVar(&batchSize, "batch-size", 256, "number of index requests to batch together")
	flag.IntVar(&documents, "documents", 1000, "number of documents to index")
}

func parseFlags() error {
	flag.Parse()
	if flag.NArg() != 1 {
		return fmt.Errorf("expected wiki dump as input")
	}
	return nil
}

func run(wikidump string) error {
	f, err := os.Open(wikidump)
	if err != nil {
		return err
	}
	defer f.Close()

	z := bzip2.NewReader(f)

	parser, err := wikiparse.NewParser(z)
	if err != nil {
		return err
	}

	articles := make([]blevebench.Article, 0, documents)
	for d := 0; d < documents; d++ {
		p, err := parser.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if len(p.Revisions) == 0 {
			continue
		}
		articles = append(articles, blevebench.Article{
			Title: p.Title,
			Text:  p.Revisions[0].Text,
		})
	}

	mapping := blevebench.ArticleMapping()
	name := fmt.Sprintf("BleveIndexBatch%d", batchSize)
	return driver.RunBenchmark(name, func(d *driver.B) error {
		index, err := bleve.NewMemOnly(mapping)
		if err != nil {
			return err
		}
		b := index.NewBatch()
		for _, a := range articles {
			b.Index(a.Title, a)
			if b.Size() >= batchSize {
				if err := index.Batch(b); err != nil {
					return err
				}
				b = index.NewBatch()
			}
		}
		if b.Size() != 0 {
			if err := index.Batch(b); err != nil {
				return err
			}
		}
		d.StopTimer()
		return index.Close()
	}, driver.InProcessMeasurementOptions...)
}

func main() {
	if err := parseFlags(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
