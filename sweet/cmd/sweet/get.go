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
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/benchmarks/sweet/cli/bootstrap"
	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/fileutil"
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
	copyAssets     bool
	cache          string
	bucket         string
	assetsDir      string
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
	f.BoolVar(&c.copyAssets, "copy", false, "copy assets to assets-dir instead of symlinking")
	f.StringVar(&c.cache, "cache", bootstrap.CacheDefault(), "cache location for tar'd and compressed assets, if set to \"\" will ignore cache")
	f.StringVar(&c.version, "version", common.Version, "the version to download assets for")
	f.StringVar(&c.bucket, "bucket", "go-sweet-assets", "GCS bucket to download assets from")
	f.StringVar(&c.assetsDir, "assets-dir", "./assets", "location to extract assets into")
	f.StringVar(&c.assetsHashFile, "assets-hash-file", "./assets.hash", "file to check SHA256 hash of the downloaded artifact against")
}

func (c *getCmd) Run(_ []string) error {
	log.SetActivityLog(true)
	if err := bootstrap.ValidateVersion(c.version); err != nil {
		return err
	}
	installAssets := func(todir string, readonly bool) error {
		return downloadAndExtract(todir, c.bucket, c.assetsHashFile, c.version, c.auth, readonly)
	}
	if c.cache == "" {
		log.Printf("Skipping cache...")
		return installAssets(c.assetsDir, false)
	}
	log.Printf("Checking cache: %s", c.cache)
	t, err := bootstrap.CachedAssets(c.cache, c.version)
	if err == bootstrap.ErrNotInCache || (err == nil && c.force) {
		if err := installAssets(t, true); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if !c.copyAssets {
		log.Printf("Creating symlink to %s", c.assetsDir)
		if info, err := os.Lstat(c.assetsDir); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				// We have a symlink, so just delete it so we can replace it.
				if err := os.Remove(c.assetsDir); err != nil {
					return fmt.Errorf("installing assets: removing %s: %v", c.assetsDir, err)
				}
			} else {
				return fmt.Errorf("installing assets: %s is not a symlink; to install assets here, remove it and re-run this command", c.assetsDir)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %v", c.assetsDir, err)
		}
		return os.Symlink(t, c.assetsDir)
	}
	if _, err := os.Stat(c.assetsDir); err == nil {
		return fmt.Errorf("installing assets: %s exists; to copy assets here, remove it and re-run this command", c.assetsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %v", c.assetsDir, err)
	}
	log.Printf("Copying assets %s", c.assetsDir)
	return fileutil.CopyDir(c.assetsDir, t)
}

func downloadAndExtract(todir, bucket, hashfile, version string, auth bootstrap.AuthOption, readonly bool) error {
	log.Printf("Downloading assets archive for version %s to %s", version, todir)

	// Create storage reader for streaming.
	rc, err := bootstrap.NewStorageReader(bucket, version, auth)
	if err != nil {
		return err
	}
	defer rc.Close()

	// Pass everything we read through a hash.
	hash := bootstrap.Hash()
	r := io.TeeReader(rc, hash)

	// Stream and extract the results.
	if err := extractAssets(r, todir, readonly); err != nil {
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

func extractAssets(r io.Reader, outdir string, readonly bool) error {
	if err := os.MkdirAll(outdir, os.ModePerm); err != nil {
		return fmt.Errorf("create assets directory: %v", err)
	}
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		fullpath := filepath.Join(outdir, hdr.Name)
		if err := os.MkdirAll(filepath.Dir(fullpath), os.ModePerm); err != nil {
			return err
		}
		f, err := os.Create(fullpath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		fperm := os.FileMode(uint32(hdr.Mode))
		if readonly {
			fperm = 0444 | (fperm & 0555)
		}
		if err := f.Chmod(fperm); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	if readonly {
		return filepath.Walk(outdir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return os.Chmod(path, 0555)
			}
			return nil
		})
	}
	return nil
}
