// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux

package driver

import (
	"log"
	"os/exec"
	"syscall"
)

type sysStats struct {
	N      uint64
	Cmd    *exec.Cmd
	Rusage syscall.Rusage
}

func InitSysStats(N uint64, cmd *exec.Cmd) sysStats {
	ss := sysStats{N: N, Cmd: cmd}
	if cmd == nil {
		err := syscall.Getrusage(0, &ss.Rusage)
		if err != nil {
			log.Printf("Getrusage failed: %v", err)
			ss.N = 0
			// Deliberately ignore the error.
		}
	}
	return ss
}

func (ss sysStats) Collect(res *Result, prefix string) {
	if ss.N == 0 {
		return
	}
	Rusage := new(syscall.Rusage)
	if ss.Cmd == nil {
		err := syscall.Getrusage(0, Rusage)
		if err != nil {
			log.Printf("Getrusage failed: %v", err)
			// Deliberately ignore the error.
			return
		}
	} else {
		Rusage = ss.Cmd.ProcessState.SysUsage().(*syscall.Rusage)
	}
	cpuTime := func(usage *syscall.Rusage) uint64 {
		return uint64(usage.Utime.Sec)*1e9 + uint64(usage.Utime.Usec*1e3) +
			uint64(usage.Stime.Sec)*1e9 + uint64(usage.Stime.Usec)*1e3
	}
	res.Metrics[prefix+"rss"] = uint64(Rusage.Maxrss) * rssMultiplier
	res.Metrics[prefix+"cputime"] = (cpuTime(Rusage) - cpuTime(&ss.Rusage)) / ss.N

	if ss.Cmd == nil {
		vm := getVMPeak()
		if vm != 0 {
			res.Metrics[prefix+"vm"] = vm
		}
	}
}
