// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package diagnostics

import (
	"flag"
	"fmt"
)

// DriverConfig is a diagnostics configuration that can be passed to a benchmark
// driver by serializing to and from command-line flags.
type DriverConfig struct {
	ConfigSet
	ResultsDir string
}

// DriverArgs returns the arguments that should be passed to a Sweet benchmark
// binary to collect data for c.
func (c *DriverConfig) DriverArgs() []string {
	args := []string{"-results-dir", c.ResultsDir}
	for _, c1 := range c.cfgs {
		args = append(args, "-"+string(c1.Type))
		if c1.Type == Perf {
			// String flag
			args = append(args, c1.Flags)
		}
	}
	return args
}

// AddFlags populates f with flags that will fill in c.
func (c *DriverConfig) AddFlags(f *flag.FlagSet) {
	*c = DriverConfig{}
	c.ConfigSet.cfgs = make(map[Type]Config)

	f.StringVar(&c.ResultsDir, "results-dir", "", "directory to write diagnostics data")
	for _, t := range Types() {
		t := t
		if t == Perf {
			f.Func(string(t), fmt.Sprintf("enable %s diagnostics with `flags`", t), func(s string) error {
				c.cfgs[t] = Config{Type: t, Flags: s}
				return nil
			})
		} else {
			f.BoolFunc(string(t), fmt.Sprintf("enable %s diagnostics", t), func(s string) error {
				c.cfgs[t] = Config{Type: t}
				return nil
			})
		}
	}
}
