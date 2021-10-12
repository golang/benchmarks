// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"
)

// TODO(prattmic): refactor bent to export Todo so we can directly build this
// in Go.
var configurationTmpl = template.Must(template.New("configuration").Parse(`
[[Configurations]]
  Name = "Benchmark"
  Root = "{{.}}"
`))

func writeConfiguration(filename, goroot string) error {
	var buf bytes.Buffer
	if err := configurationTmpl.Execute(&buf, goroot); err != nil {
		return fmt.Errorf("error generating configuration: %w", err)
	}

	log.Printf("bent configuration for GOROOT %s:\n%s", goroot, buf.String())

	if err := os.WriteFile(filename, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("error creating configurations.toml: %w", err)
	}

	return nil
}

// removeAllIncludingReadonly is like os.RemoveAll except that it'll
// also try to change permissions to work around permission errors
// when deleting.
func removeAllIncludingReadonly(dir string) error {
	err := os.RemoveAll(dir)
	if err == nil || !os.IsPermission(err) || runtime.GOOS == "windows" /* different fs permission model */ {
		return err
	}
	// Make a best effort (ignoring errors) attempt to make all
	// files and directories writable before we try to delete them
	// all again.
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		const ownerWritable = 0200
		if err != nil || fi.Mode().Perm()&ownerWritable != 0 {
			return nil
		}
		os.Chmod(path, fi.Mode().Perm()|ownerWritable)
		return nil
	})
	return os.RemoveAll(dir)
}

func bent(goroot string) (err error) {
	dir, err := os.MkdirTemp("", "bent")
	if err != nil {
		return fmt.Errorf("error creating temporary directory: %w", err)
	}
	defer func() {
		err = removeAllIncludingReadonly(dir)
		if err != nil {
			err = fmt.Errorf("error removing temporary directory: %w", err)
		}
	}()
	log.Printf("Bent temporary directory: %s", dir)

	bentPath := filepath.Join(dir, "bent")

	log.Printf("Building bent...")

	// Build bent itself. N.B. we don't need to do this with the goroot
	// under test since we aren't testing bent itself, but we are sure that
	// this toolchain exists.
	//
	// TODO(prattmic): do this only once on first call?
	cmd := goCommand(goroot, "build", "-o", bentPath, "golang.org/x/benchmarks/cmd/bent")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error building bent: %w", err)
	}

	log.Printf("Initializing bent...")

	// Initialize scratch dir for bent.
	cmd = exec.Command(bentPath, "-I")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running bent -I: %w", err)
	}

	confFile := filepath.Join(dir, "configurations.toml")
	if err := writeConfiguration(confFile, goroot); err != nil {
		return fmt.Errorf("error writing configuration: %w", err)
	}

	log.Printf("Running bent...")

	// Finally we can actually run the benchmarks.
	cmd = exec.Command(bentPath, "-C", confFile, "-B", filepath.Join(dir, "benchmarks-50.toml"))
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running bent -I: %w", err)
	}

	return nil
}
