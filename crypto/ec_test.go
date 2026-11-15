// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"crypto"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

func benchmarkECDSA(b *testing.B) {
	b.Run("Sign", func(b *testing.B) {
		b.Run("P256", func(b *testing.B) {
			benchmarkECDSASign(b, elliptic.P256(), crypto.SHA256)
		})
	})

	b.Run("Verify", func(b *testing.B) {
		b.Run("P256", func(b *testing.B) {
			benchmarkECDSAVerify(b, elliptic.P256(), crypto.SHA256)
		})
		b.Run("P384", func(b *testing.B) {
			benchmarkECDSAVerify(b, elliptic.P384(), crypto.SHA384)
		})
	})
}

func benchmarkECDSASign(b *testing.B, curve elliptic.Curve, hash crypto.Hash) {
	key := newECDSAKey(b, curve)
	digest := fixedBytes(hash.Size())
	for b.Loop() {
		if _, err := ecdsa.SignASN1(rand.Reader, key, digest); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkECDSAVerify(b *testing.B, curve elliptic.Curve, hash crypto.Hash) {
	key := newECDSAKey(b, curve)
	digest := fixedBytes(hash.Size())
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if !ecdsa.VerifyASN1(&key.PublicKey, digest, sig) {
			b.Fatal("invalid signature")
		}
	}
}

func newECDSAKey(b *testing.B, curve elliptic.Curve) *ecdsa.PrivateKey {
	size := (curve.Params().BitSize + 7) / 8
	key, err := ecdsa.ParseRawPrivateKey(curve, fixedBytes(size))
	if err != nil {
		b.Fatal(err)
	}
	return key
}

// benchmarkECDH measures one side of an ephemeral key exchange: generating a
// key, encoding its public key, and computing the shared secret with the
// public key of the peer.
func benchmarkECDH(b *testing.B) {
	b.Run("X25519", func(b *testing.B) {
		benchmarkECDHCurve(b, ecdh.X25519())
	})
	b.Run("P256", func(b *testing.B) {
		benchmarkECDHCurve(b, ecdh.P256())
	})
}

func benchmarkECDHCurve(b *testing.B, curve ecdh.Curve) {
	peerKey, err := curve.NewPrivateKey(fixedBytes(32))
	if err != nil {
		b.Fatal(err)
	}
	peerShare := peerKey.PublicKey().Bytes()
	for b.Loop() {
		key, err := curve.GenerateKey(rand.Reader)
		if err != nil {
			b.Fatal(err)
		}
		key.PublicKey().Bytes()
		peer, err := curve.NewPublicKey(peerShare)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := key.ECDH(peer); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkEd25519(b *testing.B) {
	key := ed25519.NewKeyFromSeed(fixedBytes(ed25519.SeedSize))
	message := fixedBytes(32)

	b.Run("Sign", func(b *testing.B) {
		for b.Loop() {
			ed25519.Sign(key, message)
		}
	})

	b.Run("Verify", func(b *testing.B) {
		publicKey := key.Public().(ed25519.PublicKey)
		sig := ed25519.Sign(key, message)
		for b.Loop() {
			if !ed25519.Verify(publicKey, message, sig) {
				b.Fatal("invalid signature")
			}
		}
	})
}
