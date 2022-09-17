// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package driver

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"syscall"
)

var (
	reVmPeak = regexp.MustCompile(`VmPeak:\s*(\d+) kB`)
	reVmRSS  = regexp.MustCompile(`VmRSS:\s*(\d+) kB`)
	reVmHWM  = regexp.MustCompile(`VmHWM:\s*(\d+) kB`)
)

func readStat(pid int, r *regexp.Regexp) (uint64, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	m := r.FindSubmatch(b)
	if len(m) < 2 {
		return 0, nil
	}
	val, err := strconv.ParseUint(string(m[1]), 10, 64)
	return val * 1024, err
}

func ReadRSS(pid int) (uint64, error) {
	return readStat(pid, reVmRSS)
}

func ReadPeakRSS(pid int) (uint64, error) {
	return readStat(pid, reVmHWM)
}

func ReadPeakVM(pid int) (uint64, error) {
	return readStat(pid, reVmPeak)
}

func ProcessPeakRSS(s *os.ProcessState) uint64 {
	return uint64(s.SysUsage().(*syscall.Rusage).Maxrss) * 1024
}
