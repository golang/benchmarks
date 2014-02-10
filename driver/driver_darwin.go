// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !linux

package driver

import (
	"bytes"
	"log"
	"os"
	"os/exec"
)

const rssMultiplier = 1

func RunUnderProfiler(args ...string) (string, string) {
	return "", ""
}

// Size runs size command on the file. Returns filename with output. Any errors are ignored.
func Size(file string) string {
	resf, err := os.Create(tempFilename("size.txt"))
	if err != nil {
		log.Printf("Failed to create output file: %v", err)
		return ""
	}
	defer resf.Close()

	var stderr bytes.Buffer
	cmd := exec.Command("size", "-m", file)
	cmd.Stdout = resf
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to execute 'size -m %v': %v\n%v", file, err, stderr.String())
		return ""
	}

	return resf.Name()
}

func getVMPeak() uint64 {
	return 0
}

func setProcessAffinity(v int) {
}
