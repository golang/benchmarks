// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bootstrap

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
)

type Hashes map[string]string

func (h Hashes) Get(version string) (string, bool) {
	v, b := h[version]
	return v, b
}

func (h Hashes) Put(version string, hash string, force bool) bool {
	if _, ok := h[version]; ok && !force {
		return false
	}
	h[version] = hash
	return true
}

func ReadHashesFile(hashfile string) (Hashes, error) {
	f, err := os.Open(hashfile)
	if os.IsNotExist(err) {
		return make(Hashes), nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()
	var vals map[string]string
	err = json.NewDecoder(f).Decode(&vals)
	return Hashes(vals), err
}

func (h Hashes) WriteToFile(hashfile string) error {
	f, err := os.Create(hashfile)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(&h)
}

func canonicalizeHash(h hash.Hash) string {
	return fmt.Sprintf("%x", h.Sum(nil))
}

func HashStream(r io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, r); err != nil {
		return "", err
	}
	return canonicalizeHash(hash), nil
}
