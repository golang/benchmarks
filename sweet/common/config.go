// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import (
	"fmt"
	"path/filepath"
)

const ConfigHelp = `
The input configuration format is TOML consisting of a single array field
called 'config'. Each element of the array consists of the following fields:
      name: a unique name for the configuration (required)
    goroot: path to a GOROOT representing the toolchain to run (required)
  envbuild: additional environment variables that should be used for compilation
            each variable should take the form "X=Y" (optional)
   envexec: additional environment variables that should be used for execution
            each variable should take the form "X=Y" (optional)

A simple example configuration might look like:

[[config]]
  name = "original"
  goroot = "~/work/go"
  envexec = ["GODEBUG=gctrace=1"]

[[config]]
  name = "improved"
  goroot = "~/work/go-but-better"
  envexec = ["GODEBUG=gctrace=1"]

Note that because 'config' is an array field, one may have multiple
configurations present in a single file.
`

type ConfigFile struct {
	Configs []*Config `toml:"config"`
}

type Config struct {
	Name     string    `toml:"name"`
	GoRoot   string    `toml:"goroot"`
	BuildEnv ConfigEnv `toml:"envbuild"`
	ExecEnv  ConfigEnv `toml:"envexec"`
}

func (c *Config) GoTool() *Go {
	return &Go{
		Tool: filepath.Join(c.GoRoot, "bin", "go"),
		Env:  c.BuildEnv.Env,
	}
}

type ConfigEnv struct {
	*Env
}

func (c *ConfigEnv) UnmarshalTOML(data interface{}) error {
	ldata, ok := data.([]interface{})
	if !ok {
		return fmt.Errorf("expected data for env to be a list")
	}
	vars := make([]string, 0, len(ldata))
	for _, d := range ldata {
		s, ok := d.(string)
		if !ok {
			return fmt.Errorf("expected data for env to contain strings")
		}
		vars = append(vars, s)
	}
	var err error
	c.Env = NewEnvFromEnviron()
	c.Env, err = c.Env.Set(vars...)
	return err
}
