// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "os"

var ppid int

func main() {
	for i := 0; i < 500000; i++ {
		ppid = os.Getppid()
	}
}
