// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package driver

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func RunUnderProfiler(args ...string) (string, string) {
	return "", ""
}

// Size runs size command on the file. Returns filename with output. Any errors are ignored.
func Size(file string) string {
	return ""
}

type sysStats struct {
	N       uint64
	CPUTime uint64
}

func InitSysStats(N uint64) sysStats {
	ss := sysStats{N: N}
	var err error
	ss.CPUTime, err = procCPUTime()
	if err != nil {
		log.Printf("failed to parse /dev/cputime: %v", err)
		ss.N = 0
		// Deliberately ignore the error.
	}
	return ss
}

func (ss sysStats) Collect(res *Result) {
	if ss.N == 0 {
		return
	}
	t, err := procCPUTime()
	if err != nil {
		log.Printf("failed to parse /dev/cputime: %v", err)
		// Deliberately ignore the error.
		return
	}
	res.Metrics["user+sys-ns/op"] = (t - ss.CPUTime) / ss.N
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
	w := cmd.ProcessState.SysUsage().(*syscall.Waitmsg)
	res.RunTime = uint64(t1.Sub(t0)) / N
	res.Metrics[prefix+"ns/op"] = res.RunTime
	res.Metrics[prefix+"user+sys-ns/op"] = cpuTime(w) / N
	return out.String(), nil
}

func procCPUTime() (uint64, error) {
	b, err := os.ReadFile("/dev/cputime")
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if n := len(f); n != 6 {
		return 0, fmt.Errorf("/dev/cputime has %v fields", n)
	}

	// Sum up time spent in user mode and system calls,
	// for both the current process and the descendants.
	var tt uint64
	for _, i := range []int{0, 1, 3, 4} {
		t, err := strconv.ParseUint(f[i], 10, 32)
		if err != nil {
			return 0, err
		}
		tt += t
	}
	return tt * 1e6, nil
}

func cpuTime(w *syscall.Waitmsg) uint64 {
	return uint64(w.Time[0])*1e6 + uint64(w.Time[1])*1e6
}

func setProcessAffinity(v int) {
}
