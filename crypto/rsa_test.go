// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"testing"
)

func benchmarkRSA(b *testing.B) {
	digest := fixedBytes(crypto.SHA256.Size())

	// SignPSS is the private key operation of a TLS 1.3 handshake.
	b.Run("SignPSS", func(b *testing.B) {
		forRSAKeys(b, []int{2048}, func(b *testing.B, der []byte) {
			key := parseRSAKey(b, der)
			opts := &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}
			for b.Loop() {
				if _, err := rsa.SignPSS(rand.Reader, key, crypto.SHA256, digest, opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	// VerifyPKCS1v15 is the public key operation of certificate verification.
	b.Run("VerifyPKCS1v15", func(b *testing.B) {
		forRSAKeys(b, []int{2048, 4096}, func(b *testing.B, der []byte) {
			key := parseRSAKey(b, der)
			sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest)
			if err != nil {
				b.Fatal(err)
			}
			for b.Loop() {
				if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest, sig); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	// ParsePKCS8PrivateKey includes the validation and precomputation that
	// happen when loading a private key.
	b.Run("ParsePKCS8PrivateKey", func(b *testing.B) {
		forRSAKeys(b, []int{2048}, func(b *testing.B, der []byte) {
			for b.Loop() {
				if _, err := x509.ParsePKCS8PrivateKey(der); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

// forRSAKeys runs f as a sub-benchmark named "bits=N" for each of the key
// sizes, passing it the PKCS #8 encoding of a fixed key of that size.
func forRSAKeys(b *testing.B, sizes []int, f func(b *testing.B, der []byte)) {
	for _, bits := range sizes {
		b.Run(fmt.Sprintf("bits=%d", bits), func(b *testing.B) {
			data, err := os.ReadFile(fmt.Sprintf("testdata/rsa%d.pem", bits))
			if err != nil {
				b.Fatal(err)
			}
			data = bytes.ReplaceAll(data, []byte("TESTING KEY"), []byte("PRIVATE KEY"))
			block, _ := pem.Decode(data)
			if block == nil {
				b.Fatal("invalid PEM")
			}
			f(b, block.Bytes)
		})
	}
}

func parseRSAKey(b *testing.B, der []byte) *rsa.PrivateKey {
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		b.Fatal(err)
	}
	return key.(*rsa.PrivateKey)
}
