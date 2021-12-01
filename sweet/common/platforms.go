// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import (
	"fmt"
	"runtime"
)

type Platform struct {
	GOOS, GOARCH string
}

func (p Platform) String() string {
	return fmt.Sprintf("%s/%s", p.GOOS, p.GOARCH)
}

func (p Platform) BuildEnv(e *Env) *Env {
	return e.MustSet(
		fmt.Sprintf("GOOS=%s", p.GOOS),
		fmt.Sprintf("GOARCH=%s", p.GOARCH),
	)
}

var SupportedPlatforms = []Platform{
	{"linux", "amd64"},
}

func CurrentPlatform() Platform {
	return Platform{
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}
}
