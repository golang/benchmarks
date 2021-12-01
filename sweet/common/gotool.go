// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import (
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/benchmarks/sweet/common/log"
)

type Go struct {
	Tool string
	Env  *Env
}

func SystemGoTool() (*Go, error) {
	tool, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("looking for system Go: %v", err)
	}
	return &Go{
		Tool: tool,
		Env:  NewEnvFromEnviron(),
	}, nil
}

func (g *Go) run(args ...string) error {
	cmd := exec.Command(g.Tool, args...)
	cmd.Env = g.Env.Collapse()
	log.TraceCommand(cmd, false)
	return cmd.Run()
}

func (g *Go) BuildPackage(pkg, out string) error {
	if pkg[0] == '/' || pkg[0] == '.' {
		return fmt.Errorf("path used as package in go build")
	}
	return g.build(pkg, out)
}

func (g *Go) BuildPath(path, out string) error {
	if path[0] != '/' && path[0] != '.' {
		path = "./" + path
	}
	return g.build(path, out)
}

func (g *Go) build(pkgOrPath, out string) (err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %v", err)
	}
	defer chdir(cwd)
	if err := chdir(pkgOrPath); err != nil {
		return fmt.Errorf("failed to enter build directory: %v", err)
	}
	return g.run("build", "-o", out)
}

func chdir(path string) error {
	log.CommandPrintf("cd %s", path)
	return os.Chdir(path)
}
