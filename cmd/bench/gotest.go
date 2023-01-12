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

func goTestSubrepo(tc *toolchain, subRepo, baselineDir, experimentDir string) error {
	switch subRepo {
	case "tools":
		log.Printf("Running sub-repo benchmarks for %s", subRepo)

		// Build baseline gopls binary to run benchmark on
		goplsBaseline := filepath.Join(baselineDir, "gopls")
		err := tc.Do(goplsBaseline, "build")
		if err != nil {
			log.Printf("Error: %v", err)
			return fmt.Errorf("error building sub-repo %s with toolchain %s in dir %s: %w", subRepo, tc.Name, baselineDir, err)
		}

		// Build experiment gopls binary to run benchmark on
		goplsExperiment := filepath.Join(experimentDir, "gopls")
		err = tc.Do(goplsExperiment, "build")
		if err != nil {
			log.Printf("Error: %v", err)
			return fmt.Errorf("error building sub-repo %s with toolchain %s in dir %s: %w", subRepo, tc.Name, experimentDir, err)
		}

		fmt.Println("toolchain: baseline")
		err = tc.Do(goplsExperiment, "test", "-v", "-bench=BenchmarkGoToDefinition", "./internal/regtest/bench/", fmt.Sprintf(`-gopls_path=%s`, filepath.Join(goplsBaseline, "gopls")), "-count=5")
		if err != nil {
			log.Printf("Error: %v", err)
			return fmt.Errorf("error running sub-repo %s benchmark with toolchain %s in dir %s: %w", subRepo, tc.Name, baselineDir, err)
		}

		fmt.Println("toolchain: experiment")
		err = tc.Do(goplsExperiment, "test", "-v", "-bench=BenchmarkGoToDefinition", "./internal/regtest/bench/", fmt.Sprintf(`-gopls_path=%s`, filepath.Join(goplsExperiment, "gopls")), "-count=5")
		if err != nil {
			log.Printf("Error: %v", err)
			return fmt.Errorf("error running sub-repo %s benchmark with toolchain %s in dir %s: %w", subRepo, tc.Name, experimentDir, err)
		}
	default:
		return fmt.Errorf("unsupported subrepo %s", subRepo)
	}

	return nil
}
