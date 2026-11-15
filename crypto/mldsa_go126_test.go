// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !go1.27

package crypto_test

import "testing"

func benchmarkMLDSA44(b *testing.B) {
	b.Skip("crypto/mldsa requires Go 1.27")
}
