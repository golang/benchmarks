// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package driver

import (
	"log"
	"os/exec"
	"syscall"
	"unsafe"
)

// access to Windows APIs
var (
	modkernel32                = syscall.NewLazyDLL("kernel32.dll")
	modpsapi                   = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemoryInfo   = modpsapi.NewProc("GetProcessMemoryInfo")
	procSetProcessAffinityMask = modkernel32.NewProc("SetProcessAffinityMask")
)

type PROCESS_MEMORY_COUNTERS struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func getProcessMemoryInfo(h syscall.Handle, mem *PROCESS_MEMORY_COUNTERS) (err error) {
	r1, _, e1 := syscall.Syscall(procGetProcessMemoryInfo.Addr(), 3, uintptr(h), uintptr(unsafe.Pointer(mem)), uintptr(unsafe.Sizeof(*mem)))
	if r1 == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func setProcessAffinityMask(h syscall.Handle, mask uintptr) (err error) {
	r1, _, e1 := syscall.Syscall(procSetProcessAffinityMask.Addr(), 2, uintptr(h), mask, 0)
	if r1 == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func RunUnderProfiler(args ...string) (string, string) {
	return "", ""
}

// Size runs size command on the file. Returns filename with output. Any errors are ignored.
func Size(file string) string {
	return ""
}

type sysStats struct {
	N      uint64
	Cmd    *exec.Cmd
	Handle syscall.Handle
	Mem    PROCESS_MEMORY_COUNTERS
	CPU    syscall.Rusage
}

func InitSysStats(N uint64, cmd *exec.Cmd) (ss sysStats) {
	if cmd == nil {
		h, err := syscall.GetCurrentProcess()
		if err != nil {
			log.Printf("GetCurrentProcess failed: %v", err)
			return
		}
		if err := syscall.GetProcessTimes(h, &ss.CPU.CreationTime, &ss.CPU.ExitTime, &ss.CPU.KernelTime, &ss.CPU.UserTime); err != nil {
			log.Printf("GetProcessTimes failed: %v", err)
			return
		}
		ss.Handle = h
	} else {
		h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(cmd.Process.Pid))
		if err != nil {
			log.Printf("OpenProcess failed: %v", err)
			return
		}
		ss.Handle = h
	}
	ss.N = N
	ss.Cmd = cmd
	return ss
}

func (ss sysStats) Collect(res *Result, prefix string) {
	if ss.N == 0 {
		return
	}
	if ss.Cmd != nil {
		defer syscall.CloseHandle(ss.Handle)
		// TODO(dvyukov): GetProcessMemoryInfo/GetProcessTimes return info only for the process,
		// but not for child processes, so build benchmark numbers are incorrect.
		// It's better to report nothing than to report wrong numbers.
		return
	}
	var Mem PROCESS_MEMORY_COUNTERS
	if err := getProcessMemoryInfo(ss.Handle, &Mem); err != nil {
		log.Printf("GetProcessMemoryInfo failed: %v", err)
		return
	}
	var CPU syscall.Rusage
	if err := syscall.GetProcessTimes(ss.Handle, &CPU.CreationTime, &CPU.ExitTime, &CPU.KernelTime, &CPU.UserTime); err != nil {
		log.Printf("GetProcessTimes failed: %v", err)
		return
	}
	res.Metrics[prefix+"cputime"] = (getCPUTime(CPU) - getCPUTime(ss.CPU)) / ss.N
	res.Metrics[prefix+"rss"] = uint64(Mem.PeakWorkingSetSize)
}

func getCPUTime(CPU syscall.Rusage) uint64 {
	return uint64(CPU.KernelTime.Nanoseconds() + CPU.UserTime.Nanoseconds())
}

func setProcessAffinity(v int) {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		log.Printf("GetCurrentProcess failed: %v", err)
		return
	}
	if err := setProcessAffinityMask(h, uintptr(v)); err != nil {
		log.Printf("SetProcessAffinityMask failed: %v", err)
		return
	}
}
