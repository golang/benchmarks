// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common/fileutil"
	"golang.org/x/benchmarks/sweet/common/log"
)

func gitShallowClone(dir, url, ref string) error {
	cmd := exec.Command("git", "clone", "--depth", "1", "-b", ref, url, dir)
	log.TraceCommand(cmd, false)
	return cmd.Run()
}

func gitCloneToCommit(dir, url, branch, hash string) error {
	cloneCmd := exec.Command("git", "clone", "-b", branch, url, dir)
	log.TraceCommand(cloneCmd, false)
	if err := cloneCmd.Run(); err != nil {
		return err
	}
	checkoutCmd := exec.Command("git", "-C", dir, "checkout", hash)
	log.TraceCommand(checkoutCmd, false)
	return checkoutCmd.Run()
}

func copyFile(dst, src string) error {
	log.CommandPrintf("cp %s %s", src, dst)
	return fileutil.CopyFile(dst, src, nil, nil)
}

func makeWriteable(dir string) error {
	log.CommandPrintf("chmod -R a+w %s", dir)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&0222 == 0222 {
			return nil
		}
		return os.Chmod(path, info.Mode()|0222)
	})
}

func symlink(dst, src string) error {
	log.CommandPrintf("ln -s %s %s", src, dst)
	return os.Symlink(src, dst)
}
