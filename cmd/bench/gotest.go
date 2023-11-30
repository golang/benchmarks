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

// goTestSubrepo runs subrepo tests for the subrepo x/<subRepo>, where the
// baseline commit is checked out in baselineDir, and the experiment commit is
// checked out in experimentDir.
//
// To run subrepo tests locally, use this incantation:
//
//	./bench -wait=false \
//	 	 -repository=tools \
//	   -subrepo=/path/to/experiment \
//	   -subrepo-baseline=/path/to/baseline \
//	   -goroot-baseline=$(go env GOROOT)
func goTestSubrepo(tc *toolchain, subRepo, baselineDir, experimentDir string) error {
	// Note: tests must write "toolchain: baseline|experiment" to stdout before
	// running benchmarks for the corresponding commit, as this is parsed as a
	// benchmark tag.
	switch subRepo {
	case "tools":
		log.Printf("Running sub-repo benchmarks for %s", subRepo)

		// Gopls tests run benchmarks defined in the experimentDir (i.e. the latest
		// versions of the benchmarks themselves).
		//
		// Benchmarks all communicate with a separate gopls process, which can be
		// configured via the -gopls_path flag.
		//
		// By convention, gopls uses -short to define tests that should be run by
		// x/benchmarks.
		benchmarkDir := filepath.Join(experimentDir, "gopls", "internal", "test", "integration", "bench")

		tests := []struct {
			name     string // toolchain name
			goplsDir string // dir where gopls should be built
		}{
			{"baseline", filepath.Join(baselineDir, "gopls")},
			{"experiment", filepath.Join(experimentDir, "gopls")},
		}

		for _, test := range tests {
			err := tc.Do(test.goplsDir, "build")
			if err != nil {
				return fmt.Errorf("error building sub-repo %s with toolchain %s in dir %s: %w", subRepo, tc.Name, test.goplsDir, err)
			}

			fmt.Printf("toolchain: %s\n", test.name) // set the toolchain tag

			goplsPath := filepath.Join(test.goplsDir, "gopls")
			err = tc.Do(benchmarkDir, "test", "-short", "-bench=.", fmt.Sprintf(`-gopls_path=%s`, goplsPath), "-count=5", "-timeout=180m")
			if err != nil {
				return fmt.Errorf("error running sub-repo %s benchmark %q with toolchain %s in dir %s: %w", subRepo, test.name, tc.Name, benchmarkDir, err)
			}
		}

	default:
		return fmt.Errorf("unsupported subrepo %s", subRepo)
	}

	return nil
}
