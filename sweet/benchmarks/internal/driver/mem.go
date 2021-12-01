// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !linux

package driver

import "os"

func ReadRSS(pid int) (uint64, error) {
	return 0, nil
}

func ReadPeakRSS(pid int) (uint64, error) {
	return 0, nil
}

func ReadPeakVM(pid int) (uint64, error) {
	return 0, nil
}

func ProcessPeakRSS(s *os.ProcessState) uint64 {
	return 0
}
