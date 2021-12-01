// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/cli/bootstrap"
	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

const (
	putUsage = `Uploads a new version of the benchmark assets to GCS.

Usage: %s put [flags]
`
)

type putCmd struct {
	auth           bootstrap.AuthOption
	force          bool
	public         bool
	cache          string
	bucket         string
	assetsDir      string
	assetsHashFile string
	version        string
}

func (*putCmd) Name() string { return "put" }
func (*putCmd) Synopsis() string {
	return "Uploads a new version of the benchmark assets."
}
func (*putCmd) PrintUsage(w io.Writer, base string) {
	fmt.Fprintf(w, putUsage, base)
}

func (c *putCmd) SetFlags(f *flag.FlagSet) {
	c.auth = bootstrap.AuthAppDefault
	f.Var(&c.auth, "auth", fmt.Sprintf("authentication method (options: %s)", authOpts(false)))
	f.BoolVar(&c.force, "force", false, "force upload even if assets for this version exist")
	f.BoolVar(&c.public, "public", false, "make the new assets archive public")
	f.StringVar(&c.version, "version", common.Version, "the version to upload assets for")
	f.StringVar(&c.bucket, "bucket", "go-sweet-assets", "GCS bucket to upload assets to")
	f.StringVar(&c.assetsDir, "assets-dir", "./assets", "assets directory to tar, compress, and upload")
	f.StringVar(&c.assetsHashFile, "assets-hash-file", "./assets.hash", "file containing assets SHA256 hashes")
}

func (c *putCmd) Run(_ []string) error {
	log.SetActivityLog(true)

	if err := bootstrap.ValidateVersion(c.version); err != nil {
		return err
	}
	tf, err := ioutil.TempFile("", "go-sweet-assets")
	if err != nil {
		return err
	}
	defer tf.Close()

	log.Printf("Archiving and compressing: %s", c.assetsDir)
	if err := createAssetsArchive(tf, c.assetsDir, c.version); err != nil {
		return err
	}
	if _, err := tf.Seek(0, 0); err != nil {
		return err
	}
	log.Printf("Uploading archive to %s", c.bucket)
	if err := bootstrap.UploadArchive(tf, c.bucket, c.version, c.auth, c.force, c.public); err != nil {
		return err
	}
	if _, err := tf.Seek(0, 0); err != nil {
		return err
	}
	log.Printf("Updating hash file...")
	return hashAssetsArchive(tf, c.assetsHashFile, c.version, c.force)
}

func createAssetsArchive(w io.Writer, assetsDir, version string) error {
	gw := gzip.NewWriter(w)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(assetsDir, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		isSymlink := info.Mode()&os.ModeSymlink != 0
		link := ""
		if isSymlink {
			l, err := os.Readlink(fpath)
			if err != nil {
				return err
			}
			link = l
		}
		f, err := os.Open(fpath)
		if err != nil {
			return err
		}
		defer f.Close()
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name, err = filepath.Rel(assetsDir, fpath)
		if err != nil {
			panic(err)
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if isSymlink {
			// We don't need to copy any data for the symlink.
			return nil
		}
		_, err = io.Copy(tw, f)
		return err
	})
}

func hashAssetsArchive(tf io.Reader, hashfile, version string, force bool) error {
	vals, err := bootstrap.ReadHashesFile(hashfile)
	if err != nil {
		return err
	}
	hash, err := bootstrap.HashStream(tf)
	if err != nil {
		return err
	}
	if ok := vals.Put(version, hash, force); !ok {
		return fmt.Errorf("hash for this version already exists")
	}
	return vals.WriteToFile(hashfile)
}
