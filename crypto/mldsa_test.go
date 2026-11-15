// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.27

package crypto_test

import (
	"bytes"
	"crypto/mldsa"
	"math/rand/v2"
	"os"
	"testing"
)

func benchmarkMLDSA44(b *testing.B) {
	seed := make([]byte, mldsa.PrivateKeySize)
	key, err := mldsa.NewPrivateKey(mldsa.MLDSA44(), seed)
	if err != nil {
		b.Fatal(err)
	}

	// Signing works by rejection sampling, so the time it takes varies widely
	// from one message to the other. To get stable but representative results,
	// Sign cycles through a set of messages selected to have the same
	// distribution of rejection counts and reasons as the average case.
	b.Run("Sign", func(b *testing.B) {
		data, err := os.ReadFile("testdata/mldsa44-messages.txt")
		if err != nil {
			b.Fatal(err)
		}
		messages := bytes.Fields(data)
		// Shuffle the messages so that, across runs, the results are not
		// biased by which messages a partial cycle includes.
		rand.Shuffle(len(messages), func(i, j int) {
			messages[i], messages[j] = messages[j], messages[i]
		})
		i := 0
		for b.Loop() {
			if _, err := key.SignDeterministic(messages[i], nil); err != nil {
				b.Fatal(err)
			}
			i = (i + 1) % len(messages)
		}
	})

	// Verify includes decoding the public key, which performs part of the
	// work of verification, and which usually happens once per signature.
	b.Run("Verify", func(b *testing.B) {
		publicKey := key.PublicKey().Bytes()
		message := fixedBytes(32)
		sig, err := key.SignDeterministic(message, nil)
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			pk, err := mldsa.NewPublicKey(mldsa.MLDSA44(), publicKey)
			if err != nil {
				b.Fatal(err)
			}
			if err := mldsa.Verify(pk, message, sig, nil); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Keygen derives a private key from its seed, which is also how private
	// keys are loaded from storage.
	b.Run("Keygen", func(b *testing.B) {
		for b.Loop() {
			if _, err := mldsa.NewPrivateKey(mldsa.MLDSA44(), seed); err != nil {
				b.Fatal(err)
			}
		}
	})
}
