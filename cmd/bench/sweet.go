// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/benchmarks/sweet/common"
)

func writeSweetConfiguration(filename string, tcs []*toolchain) error {
	var cfg common.ConfigFile
	for _, tc := range tcs {
		cfg.Configs = append(cfg.Configs, &common.Config{
			Name:   tc.Name,
			GoRoot: tc.GOROOT(),
		})
	}
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating configuration file for Sweet: %w", err)
	}
	defer f.Close()
	b, err := common.ConfigFileMarshalTOML(&cfg)
	if err != nil {
		return fmt.Errorf("error marshaling configuration file for Sweet: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("error writing configuration file for Sweet: %w", err)
	}
	return nil
}

func sweet(tcs []*toolchain) (err error) {
	tmpDir, err := os.MkdirTemp("", "go-sweet")
	if err != nil {
		return fmt.Errorf("error creating temporary directory: %w", err)
	}
	defer func() {
		r := removeAllIncludingReadonly(tmpDir)
		if err == nil && r != nil {
			err = fmt.Errorf("error removing temporary directory: %w", r)
		} else if err != nil && r != nil {
			log.Printf("failed to clean up sweet temporary directory: %v", r)
		}
	}()
	log.Printf("Sweet temporary directory: %s", tmpDir)

	dirBytes, err := tcs[0].List("-f", "{{.Dir}}", "golang.org/x/benchmarks/sweet/cmd/sweet")
	if err != nil {
		return fmt.Errorf("finding sweet root: %w", err)
	}
	sweetRoot := filepath.Dir(filepath.Dir(string(dirBytes)))
	sweetBin := filepath.Join(tmpDir, "sweet")

	log.Printf("Building Sweet...")

	// Build Sweet itself. N.B. we don't need to do this with the goroot
	// under test since we aren't testing sweet itself, but we are sure that
	// this toolchain exists.
	if err := tcs[0].BuildPackage("golang.org/x/benchmarks/sweet/cmd/sweet", sweetBin); err != nil {
		return fmt.Errorf("building sweet: %v", err)
	}

	log.Printf("Initializing Sweet...")

	var assetsCacheDir string
	if os.Getenv("GO_BUILDER_NAME") != "" {
		// Be explicit that we want /tmp, because the builder is
		// going to try and give us /workdir/tmp which will not
		// have enough space for us.
		assetsCacheDir = filepath.Join("/", "tmp", "go-sweet-assets")
	} else {
		assetsCacheDir = filepath.Join(tmpDir, "assets")
	}
	cmd := exec.Command(
		sweetBin, "get",
		"-cache", assetsCacheDir,
		"-auth", "none",
		"-assets-hash-file", filepath.Join(sweetRoot, "assets.hash"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running sweet get: %w", err)
	}

	confFile := filepath.Join(tmpDir, "config.toml")
	if err := writeSweetConfiguration(confFile, tcs); err != nil {
		return fmt.Errorf("error writing configuration: %w", err)
	}

	log.Printf("Running Sweet...")

	// Finally we can actually run the benchmarks.
	resultsDir := filepath.Join(tmpDir, "results")
	workDir := filepath.Join(tmpDir, "work")
	cmd = exec.Command(
		sweetBin, "run",
		"-run", "all",
		"-count", "10",
		"-bench-dir", filepath.Join(sweetRoot, "benchmarks"),
		"-cache", assetsCacheDir,
		"-work-dir", workDir,
		"-results", resultsDir,
		confFile,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Don't fail immediately. Let's try to dump whatever results we have first.
	sweetErr := cmd.Run()
	defer func() {
		// Any sweet errors take precedence over errors encountered in printing
		// results.
		if sweetErr != nil {
			if err != nil {
				log.Printf("error dumping results: %v", err)
			}
			err = fmt.Errorf("error running sweet run: %w", sweetErr)
		}
	}()

	// Dump results to stdout.
	for _, tc := range tcs {
		matches, err := filepath.Glob(filepath.Join(resultsDir, "*", fmt.Sprintf("%s.results", tc.Name)))
		if err != nil {
			return fmt.Errorf("searching for results for %s in %s: %v", tc.Name, resultsDir, err)
		}
		fmt.Printf("toolchain: %s\n", tc.Name)
		for _, match := range matches {
			// Print pkg and shortname tags because Sweet won't do it.
			benchName := filepath.Base(filepath.Dir(match))
			fmt.Printf("pkg: golang.org/x/benchmarks/sweet/benchmarks/%s\n", benchName)
			fmt.Printf("shortname: sweet_%s\n", strings.ReplaceAll(benchName, "-", "_"))

			// Dump results file.
			f, err := os.Open(match)
			if err != nil {
				return fmt.Errorf("opening result %s: %v", match, err)
			}
			if _, err := io.Copy(os.Stdout, f); err != nil {
				f.Close()
				return fmt.Errorf("reading result %s: %v", match, err)
			}
			f.Close()
		}
	}
	return nil
}
