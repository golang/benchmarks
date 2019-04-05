// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//+build dragonfly freebsd netbsd openbsd solaris

package driver

const rssMultiplier = 1

func RunUnderProfiler(args ...string) (string, string) {
	return "", ""
}

// Size runs size command on the file. Returns filename with output. Any errors are ignored.
func Size(file string) string {
	return ""
}

func getVMPeak() uint64 {
	return 0
}

func setProcessAffinity(v int) {
}
