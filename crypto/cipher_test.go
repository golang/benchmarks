// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"crypto/aes"
	"crypto/cipher"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func benchmarkAESGCM(b *testing.B) {
	block, err := aes.NewCipher(fixedBytes(16))
	if err != nil {
		b.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkAEAD(b, aead)
}

// benchmarkChaCha20Poly1305 measures the golang.org/x/crypto implementation
// selected by go.mod, which is not necessarily the version vendored into the
// standard library for use by crypto/tls.
//
// This will be replaced with crypto/aead anyway.
func benchmarkChaCha20Poly1305(b *testing.B) {
	aead, err := chacha20poly1305.New(fixedBytes(chacha20poly1305.KeySize))
	if err != nil {
		b.Fatal(err)
	}
	benchmarkAEAD(b, aead)
}

// benchmarkAEAD measures sealing and opening a network packet (1350 bytes)
// and sealing a larger buffer.
func benchmarkAEAD(b *testing.B, aead cipher.AEAD) {
	nonce := fixedBytes(aead.NonceSize())
	additionalData := fixedBytes(5) // like in TLS 1.3

	b.Run("Seal", func(b *testing.B) {
		forSizes(b, []int{1350, 8192}, func(b *testing.B, size int) {
			plaintext := make([]byte, size)
			out := make([]byte, 0, size+aead.Overhead())
			for b.Loop() {
				aead.Seal(out, nonce, plaintext, additionalData)
			}
		})
	})

	b.Run("Open", func(b *testing.B) {
		forSizes(b, []int{1350}, func(b *testing.B, size int) {
			ciphertext := aead.Seal(nil, nonce, make([]byte, size), additionalData)
			out := make([]byte, 0, size)
			for b.Loop() {
				if _, err := aead.Open(out, nonce, ciphertext, additionalData); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

func benchmarkAESCTR(b *testing.B) {
	block, err := aes.NewCipher(fixedBytes(16))
	if err != nil {
		b.Fatal(err)
	}

	// The small size is not a multiple of the block size, and measures the
	// cost of short writes to a long-lived stream.
	forSizes(b, []int{50, 8192}, func(b *testing.B, size int) {
		buf := make([]byte, size)
		stream := cipher.NewCTR(block, fixedBytes(aes.BlockSize))
		for b.Loop() {
			stream.XORKeyStream(buf, buf)
		}
	})
}

func benchmarkAESCBC(b *testing.B) {
	block, err := aes.NewCipher(fixedBytes(16))
	if err != nil {
		b.Fatal(err)
	}

	// Encryption and decryption are measured separately because the former is
	// inherently serial, while the latter can process blocks in parallel.
	b.Run("Encrypt", func(b *testing.B) {
		forSizes(b, []int{1024}, func(b *testing.B, size int) {
			buf := make([]byte, size)
			mode := cipher.NewCBCEncrypter(block, fixedBytes(aes.BlockSize))
			for b.Loop() {
				mode.CryptBlocks(buf, buf)
			}
		})
	})

	b.Run("Decrypt", func(b *testing.B) {
		forSizes(b, []int{1024}, func(b *testing.B, size int) {
			buf := make([]byte, size)
			mode := cipher.NewCBCDecrypter(block, fixedBytes(aes.BlockSize))
			for b.Loop() {
				mode.CryptBlocks(buf, buf)
			}
		})
	})
}
