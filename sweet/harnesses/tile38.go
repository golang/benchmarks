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

func (h Tile38) Get(gcfg *common.GetConfig) error {
	return gitShallowClone(
		gcfg.SrcDir,
		"https://github.com/tidwall/tile38",
		"1.29.1",
	)
}

func (h Tile38) Build(cfg *common.Config, bcfg *common.BuildConfig) error {
	env := cfg.BuildEnv.Env

	// Add the Go tool to PATH, since tile38's Makefile doesn't provide enough
	// visibility into how tile38 is built to allow us to pass this information
	// directly. Also set the GOROOT explicitly because it might have propagated
	// differently from the environment.
	env = env.Prefix("PATH", filepath.Join(cfg.GoRoot, "bin")+":")
	env = env.MustSet("GOROOT=" + cfg.GoRoot)

	cmd := exec.Command("make", "-C", bcfg.SrcDir)
	cmd.Env = env.Collapse()
	log.TraceCommand(cmd, false)
	// Call Output here to get an *ExitError with a populated Stderr field.
	if _, err := cmd.Output(); err != nil {
		return err
	}
	// Note that no matter what we do, the build script insists on putting the
	// binaries into the source directory, so copy the one we care about into
	// BinDir.
	if err := copyFile(filepath.Join(bcfg.BinDir, server), filepath.Join(bcfg.SrcDir, server)); err != nil {
		return err
	}
	return cfg.GoTool(bcfg.BuildLog).BuildPath(bcfg.BenchDir, filepath.Join(bcfg.BinDir, "tile38-bench"))
}

func (h Tile38) Run(cfg *common.Config, rcfg *common.RunConfig) error {
	var dataPath string
	if rcfg.Short {
		// Don't load the real data for short mode. It takes a long time.
		dataPath = filepath.Join(rcfg.TmpDir, "data-empty-fake")
	} else {
		// Make sure all the data passed to the server is writable.
		// The server needs to be able to open its persistent storage as read-write.
		dataPath = filepath.Join(rcfg.AssetsDir, "data")
		if err := makeWriteable(dataPath); err != nil {
			return err
		}
	}
	args := append(rcfg.Args, []string{
		"-host", "127.0.0.1",
		"-port", "9851",
		"-server", filepath.Join(rcfg.BinDir, server),
		"-data", dataPath,
		"-tmp", rcfg.TmpDir,
	}...)
	if rcfg.Short {
		args = append(args, "-short")
	}
	cmd := exec.Command(
		filepath.Join(rcfg.BinDir, "tile38-bench"),
		args...,
	)
	cmd.Env = cfg.ExecEnv.Collapse()
	cmd.Stdout = rcfg.Results
	cmd.Stderr = rcfg.Log
	log.TraceCommand(cmd, false)
	return cmd.Run()
}
