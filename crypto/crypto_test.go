// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
)

// BenchmarkCrypto measures the most widely used cryptographic operations
// through the public API of the standard library (and of golang.org/x/crypto,
// for ChaCha20-Poly1305).
//
// Names are of the form Crypto/<algorithm>[/<operation>][/<key>=<value>].
//
// Each benchmark reports a single metric, either time or throughput.
func BenchmarkCrypto(b *testing.B) {
	b.Run("SHA256", benchmarkSHA256)
	b.Run("SHA512", benchmarkSHA512)
	b.Run("SHA1", benchmarkSHA1)
	b.Run("MD5", benchmarkMD5)
	b.Run("SHA3-256", benchmarkSHA3)
	b.Run("HMAC-SHA256", benchmarkHMAC)

	b.Run("AES-128-GCM", benchmarkAESGCM)
	b.Run("ChaCha20Poly1305", benchmarkChaCha20Poly1305)
	b.Run("AES-128-CTR", benchmarkAESCTR)
	b.Run("AES-128-CBC", benchmarkAESCBC)

	b.Run("RSA", benchmarkRSA)
	b.Run("ECDSA", benchmarkECDSA)
	b.Run("ECDH", benchmarkECDH)
	b.Run("Ed25519", benchmarkEd25519)
	b.Run("MLKEM768", benchmarkMLKEM768)
	b.Run("MLDSA44", benchmarkMLDSA44)

	b.Run("Rand", benchmarkRand)
}

// bulkSize is the input size from which an operation is considered a bulk
// operation, dominated by the per-byte cost rather than by the per-call cost.
const bulkSize = 1024

// forSizes runs f as a sub-benchmark named "size=N" for each of the sizes.
//
// Bulk operations report only throughput, and other operations report only
// time. The two metrics carry the same information, and reporting only the
// most natural one keeps each benchmark to a single dashboard series.
func forSizes(b *testing.B, sizes []int, f func(b *testing.B, size int)) {
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			f(b, size)
			if size >= bulkSize {
				b.SetBytes(int64(size))
				b.ReportMetric(0, "ns/op") // suppress the metric
			}
		})
	}
}

// fixedBytes returns n bytes to use as a key, seed, or nonce. Fixed inputs make
// results comparable across runs.
func fixedBytes(n int) []byte {
	return bytes.Repeat([]byte{0x42}, n)
}

func benchmarkRand(b *testing.B) {
	b.Run("Read", func(b *testing.B) {
		forSizes(b, []int{32}, func(b *testing.B, size int) {
			buf := make([]byte, size)
			for b.Loop() {
				rand.Read(buf)
			}
		})
	})
}
