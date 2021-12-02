// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !aix && !plan9
// +build !aix,!plan9

package generators

import (
	"compress/bzip2"
	"io"
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve"
	_ "github.com/blevesearch/bleve/analysis/analyzer/keyword"
	wikiparse "github.com/dustin/go-wikiparse"

	"golang.org/x/benchmarks/sweet/common"
	blevebench "golang.org/x/benchmarks/third_party/bleve-bench"
)

// documents is the number of documents to index.
const documents = 100000

// wikiDumpPath is a path to the static asset from
// which we'll build our index.
var wikiDumpPath = filepath.Join("..", "bleve-index", wikiDumpName)

// BleveQuery is a dynamic assets Generator for the bleve-query benchmark.
type BleveQuery struct{}

// Generate creates a persistent index for the Bleve search engine for
// the bleve-query benchmark. It generates this index from a subset of
// the static assets for the bleve-index benchmark, a dump of wikipedia
// from 2008.
func (_ BleveQuery) Generate(cfg *common.GenConfig) (err error) {
	// Copy README.md over.
	if err := copyFiles(cfg.OutputDir, cfg.AssetsDir, []string{"README.md"}); err != nil {
		return err
	}

	f, err := os.Open(filepath.Join(cfg.AssetsDir, wikiDumpPath))
	if err != nil {
		return err
	}
	defer f.Close()

	z := bzip2.NewReader(f)

	parser, err := wikiparse.NewParser(z)
	if err != nil {
		return err
	}

	// Create a new Bleve index with on-disk
	// storage in the output directory.
	mapping := blevebench.ArticleMapping()
	outputDir := filepath.Join(cfg.OutputDir, "index")
	index, err := bleve.New(outputDir, mapping)
	if err != nil {
		return err
	}
	defer func() {
		// Make sure we close the index so the data
		// persists to disk.
		cerr := index.Close()
		if err == nil {
			err = cerr
		}
	}()

	todo := ^uint64(0)
	if documents >= 0 {
		todo = uint64(documents)
	}

	// Create batches of wikipedia articles
	// and index them.
	const batchSize = 256
	b := index.NewBatch()
	for i := uint64(0); i < todo; i++ {
		p, err := parser.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if len(p.Revisions) == 0 {
			continue
		}
		b.Index(p.Title, blevebench.Article{
			Title: p.Title,
			Text:  p.Revisions[0].Text,
		})
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
	return nil
}
