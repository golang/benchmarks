// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasm || plan9

package main

import (
	"errors"

	"golang.org/x/benchmarks/sweet/common/log"
)

func (c *getCmd) Run(_ []string) error {
	log.SetActivityLog(true)
	return errors.New("get unsupported on this platform: CIPD not supported on this platform")
}
