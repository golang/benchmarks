// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"

	"gitlab.com/golang-commonmark/markdown"
)

func run(mddir string) error {
	files, err := os.ReadDir(mddir)
	if err != nil {
		return err
	}

	contents := make([][]byte, 0, len(files))
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".md" {
			content, err := os.ReadFile(filepath.Join(mddir, file.Name()))
			if err != nil {
				return err
			}
			contents = append(contents, content)
		}
	}

	out := bytes.Buffer{}
	out.Grow(1024 * 1024)

	md := markdown.New(
		markdown.XHTMLOutput(true),
		markdown.Tables(true),
		markdown.MaxNesting(8),
		markdown.Typographer(true),
		markdown.Linkify(true),
	)

	return driver.RunBenchmark("MarkdownRenderXHTML", func(_ *driver.B) error {
		for _, c := range contents {
			md.Render(&out, c)
			out.Reset()
		}
		return nil
	}, driver.InProcessMeasurementOptions...)
}

func main() {
	driver.SetFlags(flag.CommandLine)
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "expected asset directory as input")
		os.Exit(1)
	}
	if err := run(flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
