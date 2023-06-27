// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !plan9
// +build !plan9

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"

	lua "github.com/yuin/gopher-lua"
)

var short bool

func init() {
	flag.BoolVar(&short, "short", false, "whether to run a short version of this benchmark")
}

func parseFlags() error {
	flag.Parse()
	if flag.NArg() != 2 {
		return fmt.Errorf("expected lua program and input for it")
	}
	return nil
}

func parseInput(inputfile string) (string, error) {
	f, err := os.Open(inputfile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var builder strings.Builder
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		_, err := builder.Write(scanner.Bytes())
		if err != nil {
			return "", err
		}
	}
	return builder.String(), nil
}

func doBenchmark(s *lua.LState, input lua.LString) error {
	freq := lua.P{
		Fn:      s.GetGlobal("frequency"),
		NRet:    0,
		Protect: true,
	}
	count := lua.P{
		Fn:      s.GetGlobal("count"),
		NRet:    0,
		Protect: true,
	}
	if err := s.CallByParam(freq, input, lua.LNumber(1)); err != nil {
		return err
	}
	if short {
		return nil
	}
	if err := s.CallByParam(freq, input, lua.LNumber(2)); err != nil {
		return err
	}
	if err := s.CallByParam(count, input, lua.LString("GGT")); err != nil {
		return err
	}
	if err := s.CallByParam(count, input, lua.LString("GGTA")); err != nil {
		return err
	}
	if err := s.CallByParam(count, input, lua.LString("GGTATT")); err != nil {
		return err
	}
	return nil
}

func run(luafile, inputfile string) error {
	s := lua.NewState()
	defer s.Close()
	if err := s.DoFile(luafile); err != nil {
		return err
	}
	input, err := parseInput(inputfile)
	if err != nil {
		return err
	}
	return driver.RunBenchmark("GopherLuaKNucleotide", func(_ *driver.B) error {
		return doBenchmark(s, lua.LString(input))
	}, driver.InProcessMeasurementOptions...)
}

func main() {
	driver.SetFlags(flag.CommandLine)
	if err := parseFlags(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(flag.Arg(0), flag.Arg(1)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
