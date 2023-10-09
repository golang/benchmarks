// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasm

package driver

import (
	"fmt"
	"os/exec"
	"runtime"
)

type sysStats struct{}

func InitSysStats(N uint64) sysStats { return sysStats{} }

func (sysStats) Collect(*Result) {}

func RunAndCollectSysStats(cmd *exec.Cmd, res *Result, N uint64, prefix string) (string, error) {
	return "", fmt.Errorf("not implemented on %s/%s", runtime.GOOS, runtime.GOARCH)
}
