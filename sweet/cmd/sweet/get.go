// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/cli/assets"
	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	cipdc "go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/hardcoded/chromeinfra"
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
}

func (c *getCmd) Run(_ []string) error {
	log.SetActivityLog(true)
	ctx := context.Background()

	// Load CIPD options, including auth, cache dir, etc. from env. The package is public, but we
	// want to be authenticated transparently when we pull the assets down on the builders.
	var opts cipd.ClientOptions
	if err := opts.LoadFromEnv(ctx); err != nil {
		return err
	}
	if opts.ServiceURL == "" {
		opts.ServiceURL = chromeinfra.CIPDServiceURL
	}
	// Use an existing CIPD cache in the environment, if available.
	// Otherwise, set up the default.
	if opts.CacheDir == "" {
		opts.CacheDir = filepath.Join(c.cache, assets.CIPDCacheDir)
	}

	// Figure out the destination directory.
	var ensureOpts cipd.EnsureOptions
	ensureOpts.Paranoia = cipd.CheckIntegrity
	if c.copyDir != "" {
		ensureOpts.OverrideInstallMode = pkg.InstallModeCopy
		opts.Root = c.copyDir
	} else {
		assetsDir, err := assets.CachedAssets(c.cache, c.version)
		if err == nil {
			// Nothing to do.
			return nil
		}
		if err != assets.ErrNotInCache {
			return err
		}
		opts.Root = assetsDir
	}

	// Find the assets by version.
	cc, err := cipd.NewClient(opts)
	if err != nil {
		return err
	}
	defer cc.Close(ctx)
	pins, err := cc.SearchInstances(ctx, "golang/sweet/assets", []string{"version:" + assets.ToCIPDVersion(c.version)})
	if err != nil {
		return err
	}
	if len(pins) == 0 {
		return fmt.Errorf("unable to find CIPD package instance for version %s", c.version)
	}

	log.Printf("Fetching assets %s", c.version)

	// Fetch the instance.
	_, err = cc.EnsurePackages(ctx, map[string]cipdc.PinSlice{"": pins[:1]}, &ensureOpts)
	if err != nil {
		return fmt.Errorf("fetching CIPD package instance %s: %v", pins[0], err)
	}
	return nil
}
