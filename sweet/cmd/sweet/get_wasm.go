// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"

	"golang.org/x/benchmarks/sweet/common/log"
)

func (c *getCmd) Run(_ []string) error {
	log.SetActivityLog(true)
	return errors.New("get unsupported on wasm: CIPD not supported on wasm")
}
