// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"testing"
)

// The hash benchmarks call the one-shot functions directly, rather than
// through a shared helper, so that the compiler can optimize the calls like it
// would in an application.

func benchmarkSHA256(b *testing.B) {
	// The small size measures the fixed per-call cost, which dominates when
	// hashing short inputs like keys and identifiers.
	forSizes(b, []int{8, 8192}, func(b *testing.B, size int) {
		buf := make([]byte, size)
		for b.Loop() {
			sha256.Sum256(buf)
		}
	})
}

func benchmarkSHA512(b *testing.B) {
	forSizes(b, []int{8192}, func(b *testing.B, size int) {
		buf := make([]byte, size)
		for b.Loop() {
			sha512.Sum512(buf)
		}
	})
}

func benchmarkSHA1(b *testing.B) {
	forSizes(b, []int{8192}, func(b *testing.B, size int) {
		buf := make([]byte, size)
		for b.Loop() {
			sha1.Sum(buf)
		}
	})
}

func benchmarkMD5(b *testing.B) {
	forSizes(b, []int{8192}, func(b *testing.B, size int) {
		buf := make([]byte, size)
		for b.Loop() {
			md5.Sum(buf)
		}
	})
}

func benchmarkSHA3(b *testing.B) {
	forSizes(b, []int{1350}, func(b *testing.B, size int) {
		buf := make([]byte, size)
		for b.Loop() {
			sha3.Sum256(buf)
		}
	})
}

func benchmarkHMAC(b *testing.B) {
	key := fixedBytes(32)

	// Reuse computes MACs with a long-lived hash.Hash, like PBKDF2 does.
	b.Run("Reuse", func(b *testing.B) {
		forSizes(b, []int{32}, func(b *testing.B, size int) {
			buf := make([]byte, size)
			mac := make([]byte, 0, sha256.Size)
			h := hmac.New(sha256.New, key)
			for b.Loop() {
				h.Reset()
				h.Write(buf)
				h.Sum(mac)
			}
		})
	})

	// New computes each MAC with a fresh hash.Hash, like most applications
	// authenticating a single message do.
	b.Run("New", func(b *testing.B) {
		forSizes(b, []int{32}, func(b *testing.B, size int) {
			buf := make([]byte, size)
			mac := make([]byte, 0, sha256.Size)
			for b.Loop() {
				h := hmac.New(sha256.New, key)
				h.Write(buf)
				h.Sum(mac)
			}
		})
	})
}
