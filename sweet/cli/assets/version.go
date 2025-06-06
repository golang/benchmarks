// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package assets

import (
	"fmt"
	"regexp"
)

var versionRegexp = regexp.MustCompile(`v\d+\.\d+\.\d+`)

func ValidateVersion(version string) error {
	if !versionRegexp.MatchString(version) {
		return fmt.Errorf("version must be of the form 'v1.2.3'")
	}
	return nil
}

func ToCIPDVersion(version string) string {
	return version[1:]
}
