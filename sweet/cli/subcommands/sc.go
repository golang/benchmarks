// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package subcommands

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"golang.org/x/benchmarks/sweet/common"
)

const (
	usageHeader = `Sweet %s: Go Benchmarking Suite

`
	usageTop = `Sweet is a set of benchmarks derived from the Go community which are intended
to represent a breadth of real-world applications. The primary use-case of this
suite is to perform an evaluation of the difference in CPU and memory
performance between two Go implementations.

If you use this benchmarking suite for any measurements, please ensure you use
a versioned release and note the version in the release.

All results are reported in the standard Go testing package format, such that
results may be compared using the benchstat tool.

Usage: %s <subcommand> [subcommand flags] [subcommand args]

Subcommands:
`
)

var (
	base string
	cmds []*command
	out  io.Writer
)

func init() {
	base = filepath.Base(os.Args[0])
	out = os.Stderr
}

type command struct {
	Command
	flags *flag.FlagSet
}

func (c *command) usage() {
	fmt.Fprintf(out, usageHeader, common.Version)
	c.PrintUsage(out, base)
	c.flags.PrintDefaults()
}

type Command interface {
	Name() string
	Synopsis() string
	PrintUsage(w io.Writer, base string)
	SetFlags(f *flag.FlagSet)
	Run(args []string) error
}

func Register(cmd Command) {
	f := flag.NewFlagSet(cmd.Name(), flag.ExitOnError)
	cmd.SetFlags(f)
	c := &command{
		Command: cmd,
		flags:   f,
	}
	f.Usage = func() {
		c.usage()
	}
	cmds = append(cmds, c)
}

func usage() {
	fmt.Fprintf(out, usageHeader, common.Version)
	fmt.Fprintf(out, usageTop, base)
	maxnamelen := 10
	for _, c := range cmds {
		l := utf8.RuneCountInString(c.Name())
		if l > maxnamelen {
			maxnamelen = l
		}
	}
	for _, c := range cmds {
		fmt.Fprintf(out, fmt.Sprintf("  %%%ds: %%s\n", maxnamelen), c.Name(), c.Synopsis())
	}
}

func Run() int {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	subcmd := os.Args[1]
	if subcmd == "help" {
		if len(os.Args) >= 3 {
			subhelp := os.Args[2]
			for _, cmd := range cmds {
				if cmd.Name() == subhelp {
					cmd.usage()
					return 0
				}
			}
		}
		usage()
		return 0
	}
	var chosen *command
	for _, cmd := range cmds {
		if cmd.Name() == subcmd {
			chosen = cmd
			break
		}
	}
	if chosen == nil {
		fmt.Fprintf(out, "unknown subcommand: %q\n", subcmd)
		fmt.Fprintln(out)
		usage()
		return 1
	}
	chosen.flags.Parse(os.Args[2:])
	if err := chosen.Run(chosen.flags.Args()); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	return 0
}
