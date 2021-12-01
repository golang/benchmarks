// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

type GenConfig struct {
	AssetsDir       string
	SourceAssetsDir string
	OutputDir       string
	GoTool          *Go
}

type Generator interface {
	// Generate generates a new assets directory
	// from an old assets directory, given a configuration.
	Generate(*GenConfig) error
}
