// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

const (
	server = "tile38-server"
)

type Tile38 struct{}

func (h Tile38) CheckPrerequisites() error {
	return nil
}

func (h Tile38) Get(srcDir string) error {
	return gitShallowClone(
		srcDir,
		"https://github.com/tidwall/tile38",
		"1.25.3",
	)
}

func (h Tile38) Build(cfg *common.Config, bcfg *common.BuildConfig) error {
	env := cfg.BuildEnv.Env

	// Add the Go tool to PATH, since tile38's Makefile doesn't provide enough
	// visibility into how tile38 is built to allow us to pass this information
	// directly.
	env = env.Prefix("PATH", filepath.Join(cfg.GoRoot, "bin")+":")

	cmd := exec.Command("make", "-C", bcfg.SrcDir)
	cmd.Env = env.Collapse()
	log.TraceCommand(cmd, false)
	if err := cmd.Run(); err != nil {
		return err
	}
	// Note that no matter what we do, the build script insists on putting the
	// binaries into the source directory, so copy the one we care about into
	// BinDir.
	if err := copyFile(filepath.Join(bcfg.BinDir, server), filepath.Join(bcfg.SrcDir, server)); err != nil {
		return err
	}
	return cfg.GoTool().BuildPath(bcfg.BenchDir, filepath.Join(bcfg.BinDir, "tile38-bench"))
}

func (h Tile38) Run(cfg *common.Config, rcfg *common.RunConfig) error {
	// Make sure all the data passed to the server is writable.
	// The server needs to be able to open its persistent storage as read-write.
	dataPath := filepath.Join(rcfg.AssetsDir, "data")
	if err := makeWriteable(dataPath); err != nil {
		return err
	}
	cmd := exec.Command(
		filepath.Join(rcfg.BinDir, "tile38-bench"),
		append(rcfg.Args, []string{
			"-host", "127.0.0.1",
			"-port", "9851",
			"-server", filepath.Join(rcfg.BinDir, server),
			"-data", dataPath,
			"-tmp", rcfg.TmpDir,
		}...)...,
	)
	cmd.Env = cfg.ExecEnv.Collapse()
	cmd.Stdout = rcfg.Results
	cmd.Stderr = rcfg.Results
	log.TraceCommand(cmd, false)
	return cmd.Run()
}
