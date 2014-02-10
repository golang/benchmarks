// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !go1.2

package driver

import (
	"runtime"
)

// New mem stats added in Go1.2
func collectGo12MemStats(res *Result, mstats0, mstats1 *runtime.MemStats) {
}
