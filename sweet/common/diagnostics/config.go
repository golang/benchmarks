// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package diagnostics

import (
	"fmt"
	"strings"
)

// ConfigSet is an immutable set of Config, containing at most
// one Config of each supported type.
type ConfigSet struct {
	cfgs map[Type]Config
}

// Strings returns the set of ConfigSet as strings by calling the String
// method on each Config.
func (c ConfigSet) Strings() []string {
	var diags []string
	for _, diag := range c.cfgs {
		diags = append(diags, diag.String())
	}
	return diags
}

// UnmarshalTOML implements TOML unmarshaling for ConfigSet.
func (c *ConfigSet) UnmarshalTOML(data interface{}) error {
	ldata, ok := data.([]interface{})
	if !ok {
		return fmt.Errorf("expected data for diagnostics to be a list")
	}
	cfgs := make(map[Type]Config, len(ldata))
	for _, li := range ldata {
		s, ok := li.(string)
		if !ok {
			return fmt.Errorf("expected data for env to contain strings")
		}
		d, err := ParseConfig(s)
		if err != nil {
			return err
		}
		cfgs[d.Type] = d
	}
	c.cfgs = cfgs
	return nil
}

// Copy creates a deep clone of a ConfigSet.
func (c ConfigSet) Copy() ConfigSet {
	cfgs := make(map[Type]Config, len(c.cfgs))
	for k, v := range c.cfgs {
		cfgs[k] = v
	}
	return ConfigSet{cfgs}
}

// Set adds a Config to ConfigSet, overwriting any Config of the same Type.
func (c *ConfigSet) Set(d Config) {
	c.cfgs[d.Type] = d
}

// Clear removes the Config with the provided Type from the ConfigSet, if applicable.
func (c *ConfigSet) Clear(typ Type) {
	delete(c.cfgs, typ)
}

// Get looks up the Config with the provided Type and returns it if it exists with the
// second result indicating presence.
func (c ConfigSet) Get(typ Type) (Config, bool) {
	cfg, ok := c.cfgs[typ]
	return cfg, ok
}

// Empty returns true if the ConfigSet is empty.
func (c ConfigSet) Empty() bool {
	return len(c.cfgs) == 0
}

// ToSlice returns each Config contained in the ConfigSet in a slice.
func (c ConfigSet) ToSlice() []Config {
	cfgs := make([]Config, 0, len(c.cfgs))
	for _, cfg := range c.cfgs {
		cfgs = append(cfgs, cfg)
	}
	return cfgs
}

// Type is a diagnostic type supported by Sweet.
type Type string

const (
	CPUProfile Type = "cpuprofile"
	MemProfile Type = "memprofile"
	Perf       Type = "perf"
	Trace      Type = "trace"
)

// IsPprof returns whether the diagnostic's data is stored in the pprof format.
func (t Type) IsPprof() bool {
	return t == CPUProfile || t == MemProfile
}

// AsFlag returns the Type suitable for use as a CLI flag.
func (t Type) AsFlag() string {
	return "-" + string(t)
}

// Types returns a slice of all supported types.
func Types() []Type {
	return []Type{
		CPUProfile,
		MemProfile,
		Perf,
		Trace,
	}
}

// Config is an intent to collect data for some diagnostic with some room
// for additional configuration as to how that data is collected.
type Config struct {
	// Type is the diagnostic to collect data for.
	Type

	// Flags is additional opaque configuration for data collection.
	//
	// Currently only used if Type == Perf.
	Flags string
}

// String returns the string representation of a Config, as it would appear
// in a Sweet common.Config.
func (d Config) String() string {
	result := string(d.Type)
	if d.Flags != "" {
		result += "=" + d.Flags
	}
	return result
}

// ParseConfig derives a Config from a string. The string must take the form
//
//    <type>[=<flags>]
//
// where [=<flags>] is only accepted if <type> is perf.
func ParseConfig(d string) (Config, error) {
	comp := strings.SplitN(d, "=", 2)
	var result Config
	switch comp[0] {
	case string(CPUProfile):
		fallthrough
	case string(MemProfile):
		fallthrough
	case string(Trace):
		if len(comp) != 1 {
			return result, fmt.Errorf("diagnostic %q does not take flags", comp[0])
		}
		result.Type = Type(comp[0])
	case string(Perf):
		if len(comp) == 2 {
			result.Flags = comp[1]
		}
		result.Type = Type(comp[0])
	default:
		return result, fmt.Errorf("invalid diagnostic %q", comp[0])
	}
	return result, nil
}
