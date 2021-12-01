// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bootstrap

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

func VersionArchiveName(version string) string {
	return fmt.Sprintf("%s.tar.gz", VersionDirName(version))
}

func VersionDirName(version string) string {
	return fmt.Sprintf("assets-%s", version)
}
