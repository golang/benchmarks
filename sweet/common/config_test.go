// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common_test

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"golang.org/x/benchmarks/sweet/common"
)

func TestConfigMarshalTOML(t *testing.T) {
	cfgsBefore := common.ConfigFile{
		Configs: []*common.Config{
			&common.Config{
				Name:   "go",
				GoRoot: "/path/to/my/goroot",
				// The unmarashaler propagates the environment,
				// so to make sure this works, let's also seed
				// from the environment.
				BuildEnv: common.ConfigEnv{common.NewEnvFromEnviron()},
				ExecEnv:  common.ConfigEnv{common.NewEnvFromEnviron()},
			},
		},
	}
	b, err := common.ConfigFileMarshalTOML(&cfgsBefore)
	if err != nil {
		t.Fatal(err)
	}
	var cfgsAfter common.ConfigFile
	if err := toml.Unmarshal(b, &cfgsAfter); err != nil {
		t.Fatal(err)
	}
	if l := len(cfgsAfter.Configs); l != len(cfgsBefore.Configs) {
		t.Fatalf("unexpected number of configs: got %d, want %d", l, len(cfgsBefore.Configs))
	}
	for i := range cfgsAfter.Configs {
		cfgBefore := cfgsBefore.Configs[i]
		cfgAfter := cfgsAfter.Configs[i]

		if cfgBefore.Name != cfgAfter.Name {
			t.Fatalf("unexpected name: got %s, want %s", cfgAfter.Name, cfgBefore.Name)
		}
		if cfgBefore.GoRoot != cfgAfter.GoRoot {
			t.Fatalf("unexpected GOROOT: got %s, want %s", cfgAfter.GoRoot, cfgBefore.GoRoot)
		}
		compareEnvs(t, cfgBefore.BuildEnv.Env, cfgAfter.BuildEnv.Env)
		compareEnvs(t, cfgBefore.ExecEnv.Env, cfgAfter.ExecEnv.Env)
	}
}

func compareEnvs(t *testing.T, a, b *common.Env) {
	t.Helper()

	aIndex := makeEnvIndex(a)
	bIndex := makeEnvIndex(b)
	for aKey, aVal := range aIndex {
		if bVal, ok := bIndex[aKey]; !ok {
			t.Errorf("%s in A but not B", aKey)
		} else if aVal != bVal {
			t.Errorf("%s has value %s A but %s in B", aKey, aVal, bVal)
		}
	}
	for bKey := range bIndex {
		if _, ok := aIndex[bKey]; !ok {
			t.Errorf("%s in B but not A", bKey)
		}
		// Don't check values that exist in both. We got that already
		// in the first pass.
	}
}

func makeEnvIndex(a *common.Env) map[string]string {
	index := make(map[string]string)
	for _, s := range a.Collapse() {
		d := strings.IndexRune(s, '=')
		index[s[:d]] = s[d+1:]
	}
	return index
}
