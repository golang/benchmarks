// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import "strings"

const escapeThese = "<>/\\|:=+*:,?\"'`(!#$&)" // forbidden in windows file names or problematic for casual pasting into shell commands
const upperhex = "0123456789ABCDEF"

func Escapify(s string) string {
	var b []byte
	for _, c := range []byte(s) {
		if strings.IndexByte(escapeThese, c) != -1 {
			b = append(b, '%')
			b = append(b, upperhex[c>>4])
			b = append(b, upperhex[c&15])
		} else {
			b = append(b, c)
		}
	}
	return string(b)
}

func HasEscapes(s string) bool {
	return strings.ContainsAny(s, escapeThese)
}
