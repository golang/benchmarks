// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"crypto/mlkem"
	"testing"
)

// benchmarkMLKEM768 measures the two sides of an ephemeral key exchange.
func benchmarkMLKEM768(b *testing.B) {
	fixedKey, err := mlkem.NewDecapsulationKey768(fixedBytes(mlkem.SeedSize))
	if err != nil {
		b.Fatal(err)
	}
	fixedEncapsulationKey := fixedKey.EncapsulationKey().Bytes()
	_, fixedCiphertext := fixedKey.EncapsulationKey().Encapsulate()

	// KeygenDecaps is the initiator side: generating a key, encoding its
	// encapsulation key, and decapsulating the ciphertext of the peer.
	b.Run("KeygenDecaps", func(b *testing.B) {
		for b.Loop() {
			key, err := mlkem.GenerateKey768()
			if err != nil {
				b.Fatal(err)
			}
			key.EncapsulationKey().Bytes()
			// We don't want to run Encapsulate, so use a fixed key/ciphertext pair.
			if _, err := fixedKey.Decapsulate(fixedCiphertext); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Encaps is the responder side: decoding the encapsulation key of the peer
	// and encapsulating to it.
	b.Run("Encaps", func(b *testing.B) {
		for b.Loop() {
			key, err := mlkem.NewEncapsulationKey768(fixedEncapsulationKey)
			if err != nil {
				b.Fatal(err)
			}
			key.Encapsulate()
		}
	})
}
