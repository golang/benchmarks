// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// Run as a test, reports allocation time statistics for stack, heap, and globally
// allocated buffers, out to the 99.9999th percentile.  Optionally reports worst
// allocation time if -worst is specified, but this is normally too noisy for any
// sort of trend tracking or alerting.  The default test usually runs long enough that
// it requires only one iteration.

func TestMain(m *testing.M) {
	flag.BoolVar(&reportWorstFlag, "worst", false, "report otherwise too-noisy 'worst' metric in benchmark")
	flag.Parse()
	os.Exit(m.Run())
}

type testCase struct {
	howAlloc  string
	withFluff bool
}

func BenchmarkGCLatency(b *testing.B) {
	tcs := []testCase{
		{"stack", false},
		{"stack", true},
		{"heap", false},
		{"heap", true},
		{"global", false},
		{"global", true},
	}

	for _, tc := range tcs {
		lb := &LB{doFluff: tc.withFluff, howAllocated: tc.howAlloc}
		b.Run(fmt.Sprintf("how=%s/fluff=%v", tc.howAlloc, tc.withFluff),
			func(b *testing.B) { lb.bench(b) })
	}
}
