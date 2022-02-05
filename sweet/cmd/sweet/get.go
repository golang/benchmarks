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
	"strings"

	"golang.org/x/benchmarks/sweet/cli/bootstrap"
	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

const (
	getUsage = `Retrieves assets for benchmarks from GCS.

Usage: %s get [flags]
`
)

func authOpts(includeNone bool) string {
	i := bootstrap.AuthOption(0)
	if !includeNone {
		i = 1
	}
	s := make([]string, 0, bootstrap.NumAuthOptions)
	for ; i < bootstrap.NumAuthOptions; i++ {
		s = append(s, i.String())
	}
	return strings.Join(s, ", ")
}

type getCmd struct {
	auth           bootstrap.AuthOption
	force          bool
	cache          string
	bucket         string
	copyDir        string
	assetsHashFile string
	version        string
}

func (*getCmd) Name() string     { return "get" }
func (*getCmd) Synopsis() string { return "Retrieves assets for benchmarks." }
func (*getCmd) PrintUsage(w io.Writer, base string) {
	fmt.Fprintf(w, getUsage, base)
}

func (c *getCmd) SetFlags(f *flag.FlagSet) {
	f.Var(&c.auth, "auth", fmt.Sprintf("authentication method (options: %s)", authOpts(true)))
	f.BoolVar(&c.force, "force", false, "force download even if assets for this version exist in the cache")
	f.StringVar(&c.cache, "cache", bootstrap.CacheDefault(), "cache location for assets")
	f.StringVar(&c.version, "version", common.Version, "the version to download assets for")
	f.StringVar(&c.bucket, "bucket", "go-sweet-assets", "GCS bucket to download assets from")
	f.StringVar(&c.copyDir, "copy", "", "location to extract assets into, useful for development")
	f.StringVar(&c.assetsHashFile, "assets-hash-file", "./assets.hash", "file to check SHA256 hash of the downloaded artifact against")
}

func (c *getCmd) Run(_ []string) error {
	log.SetActivityLog(true)
	if err := bootstrap.ValidateVersion(c.version); err != nil {
		return err
	}
	if c.copyDir == "" && c.cache == "" {
		log.Printf("No cache to populate and assets are not copied. Nothing to do.")
		return nil
	}

	// Create a file that we'll download assets into.
	var (
		f     *os.File
		fName string
		err   error
	)
	if c.cache == "" {
		// There's no cache, which means we'll be extracting directly.
		// Just create a temporary file. zip archives cannot be streamed
		// out unfortunately (the API requires a ReaderAt).
		f, err = os.CreateTemp("", "go-sweet-assets")
		if err != nil {
			return err
		}
		defer f.Close()
		fName = f.Name()
	} else {
		// There is a cache, so create a file in the cache if there isn't
		// one already.
		log.Printf("Checking cache: %s", c.cache)
		fName, err = bootstrap.CachedAssets(c.cache, c.version)
		if err == bootstrap.ErrNotInCache || (err == nil && c.force) {
			f, err = os.Create(fName)
			if err != nil {
				return err
			}
			defer f.Close()
		} else if err != nil {
			return err
		}
	}

	// If f is not nil, then we need to download assets.
	// Otherwise they're in a cache.
	if f != nil {
		// Download the compressed assets into f.
		if err := downloadAssets(f, c.bucket, c.assetsHashFile, c.version, c.auth); err != nil {
			return err
		}
	}
	// If we're not copying, we're done.
	if c.copyDir == "" {
		return nil
	}
	if f == nil {
		// Since f is nil, and we'll be extracting, we need to open the file.
		f, err := os.Open(fName)
		if err != nil {
			return err
		}
		defer f.Close()
	}

	// Check to make sure out destination is clear.
	if _, err := os.Stat(c.copyDir); err == nil {
		return fmt.Errorf("installing assets: %s exists; to copy assets here, remove it and re-run this command", c.copyDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %v", c.copyDir, err)
	}

	// Extract assets into assetsDir.
	log.Printf("Copying assets to %s", c.copyDir)
	return extractAssets(f, c.copyDir)
}

func downloadAssets(toFile *os.File, bucket, hashfile, version string, auth bootstrap.AuthOption) error {
	log.Printf("Downloading assets archive for version %s to %s", version, toFile.Name())

	// Create storage reader for streaming.
	rc, err := bootstrap.NewStorageReader(bucket, version, auth)
	if err != nil {
		return err
	}
	defer rc.Close()

	// Pass everything we read through a hash.
	hash := bootstrap.Hash()
	r := io.TeeReader(rc, hash)

	// Stream the results.
	if _, err := io.Copy(toFile, r); err != nil {
		return err
	}

	// Check the hash.
	return checkAssetsHash(bootstrap.CanonicalizeHash(hash), hashfile, version)
}

func checkAssetsHash(hash, hashfile, version string) error {
	vals, err := bootstrap.ReadHashesFile(hashfile)
	if err != nil {
		return err
	}
	check, ok := vals.Get(version)
	if !ok {
		return fmt.Errorf("hash for version %s not found", version)
	}
	if hash != check {
		return fmt.Errorf("downloaded artifact has unexpected hash: expected %s, got %s", hash, check)
	}
	return nil
}

func extractAssets(archive *os.File, outdir string) error {
	if err := os.MkdirAll(outdir, os.ModePerm); err != nil {
		return fmt.Errorf("create assets directory: %w", err)
	}
	archiveInfo, err := archive.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(archive, archiveInfo.Size())
	if err != nil {
		return err
	}
	for _, zf := range zr.File {
		err := func(zf *zip.File) error {
			fullpath := filepath.Join(outdir, zf.Name)
			if err := os.MkdirAll(filepath.Dir(fullpath), os.ModePerm); err != nil {
				return err
			}
			inFile, err := zf.Open()
			if err != nil {
				return err
			}
			defer inFile.Close()
			outFile, err := os.Create(fullpath)
			if err != nil {
				return err
			}
			defer outFile.Close()
			if _, err := io.Copy(outFile, inFile); err != nil {
				return err
			}
			if err := outFile.Chmod(zf.Mode()); err != nil {
				return err
			}
			return nil
		}(zf)
		if err != nil {
			return err
		}
	}
	return nil
}
