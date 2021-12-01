// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fileutil

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// FileExists returns true if a file or directory exists at the
// specified path, otherwise it returns false. If an error is
// encountered while checking, an error is returned.
func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// CopyFile copies a file at path src to dst. sfinfo
// is the os.FileInfo associated with the file at path src
// and must be derived from it. sfinfo may be nil, in which
// case the file at src is queried for its os.FileInfo,
// and symbolic links are followed.
//
// In effect, sfinfo is just an optimization to avoid
// querying the path for the os.FileInfo more than necessary.
//
// Thus, CopyFile always copies the bytes of the file at
// src to a new file created at dst with the same file mode
// as the old one.
//
// Returns a non-nil error if copying or acquiring the
// os.FileInfo for the file fails.
func CopyFile(dst, src string, sfinfo os.FileInfo) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	if sfinfo == nil || sfinfo.Mode()&os.ModeSymlink != 0 {
		sfinfo, err = sf.Stat()
		if err != nil {
			return err
		}
	}
	df, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, sfinfo.Mode())
	if err != nil {
		return err
	}
	defer df.Close()
	_, err = io.Copy(df, sf)
	return err
}

// CopySymlink takes the symlink at path src and installs a new
// symlink at path dst which contains the same link path. As a result,
// relative symlinks point to a new location, relative to dst.
//
// sfinfo should be the result of an Lstat on src, and should always
// indicate a symlink. If not, or if sfinfo is nil, then the os.FileInfo
// for the symlink at src is regenerated.
//
// In effect, sfinfo is just an optimization to avoid
// querying the path for the os.FileInfo more than necessary.
//
// Returns a non-nil error if the path src doesn't point to a symlink
// or if an error is encountered in reading the link or installing
// a new link.
func CopySymlink(dst, src string, sfinfo os.FileInfo) error {
	if sfinfo == nil || sfinfo.Mode()&os.ModeSymlink == 0 {
		var err error
		sfinfo, err = os.Lstat(src)
		if err != nil {
			return err
		}
	}
	if sfinfo.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("source file is not a symlink")
	}
	// Handle a symlink by copying the
	// link verbatim.
	link, err := os.Readlink(src)
	if err != nil {
		return err
	}
	return os.Symlink(link, dst)
}

// CopyDir recursively copies the directory at path src to
// a new directory at path dst. If a symlink is encountered
// along the way, its link is copied verbatim and installed
// in the destination directory heirarchy, as in CopySymlink.
//
// dst and directories under dst may not retain the permissions
// of src or the corresponding directories under src. Instead,
// we always set the permissions of the new directories to 0755.
func CopyDir(dst, src string) error {
	// Ignore the permissions of src, since if dst
	// isn't writable we can't actually copy files into it.
	// Pick a safe default that allows us to modify the
	// directory and files within however we want, but let
	// others only inspect it.
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	fs, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}
	for _, fi := range fs {
		d, s := filepath.Join(dst, fi.Name()), filepath.Join(src, fi.Name())
		if fi.IsDir() {
			if err := CopyDir(d, s); err != nil {
				return err
			}
		} else if fi.Mode()&os.ModeSymlink != 0 {
			if err := CopySymlink(d, s, fi); err != nil {
				return err
			}
		} else {
			if err := CopyFile(d, s, fi); err != nil {
				return err
			}
		}
	}
	return nil
}
