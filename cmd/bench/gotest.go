// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"path/filepath"
)

func goTest(tcs []*toolchain) error {
	for _, tc := range tcs {
		log.Printf("Running Go test benchmarks for %s", tc.Name)
		fmt.Printf("toolchain: %s\n", tc.Name)
		err := tc.Do("", "test", "-v", "-run=none", "-bench=.", "-count=5", "golang.org/x/benchmarks/...")
		if err != nil {
			return fmt.Errorf("error running gotest with toolchain %s: %w", tc.Name, err)
		}
	}
	return nil
}

func goTestSubrepo(tc *toolchain, subRepo string, dirs []string) error {
	switch subRepo {
	case "tools":
		log.Printf("Running sub-repo benchmarks for %s", subRepo)
		fmt.Printf("toolchain: %s\n", tc.Name)

		for _, dir := range dirs {
			err := tc.Do(filepath.Join(dir, "gopls"), "test", "-v", "-bench=BenchmarkGoToDefinition", "./internal/regtest/bench/", "-count=5")
			if err != nil {
				log.Printf("Error: %v", err)
				return fmt.Errorf("error running sub-repo %s benchmark with toolchain %s in dir %s: %w", subRepo, tc.Name, dir, err)
			}
		}
	default:
		return fmt.Errorf("unsupported subrepo %s", subRepo)
	}

	return nil
}
