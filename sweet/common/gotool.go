// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common/log"
)

type Go struct {
	Tool       string
	Env        *Env
	PassOutput bool
	BuildLog   io.Writer
}

func SystemGoTool() (*Go, error) {
	tool, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("looking for system Go: %w", err)
	}
	return &Go{
		Tool: tool,
		Env:  NewEnvFromEnviron().MustSet("GOROOT=" + filepath.Dir(filepath.Dir(tool))),
	}, nil
}

func (g *Go) Do(dir string, args ...string) error {
	cmd := exec.Command(g.Tool, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = g.Env.Collapse()
	if g.PassOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else if g.BuildLog != nil {
		cmd.Stderr = g.BuildLog
		cmd.Stdout = g.BuildLog
	}
	log.TraceCommand(cmd, false)
	if g.PassOutput || g.BuildLog != nil {
		return cmd.Run()
	}
	// Use cmd.Output to get an ExitError with Stderr populated.
	_, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		// ExitError includes stderr, but doesn't inclue it in Error.
		// Create a new error that does display stderr.
		return fmt.Errorf("%w. stderr:\n%s", err, ee.Stderr)
	}
	return err
}

func (g *Go) List(args ...string) ([]byte, error) {
	cmd := exec.Command(g.Tool, append([]string{"list"}, args...)...)
	cmd.Env = g.Env.Collapse()
	log.TraceCommand(cmd, false)
	return cmd.Output()
}

func (g *Go) GOROOT() string {
	return filepath.Dir(filepath.Dir(g.Tool))
}

func (g *Go) BuildPackage(pkg, out string) error {
	if pkg[0] == '/' || pkg[0] == '.' {
		return fmt.Errorf("path used as package in go build")
	}
	return g.Do("", "build", "-o", out, pkg)
}

func (g *Go) Version() (string, error) {
	cmd := exec.Command(g.Tool, "version")
	cmd.Env = g.Env.Collapse()
	log.TraceCommand(cmd, false)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error running 'go version': %w", err)
	}
	return string(out), nil
}

func (g *Go) BuildPath(path, out string, args ...string) error {
	if path[0] != '/' && path[0] != '.' {
		path = "./" + path
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}
	defer chdir(cwd)
	if err := chdir(path); err != nil {
		return fmt.Errorf("failed to enter build directory: %w", err)
	}
	args = append([]string{"build", "-o", out}, args...)
	return g.Do("", args...)
}

func chdir(path string) error {
	log.CommandPrintf("cd %s", path)
	return os.Chdir(path)
}
