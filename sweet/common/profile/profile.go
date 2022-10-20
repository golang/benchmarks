// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package profile supports working with pprof profiles.
package profile

import (
	"os"
	"path/filepath"

	"github.com/google/pprof/profile"
)

func Read(filename string) (*profile.Profile, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return profile.Parse(f)
}

// ReadDir reads all profiles in dir whose name matches match(name).
func ReadDir(dir string, match func(string) bool) ([]*profile.Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var profiles []*profile.Profile
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		if info, err := entry.Info(); err != nil {
			return nil, err
		} else if info.Size() == 0 {
			// Skip zero-sized files, otherwise the pprof package
			// will call it a parsing error.
			continue
		}
		if match(name) {
			p, err := Read(path)
			if err != nil {
				return nil, err
			}
			profiles = append(profiles, p)
			continue
		}
	}
	return profiles, nil
}
