// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import "os"

type GetConfig struct {
	// SrcDir is the path to the directory that the harness should write
	// benchmark source into. This is then fed into the BuildConfig.
	//
	// The harness should not write benchmark source code from the Sweet
	// repository here; it need only collect source code it needs to fetch
	// from a remote source.
	SrcDir string

	// Short indicates whether or not to run a short version of the benchmarks
	// for testing. Guaranteed to be the same as BuildConfig.Short and
	// RunConfig.Short.
	Short bool
}

type BuildConfig struct {
	// BinDir is the path to the directory where all built binaries should be
	// placed.
	BinDir string

	// SrcDir is the path to the directory containing the benchmark's source
	// code, excluding benchmark code that is part of the Sweet repository.
	//
	// For instance, this directory would contain a pulled source repository.
	SrcDir string

	// BenchDir is the path to the benchmark's source directory in the Sweet
	// repository.
	BenchDir string

	// Short indicates whether or not to run a short version of the benchmarks
	// for testing. Guaranteed to be the same as GetConfig.Short and
	// RunConfig.Short.
	Short bool
}

type RunConfig struct {
	// BinDir is the path to the directory containing the benchmark
	// binaries.
	BinDir string

	// TmpDir is the path to a dedicated scratch directory.
	//
	// This directory is empty at the beginning of each run.
	TmpDir string

	// AssetsDir is the path to the directory containing runtime assets
	// for the benchmark.
	//
	// AssistsDir is reconstructed for each run, so files within are safe
	// to mutate.
	AssetsDir string

	// Args is a set of additional command-line arguments to pass to the
	// primary benchmark binary (e.g. -dump-cores).
	//
	// The purpose of this field is to plumb through flags that all
	// benchmarks support, such as flags for generating CPU profiles and
	// such.
	Args []string

	// Results is the file to which benchmark results should be appended
	// in the Go benchmark format.
	//
	// By convention, this is the benchmark binary's stdout.
	Results *os.File

	// Log contains additional information information from the benchmark,
	// for example for debugging.
	//
	// By convention, this is the benchmark binary's stderr.
	Log *os.File

	// Short indicates whether or not to run a short version of the benchmarks
	// for testing. Guaranteed to be the same as GetConfig.Short and
	// BuildConfig.Short.
	Short bool
}

type Harness interface {
	// CheckPrerequisites checks benchmark-specific environment prerequisites
	// such as whether we're running as root or on a specific platform.
	//
	// Returns an error if any prerequisites are not met, nil otherwise.
	CheckPrerequisites() error

	// Get retrieves the source code for a benchmark and places it in srcDir.
	Get(g *GetConfig) error

	// Build builds a benchmark and places the binaries in binDir.
	Build(cfg *Config, b *BuildConfig) error

	// Run runs the given benchmark and writes Go `testing` formatted benchmark
	// output to `results`.
	Run(cfg *Config, r *RunConfig) error
}
