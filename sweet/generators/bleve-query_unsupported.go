// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build aix || plan9
// +build aix plan9

package generators

import (
	"fmt"
	"runtime"

	"golang.org/x/benchmarks/sweet/common"
)

// BleveQuery is a dynamic assets Generator for the bleve-query benchmark.
type BleveQuery struct{}

// Generate cannot run on these platforms.
func (_ BleveQuery) Generate(_ *common.GenConfig) (err error) {
	return fmt.Errorf("platform %s/%s unsupported", runtime.GOOS, runtime.GOARCH)
}
