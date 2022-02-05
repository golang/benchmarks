// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

const (
	genLongDesc = `Generate dynamic binary assets.`
	genUsage    = `Usage: %s gen [flags]
`
)

type genCfg struct {
	assetsDir       string
	sourceAssetsDir string
	outputDir       string
}

type genCmd struct {
	genCfg
	quiet bool
	toGen csvFlag
}

func (*genCmd) Name() string     { return "gen" }
func (*genCmd) Synopsis() string { return "Generate dynamic binary assets." }
func (*genCmd) PrintUsage(w io.Writer, base string) {
	// Print header.
	fmt.Fprintln(w, genLongDesc)

	// Print supported benchmarks.
	fmt.Fprintln(w, "\nSupported benchmarks:")
	maxBenchNameLen := 0
	for _, b := range allBenchmarks {
		l := utf8.RuneCountInString(b.name)
		if l > maxBenchNameLen {
			maxBenchNameLen = l
		}
	}
	for _, b := range allBenchmarks {
		fmt.Fprintf(w, fmt.Sprintf("  %%%ds: %%s\n", maxBenchNameLen), b.name, b.description)
	}

	// Print benchmark groups.
	fmt.Fprintln(w, "\nBenchmark groups:")
	maxGroupNameLen := 0
	var groups []string
	for groupName := range benchmarkGroups {
		l := utf8.RuneCountInString(groupName)
		if l > maxGroupNameLen {
			maxGroupNameLen = l
		}
		groups = append(groups, groupName)
	}
	sort.Strings(groups)
	for _, group := range groups {
		var groupBenchNames []string
		if group == "all" {
			groupBenchNames = []string{"all supported benchmarks"}
		} else {
			groupBenchNames = benchmarkNames(benchmarkGroups[group])
		}
		fmt.Fprintf(w, fmt.Sprintf("  %%%ds: %%s\n", maxGroupNameLen), group, strings.Join(groupBenchNames, " "))
	}

	// Print usage line. Flags will automatically be added after.
	fmt.Fprintf(w, genUsage, base)
}

func (c *genCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.genCfg.assetsDir, "assets-dir", "./assets", "the directory containing existing assets for sweet benchmarks")
	f.StringVar(&c.genCfg.sourceAssetsDir, "source-assets-dir", "./source-assets", "the directory containing source assets for some of the generators")
	f.StringVar(&c.genCfg.outputDir, "output-dir", "./assets", "the directory into which new assets should be generated")

	f.BoolVar(&c.quiet, "quiet", false, "whether to suppress activity output on stderr (no effect on -shell)")
	f.Var(&c.toGen, "gen", "benchmark group or comma-separated list of benchmarks to gen (default: all)")
}

func (c *genCmd) Run(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unexpected arguments")
	}

	log.SetActivityLog(!c.quiet)

	// Ensure all provided directories are absolute paths. This avoids problems with
	// benchmarks potentially changing their current working directory.
	var err error
	c.assetsDir, err = filepath.Abs(c.assetsDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from assets path: %w", err)
	}
	c.sourceAssetsDir, err = filepath.Abs(c.sourceAssetsDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from source assets path: %w", err)
	}
	c.outputDir, err = filepath.Abs(c.outputDir)
	if err != nil {
		return fmt.Errorf("creating absolute path from output path: %w", err)
	}

	// Make sure the assets directory is there.
	if info, err := os.Stat(c.assetsDir); os.IsNotExist(err) {
		return fmt.Errorf("assets not found at %q: forgot to run `sweet get (-copy)`?", c.assetsDir)
	} else if err != nil {
		return fmt.Errorf("stat assets %q: %v", c.assetsDir, err)
	} else if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", c.assetsDir)
	}

	// Decide which benchmarks to gen assets for, based on the -gen flag.
	var benchmarks []*benchmark
	var unknown []string
	switch len(c.toGen) {
	case 0:
		benchmarks = benchmarkGroups["all"]
	case 1:
		if grp, ok := benchmarkGroups[c.toGen[0]]; ok {
			benchmarks = grp
			break
		}
		fallthrough
	default:
		for _, name := range c.toGen {
			if benchmark, ok := allBenchmarksMap[name]; ok {
				benchmarks = append(benchmarks, benchmark)
			} else {
				unknown = append(unknown, name)
			}
		}
	}
	if len(unknown) != 0 {
		return fmt.Errorf("unknown benchmarks: %s", strings.Join(unknown, ", "))
	}

	// Find the go tool.
	goTool, err := common.SystemGoTool()
	if err != nil {
		return err
	}
	log.Printf("Using Go: %s", goTool.Tool)

	// Execute each generator.
	for _, b := range benchmarks {
		log.Printf("Generating assets: %s", b.name)
		outputDir := filepath.Join(c.outputDir, b.name)
		if err := mkdirAll(outputDir); err != nil {
			return err
		}
		// Make a copy of the Go tool so that the generator
		// is free to manipulate it without it leaking between
		// generators.
		gt := *goTool
		cfg := common.GenConfig{
			AssetsDir:       filepath.Join(c.assetsDir, b.name),
			SourceAssetsDir: filepath.Join(c.sourceAssetsDir, b.name),
			OutputDir:       outputDir,
			GoTool:          &gt,
		}
		if err := b.generator.Generate(&cfg); err != nil {
			return err
		}
	}
	return nil
}
