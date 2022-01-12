// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
)

func goTest(tcs []*toolchain) error {
	for _, tc := range tcs {
		log.Printf("Running Go test benchmarks for %s", tc.Name)
		fmt.Printf("toolchain: %s\n", tc.Name)
		err := tc.Do("test", "-v", "-run=none", "-bench=.", "-count=5", "golang.org/x/benchmarks/...")
		if err != nil {
			return fmt.Errorf("error running gotest with toolchain %s: %w", tc.Name, err)
		}
	}
	return nil
}
