// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

const ConfigHelp = `
The input configuration format is TOML consisting of a single array field
called 'config'. Each element of the array consists of the following fields:
         name: a unique name for the configuration (required)
       goroot: path to a GOROOT representing the toolchain to run (required)
     envbuild: additional environment variables that should be used for
               compilation each variable should take the form "X=Y" (optional)
      envexec: additional environment variables that should be used for execution
               each variable should take the form "X=Y" (optional)
     pgofiles: a map of benchmark names (see 'sweet help run') to profile files
               to be passed to the Go compiler for optimization (optional)
  diagnostics: profile types to collect for each benchmark run of this
               configuration, which may be one of: cpuprofile, memprofile,
               perf[=flags], trace (optional)

A simple example configuration might look like:

[[config]]
  name = "original"
  goroot = "/path/to/go"

[[config]]
  name = "improved"
  goroot = "/path/to/go-but-better"

Note that because 'config' is an array field, one may have multiple
configurations present in a single file.

An example of using some of the other fields to diagnose performance differences:

[[config]]
  name = "improved-but-why"
  goroot = "/path/to/go-but-better"
  envexec = ["GODEBUG=gctrace=1"]
  diagnostics = ["cpuprofile", "perf=-e page-faults"]
`

type ConfigFile struct {
	Configs []*Config `toml:"config"`
}

type Config struct {
	Name        string                `toml:"name"`
	GoRoot      string                `toml:"goroot"`
	BuildEnv    ConfigEnv             `toml:"envbuild"`
	ExecEnv     ConfigEnv             `toml:"envexec"`
	PGOFiles    map[string]string     `toml:"pgofiles"`
	Diagnostics diagnostics.ConfigSet `toml:"diagnostics"`
}

func (c *Config) GoTool() *Go {
	return &Go{
		Tool: filepath.Join(c.GoRoot, "bin", "go"),
		// Update the GOROOT so the wrong one doesn't propagate from
		// the environment.
		Env: c.BuildEnv.Env.MustSet("GOROOT=" + c.GoRoot),
	}
}

// Copy returns a deep copy of Config.
func (c *Config) Copy() *Config {
	cc := *c
	cc.PGOFiles = make(map[string]string)
	for k, v := range c.PGOFiles {
		cc.PGOFiles[k] = v
	}
	cc.Diagnostics = c.Diagnostics.Copy()
	return &cc
}

func ConfigFileMarshalTOML(c *ConfigFile) ([]byte, error) {
	// Unfortunately because the github.com/BurntSushi/toml
	// package at v1.0.0 doesn't correctly support Marshaler
	// (see https://github.com/BurntSushi/toml/issues/341)
	// we can't actually implement Marshaler for ConfigEnv.
	// So instead we work around this by implementing MarshalTOML
	// on Config and use dummy types that have a straightforward
	// mapping that *does* work.
	type config struct {
		Name        string            `toml:"name"`
		GoRoot      string            `toml:"goroot"`
		BuildEnv    []string          `toml:"envbuild"`
		ExecEnv     []string          `toml:"envexec"`
		PGOFiles    map[string]string `toml:"pgofiles"`
		Diagnostics []string          `toml:"diagnostics"`
	}
	type configFile struct {
		Configs []*config `toml:"config"`
	}
	var cfgs configFile
	for _, c := range c.Configs {
		var cfg config
		cfg.Name = c.Name
		cfg.GoRoot = c.GoRoot
		cfg.BuildEnv = c.BuildEnv.Collapse()
		cfg.ExecEnv = c.ExecEnv.Collapse()
		cfg.PGOFiles = c.PGOFiles
		cfg.Diagnostics = c.Diagnostics.Strings()

		cfgs.Configs = append(cfgs.Configs, &cfg)
	}
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(&cfgs); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
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
