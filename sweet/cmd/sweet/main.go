// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"golang.org/x/benchmarks/sweet/cli/subcommands"
)

func main() {
	subcommands.Register(&getCmd{})
	subcommands.Register(&putCmd{})
	subcommands.Register(&runCmd{})
	subcommands.Register(&genCmd{})
	os.Exit(subcommands.Run())
}
