// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generators

import "golang.org/x/benchmarks/sweet/common"

// None is a Generator that does nothing.
type None struct{}

// Generate does nothing.
func (_ None) Generate(_ *common.GenConfig) error {
	return nil
}
