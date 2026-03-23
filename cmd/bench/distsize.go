package main

import (
	"fmt"
	"go/version"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func distsizes(tcs []*toolchain) error {
	tmpdir, err := os.MkdirTemp("", "go-distsize")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	for _, tc := range tcs {
		fmt.Printf("toolchain: %s\n", tc.Name)
		if err := distsize(tmpdir, tc); err != nil {
			return err
		}
	}
	return nil
}

func distsize(tmpdir string, tc *toolchain) error {
	goroot := filepath.Join(tmpdir, tc.Name)
	if err := os.Mkdir(goroot, 0o777); err != nil {
		return err
	}
	defer os.RemoveAll(goroot)

	// tc.GOROOT() is the GOROOT we are measuring. We're going to make a copy
	// of it from which we'll run make.(bash|bat|rc) -distpack. That will keep us from
	// modifying the original GOROOT. We will run distpack from h.goroot.
	if err := os.CopyFS(goroot, os.DirFS(tc.GOROOT())); err != nil {
		return fmt.Errorf("error copying GOROOT: %v", err)
	}
	goversion, err := tc.Go.Version()
	if err != nil {
		return err
	}
	goversion, _, _ = strings.Cut(goversion, " ") // remove date
	if !version.IsValid(goversion) {
		return fmt.Errorf("could not parse go version: %v", goversion)
	}
	versionFile := filepath.Join(goroot, "VERSION")
	if err := os.WriteFile(versionFile, []byte(goversion+"\n"), 0666); err != nil {
		return err
	}

	// Run make.(bash|bat|rc) -distpack in the temp GOROOT to produce the zip.
	var makeScript string
	switch runtime.GOOS {
	case "windows":
		makeScript = "make.bat"
	case "plan9":
		makeScript = "make.rc"
	default:
		makeScript = "make.bash"
	}
	cmd := exec.Command(filepath.Join(goroot, "src", makeScript), "-distpack")
	cmd.Dir = filepath.Join(goroot, "src")
	cmd.Env = tc.Env.MustSet("GOROOT_BOOTSTRAP=" + tc.GOROOT()).Collapse()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running %s -distpack: %v", makeScript, err)
	}

	zipBaseName := fmt.Sprintf("v0.0.1-%s.%s-%s.zip", goversion, runtime.GOOS, runtime.GOARCH)
	zipFile := filepath.Join(goroot, "pkg", "distpack", zipBaseName)
	fi, err := os.Stat(zipFile)
	if err != nil {
		return fmt.Errorf("could not find module distribution zip file: %v", err)
	}
	size := fi.Size()

	fmt.Println("Unit total-bytes assume=exact")
	fmt.Printf("BenchmarkGoDistribution 1 %d total-bytes\n", size)
	return nil
}
