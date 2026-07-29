// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fileutil

import (
	"bytes"
	"errors"
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
// Thus, CopyFile copies the bytes of the file at src to a file
// created at dst with the same file mode as the old one.
// If dst already exists and has the same contents as src, CopyFile
// returns success without copying the file.
//
// If srcFS != nil, then src is assumed to be a path within
// srcFS.
//
// Returns a non-nil error if copying or acquiring the
// os.FileInfo for the file fails.
func CopyFile(dst, src string, sfinfo fs.FileInfo, srcFS fs.FS) error {
	openSrc := func() (fs.File, error) {
		if srcFS != nil {
			return srcFS.Open(src)
		}
		return os.Open(src)
	}
	sf, err := openSrc()
	if err != nil {
		return err
	}
	defer func() {
		if sf != nil {
			sf.Close()
		}
	}()

	if sfinfo == nil || sfinfo.Mode()&os.ModeSymlink != 0 {
		sfinfo, err = sf.Stat()
		if err != nil {
			return err
		}
	}

	if df, err := os.Open(dst); err == nil {
		dfinfo, err := df.Stat()
		if err == nil && dfinfo.Mode().IsRegular() && dfinfo.Size() == sfinfo.Size() {
			same, err := sameContent(sf, df)
			df.Close()
			if err == nil && same {
				return nil
			}
		} else {
			df.Close()
		}
		if seeker, ok := sf.(io.Seeker); ok {
			_, err = seeker.Seek(0, io.SeekStart)
		} else {
			err = errors.New("not seekable")
		}
		if err != nil {
			sf.Close()
			sf, err = openSrc()
			if err != nil {
				sf = nil
				return err
			}
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

func sameContent(r1, r2 io.Reader) (bool, error) {
	buf1 := make([]byte, 64*1024)
	buf2 := make([]byte, 64*1024)
	for {
		n1, err1 := io.ReadFull(r1, buf1)
		n2, err2 := io.ReadFull(r2, buf2)
		if err1 != nil && err1 != io.EOF && err1 != io.ErrUnexpectedEOF {
			return false, err1
		}
		if err2 != nil && err2 != io.EOF && err2 != io.ErrUnexpectedEOF {
			return false, err2
		}
		if n1 != n2 {
			return false, nil
		}
		if !bytes.Equal(buf1[:n1], buf2[:n2]) {
			return false, nil
		}
		if err1 == io.EOF || err1 == io.ErrUnexpectedEOF {
			if err2 == io.EOF || err2 == io.ErrUnexpectedEOF {
				return true, nil
			}
			return false, nil
		}
	}
}

// CopyDir recursively copies the directory at path src to
// a new directory at path dst. If a symlink is encountered
// along the way, it is deep-copied.
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
		} else {
			if err := CopyFile(d, s, fi, srcFS); err != nil {
				return err
			}
		}
	}
	return nil
}
