// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package assets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrNotInCache = errors.New("not found in cache")

func CachedAssets(cache, version string) (string, error) {
	name := version
	if err := os.MkdirAll(cache, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}
	cacheloc := filepath.Join(cache, name)
	if _, err := os.Lstat(cacheloc); os.IsNotExist(err) {
		return cacheloc, ErrNotInCache
	} else if err != nil {
		return "", fmt.Errorf("failed to check cache: %w", err)
	}
	return cacheloc, nil
}

func CacheDefault() string {
	cache, err := os.UserCacheDir()
	if err == nil {
		cache = filepath.Join(cache, "go-sweet")
	}
	return cache
}

const CIPDCacheDir = ".cipd-cache"
