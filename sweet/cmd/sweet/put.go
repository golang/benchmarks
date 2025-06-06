// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"

	"golang.org/x/benchmarks/sweet/cli/assets"
	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

const (
	putUsage = `Uploads a new version of the benchmark assets to GCS.

Usage: %s put [flags]
`
)

type putCmd struct {
	force          bool
	bucket         string
	assetsDir      string
	assetsHashFile string
	version        string
}

func (*putCmd) Name() string { return "put" }
func (*putCmd) Synopsis() string {
	return "Uploads a new version of the benchmark assets."
}
func (*putCmd) PrintUsage(w io.Writer, base string) {
	fmt.Fprintf(w, putUsage, base)
}

func (c *putCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.version, "version", common.Version, "the version to upload assets for")
	f.StringVar(&c.assetsDir, "assets-dir", "./assets", "assets directory to zip and upload")
}

func (c *putCmd) Run(_ []string) error {
	log.SetActivityLog(true)

	if err := assets.ValidateVersion(c.version); err != nil {
		return err
	}
	log.Printf("Uploading %s to CIPD and tagging with version: %s", c.assetsDir, c.version)

	// Just shell out to cipd. The put subcommand is intended to be used by an expert.
	createCmd := exec.Command("cipd", "create", "-in", c.assetsDir, "-name", "golang/sweet/assets", "-tag", "version:"+assets.ToCIPDVersion(c.version), "-compression-level", "9")
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("executing `%s`: %v", createCmd, err)
	}
	return nil
}
