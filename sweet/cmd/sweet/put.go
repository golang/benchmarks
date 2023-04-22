// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
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
	f.StringVar(&c.version, "version", common.Version, "the version to upload assets for")
	f.StringVar(&c.bucket, "bucket", "go-sweet-assets", "GCS bucket to upload assets to")
	f.StringVar(&c.assetsDir, "assets-dir", "./assets", "assets directory to zip and upload")
	f.StringVar(&c.assetsHashFile, "assets-hash-file", "./assets.hash", "file containing assets SHA256 hashes")
}

func (c *putCmd) Run(_ []string) error {
	log.SetActivityLog(true)

	if err := bootstrap.ValidateVersion(c.version); err != nil {
		return err
	}

	log.Printf("Archiving, compressing, and uploading: %s", c.assetsDir)

	// Create storage writer for streaming.
	wc, err := bootstrap.NewStorageWriter(c.bucket, c.version, c.auth, c.force)
	if err != nil {
		return err
	}
	defer wc.Close()

	// Pass everything we write through a hash.
	hash := bootstrap.Hash()
	w := io.MultiWriter(wc, hash)

	// Write the archive.
	if err := createAssetsArchive(w, c.assetsDir, c.version); err != nil {
		return err
	}

	// Update hash file.
	log.Printf("Updating hash file...")
	return updateAssetsHash(bootstrap.CanonicalizeHash(hash), c.assetsHashFile, c.version, c.force)
}

func createAssetsArchive(w io.Writer, assetsDir, version string) (err error) {
	zw := zip.NewWriter(w)
	defer func() {
		if cerr := zw.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing zip archive: %w", cerr)
		}
	}()
	return filepath.Walk(assetsDir, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		outPath, err := filepath.Rel(assetsDir, fpath)
		if err != nil {
			// By the guarantees of filepath.Walk, this shouldn't happen.
			panic(err)
		}
		if info.IsDir() {
			// Add a trailing slash to indicate we're creating a directory.
			_, err := zw.Create(outPath + "/")
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("encountered symlink %s: symbolic links are not supported in assets", fpath)
		}
		// Create a file in our zip archive for writing.
		fh := new(zip.FileHeader)
		fh.Name = outPath
		fh.Method = zip.Deflate
		fh.SetMode(info.Mode())
		zf, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		// Open the original file for reading.
		f, err := os.Open(fpath)
		if err != nil {
			return err
		}
		defer f.Close()

		// Copy data into the archive.
		_, err = io.Copy(zf, f)
		return err
	})
}

func updateAssetsHash(hash, hashfile, version string, force bool) error {
	vals, err := bootstrap.ReadHashesFile(hashfile)
	if err != nil {
		return err
	}
	if ok := vals.Put(version, hash, force); !ok {
		return fmt.Errorf("hash for this version already exists")
	}
	return vals.WriteToFile(hashfile)
}
