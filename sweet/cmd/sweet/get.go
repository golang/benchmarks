// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"

	"golang.org/x/benchmarks/sweet/cli/assets"
	"golang.org/x/benchmarks/sweet/common"
)

const (
	getUsage = `Retrieves assets for benchmarks from GCS.

Usage: %s get [flags]
`
)

type getCmd struct {
	cache   string
	copyDir string
	version string
	clean   bool
}

func (*getCmd) Name() string     { return "get" }
func (*getCmd) Synopsis() string { return "Retrieves assets for benchmarks." }
func (*getCmd) PrintUsage(w io.Writer, base string) {
	fmt.Fprintf(w, getUsage, base)
}

func (c *getCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.cache, "cache", assets.CacheDefault(), "cache location for assets")
	f.StringVar(&c.version, "version", common.Version, "the version to download assets for")
	f.StringVar(&c.copyDir, "copy", "", "location to extract assets into, useful for development")
	f.BoolVar(&c.clean, "clean", false, "delete all cached assets before installing new ones")
}
