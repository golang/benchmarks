// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package driver

import (
	"bytes"
	"log"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type sysStats struct {
	N      uint64
	Rusage unix.Rusage
}

func InitSysStats(N uint64) sysStats {
	ss := sysStats{N: N}
	if err := unix.Getrusage(0, &ss.Rusage); err != nil {
		log.Printf("Getrusage failed: %v", err)
		ss.N = 0
		// Deliberately ignore the error.
	}
	return ss
}

func (ss sysStats) Collect(res *Result) {
	if ss.N == 0 {
		return
	}
	if vm := getVMPeak(); vm != 0 {
		res.Metrics["peak-VM-bytes"] = vm
	}
	usage := new(unix.Rusage)
	if err := unix.Getrusage(0, usage); err != nil {
		log.Printf("Getrusage failed: %v", err)
		// Deliberately ignore the error.
		return
	}
	res.Metrics["peak-RSS-bytes"] = uint64(usage.Maxrss) * rssMultiplier
	res.Metrics["user+sys-ns/op"] = (cpuTime(usage) - cpuTime(&ss.Rusage)) / ss.N
}

func RunAndCollectSysStats(cmd *exec.Cmd, res *Result, N uint64, prefix string) (string, error) {
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	t0 := time.Now()
	if err := cmd.Run(); err != nil {
		return out.String(), err
	}
	t1 := time.Now()
	usage := fromStdUsage(cmd.ProcessState.SysUsage().(*syscall.Rusage))
	res.RunTime = uint64(t1.Sub(t0)) / N
	res.Metrics[prefix+"ns/op"] = res.RunTime
	res.Metrics[prefix+"user+sys-ns/op"] = cpuTime(usage) / N
	res.Metrics[prefix+"peak-RSS-bytes"] = uint64(usage.Maxrss) * rssMultiplier
	return out.String(), nil
}

func cpuTime(usage *unix.Rusage) uint64 {
	return uint64(usage.Utime.Sec)*1e9 + uint64(usage.Utime.Usec*1e3) +
		uint64(usage.Stime.Sec)*1e9 + uint64(usage.Stime.Usec)*1e3
}

func fromStdUsage(su *syscall.Rusage) *unix.Rusage {
	return &unix.Rusage{
		Utime:    unix.Timeval{Sec: su.Utime.Sec, Usec: su.Utime.Usec},
		Stime:    unix.Timeval{Sec: su.Stime.Sec, Usec: su.Stime.Usec},
		Maxrss:   su.Maxrss,
		Ixrss:    su.Ixrss,
		Idrss:    su.Idrss,
		Isrss:    su.Isrss,
		Minflt:   su.Minflt,
		Majflt:   su.Majflt,
		Nswap:    su.Nswap,
		Inblock:  su.Inblock,
		Oublock:  su.Oublock,
		Msgsnd:   su.Msgsnd,
		Msgrcv:   su.Msgrcv,
		Nsignals: su.Nsignals,
		Nvcsw:    su.Nvcsw,
		Nivcsw:   su.Nivcsw,
	}
}
