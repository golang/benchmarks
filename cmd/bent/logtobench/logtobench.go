// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// logtobench is a tool used to construct a "benchmarks.toml"
// file that runs each benchmark from a binary in isolation.
// The purpose of this is to allow benchmark-by-benchmark profiling,
// so that the set of profile results can be analyzed to determine
// which benchmarks are redundant, which portions of the runtime
// and standard library are covered, etc., with the ultimate goal
// of allowing both a smaller set of benchmarks, and the possibility
// of matching the profile from some other program (or set of programs)
// against a weighted subset of these benchmarks.
//
// The input to this program is just the output of "bent -v" applied
// to a single configuration (for example, what might result from
// `bent -v -C configurations-pgo.toml -c=pgo-generate`).
//
// The output (on standard out) can be used as a benchmarks-xxx.toml
// file.

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	"golang.org/x/benchmarks/sweet/common"
)

func process(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Support long lines if necessary
	const maxCapacity = 1024 * 1024
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	var suite string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "shortname:") {
			suite = strings.TrimSpace(line[len("shortname:"):])
			continue
		}
		if suite != "" && strings.HasPrefix(line, "Benchmark") {
			details := line[len("Benchmark"):]
			ws := strings.IndexAny(details, " \t")
			if ws != -1 { // look for trailing benchmarks, directly from a log file
				trailing := details[ws:]
				details = details[:ws] // lose the white space
				if strings.ContainsAny(trailing, "0123456789") {
					dash := strings.LastIndex(details, "-")
					if dash != -1 {
						details = details[:dash] // lose the processor count
					}
				}
			}

			escapedBenchmark := "Benchmark" + strings.ReplaceAll(regexp.QuoteMeta(details), "\\", "\\\\")
			escapedBenchmark = strings.ReplaceAll(escapedBenchmark, "/", "$/^")
			fmt.Printf("[[Benchmarks]]\n  Name = \"%s-%s\"\n  Suite = \"%s\"\n  Benchmarks = \"^%s$\"\n\n",
				suite, common.Escapify(details), suite, escapedBenchmark)
			if deDetails, err := url.PathUnescape(common.Escapify(details)); err != nil || details != deDetails {
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error attempting to round-trip encoded string for %s\n", details)
				} else {
					fmt.Fprintf(os.Stderr, "Error decoding encoded string for %s, DE = %s\n", details, deDetails)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
	}
}

func main() {
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			if arg == "-" {
				process(os.Stdin)
			} else {
				f, err := os.Open(arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error opening %s: %v\n", arg, err)
					os.Exit(1)
				}
				process(f)
				f.Close()
			}
		}
	} else {
		process(os.Stdin)
	}
}
