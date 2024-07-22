// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestReadFileTail(t *testing.T) {
	tmpDir := t.TempDir()
	check := func(data string, want string) {
		t.Helper()
		f, err := os.CreateTemp(tmpDir, "")
		if err != nil {
			t.Fatalf("creating temp input file: %s", err)
		}
		defer f.Close()
		_, err = f.WriteString(data)
		if err != nil {
			t.Fatalf("writing temp input file: %s", err)
		}

		got, err := readFileTail(f)

		if got != want {
			t.Errorf("got:\n%q\nwant:\n%q", got, want)
		}
	}

	numbers := func(n, m int) string {
		var buf strings.Builder
		for i := n; i < m; i++ {
			fmt.Fprintf(&buf, "%d\n", i)
		}
		return buf.String()
	}

	// Basic test
	check(numbers(0, 40), numbers(20, 40))
	// Multiple blocks
	check(numbers(0, 5000), numbers(5000-20, 5000))
	// Byte limit
	check(
		strings.Repeat("a", 32<<10)+"\nb\n",
		strings.Repeat("a", 16<<10-3)+"\nb\n")
}
