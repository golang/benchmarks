// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.5

package driver

import (
	"flag"
	"fmt"
	"os"
	"runtime/trace"
)

var traceFile = flag.String("trace", "", "write an execution trace to the named file after execution")

func startTraceGo15() func() {
	// New runtime tracing added in Go 1.5.
	if *traceFile != "" {
		f, err := os.Create(*traceFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := trace.Start(f); err != nil {
			fmt.Fprintf(os.Stderr, "can't start tracing: %s\n", err)
			os.Exit(1)
		}

		return func() {
			trace.Stop()
			f.Close()
		}
	}
	return func() {}
}

func init() {
	startTrace = startTraceGo15
}
