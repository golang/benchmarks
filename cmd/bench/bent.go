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
{{- range . -}}
[[Configurations]]
  Name = "{{.Name}}"
  Root = "{{.GOROOT}}"
  AfterBuild = ["benchsize"]

{{end -}}
`))

func writeBentConfiguration(filename string, tcs []*toolchain) error {
	var buf bytes.Buffer
	if err := configurationTmpl.Execute(&buf, tcs); err != nil {
		return fmt.Errorf("error generating configuration: %w", err)
	}

	log.Printf("bent configuration:\n%s", buf.String())

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

func bent(tcs []*toolchain, pgo bool) (err error) {
	if pgo {
		log.Printf("Skipping bent benchmarks (PGO not supported)")
		return nil
	}
	dir, err := os.MkdirTemp("", "bent")
	if err != nil {
		return fmt.Errorf("error creating temporary directory: %w", err)
	}
	defer func() {
		r := removeAllIncludingReadonly(dir)
		if r != nil && err == nil {
			err = fmt.Errorf("error removing temporary directory: %w", err)
		} else if r != nil {
			log.Printf("error removing temporary directory: %v", err)
		}
	}()
	log.Printf("Bent temporary directory: %s", dir)

	log.Printf("Building bent...")

	// Build bent itself. Just pick any toolchain we have; it doesn't matter which.
	// We're not benchmarking bent itself, it's just a driver.
	bentPath := filepath.Join(dir, "bent")
	if err := tcs[0].BuildPackage("golang.org/x/benchmarks/cmd/bent", bentPath); err != nil {
		return fmt.Errorf("building bent: %w", err)
	}

	log.Printf("Initializing bent...")

	// Initialize scratch dir for bent.
	cmd := exec.Command(bentPath, "-I")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running bent -I: %w", err)
	}

	confFile := filepath.Join(dir, "configurations.toml")
	if err := writeBentConfiguration(confFile, tcs); err != nil {
		return fmt.Errorf("error writing configuration: %w", err)
	}

	log.Printf("Running bent...")

	// Finally we can actually run the benchmarks.
	// N.B. bent prints the "toolchain" tag to indicate which toolchain is being used.
	// It's passed to bent via the TOML configuration.
	cmd = exec.Command(bentPath,
		"-N", "10",
		"-C", confFile,
		"-B", filepath.Join(dir, "benchmarks-50.toml"),
		"-report-build-time=false", // We only run builds once, which won't yield statistically significant results.
		"-v",
	)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running bent: %w", err)
	}
	return nil
}
