// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package diagnostics

import (
	"flag"
	"fmt"
)

// DriverArgs returns the arguments that should be passed to a Sweet benchmark
// binary to collect data for the Config.
func (d Config) DriverArgs(resultsDir string) []string {
	flag := d.Type.AsFlag()
	args := []string{flag, resultsDir}
	if d.Flags != "" {
		args = append(args, flag+"-flags", d.Flags)
	}
	return args
}

type DriverConfig struct {
	Config
	Dir string
}

func SetFlagsForDriver(f *flag.FlagSet) map[Type]*DriverConfig {
	storage := make(map[Type]*DriverConfig)
	for _, t := range Types() {
		dc := new(DriverConfig)
		dc.Type = t
		storage[t] = dc
		f.StringVar(&dc.Dir, string(t), "", fmt.Sprintf("directory to write %s data", t))
		if t == Perf {
			f.StringVar(&dc.Flags, string(t)+"-flags", "", "flags for Linux perf")
		}
	}
	return storage
}
