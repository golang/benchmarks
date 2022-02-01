// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fileutil

import (
	"fmt"
	"io"
	"io/fs"
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
// If srcFS != nil, then src is assumed to be a path within
// srcFS.
//
// Returns a non-nil error if copying or acquiring the
// os.FileInfo for the file fails.
func CopyFile(dst, src string, sfinfo fs.FileInfo, srcFS fs.FS) error {
	var sf fs.File
	var err error
	if srcFS != nil {
		sf, err = srcFS.Open(src)
	} else {
		sf, err = os.Open(src)
	}
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

// CopyDir recursively copies the directory at path src to
// a new directory at path dst. If a symlink is encountered
// along the way, its link is copied verbatim and installed
// in the destination directory heirarchy, as in CopySymlink.
//
// If srcFS != nil, then src is assumed to be a path within
// srcFS.
//
// dst and directories under dst may not retain the permissions
// of src or the corresponding directories under src. Instead,
// we always set the permissions of the new directories to 0755.
func CopyDir(dst, src string, srcFS fs.FS) error {
	// Ignore the permissions of src, since if dst
	// isn't writable we can't actually copy files into it.
	// Pick a safe default that allows us to modify the
	// directory and files within however we want, but let
	// others only inspect it.
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	var des []fs.DirEntry
	var err error
	if srcFS != nil {
		des, err = fs.ReadDir(srcFS, src)
	} else {
		des, err = os.ReadDir(src)
	}
	if err != nil {
		return err
	}
	for _, de := range des {
		fi, err := de.Info()
		if err != nil {
			return err
		}
		d, s := filepath.Join(dst, fi.Name()), filepath.Join(src, fi.Name())
		if fi.IsDir() {
			if err := CopyDir(d, s, srcFS); err != nil {
				return err
			}
		} else if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links not supported")
		} else {
			if err := CopyFile(d, s, fi, srcFS); err != nil {
				return err
			}
		}
	}
	return nil
}
