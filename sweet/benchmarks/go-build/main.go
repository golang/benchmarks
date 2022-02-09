// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/pprof/profile"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/cgroups"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/common"
)

var (
	goTool    string
	tmpDir    string
	toolexec  bool
	benchName string
)

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.StringVar(&goTool, "go", "", "path to cmd/go binary")
	flag.StringVar(&tmpDir, "tmp", "", "work directory (cleared before use)")
	flag.BoolVar(&toolexec, "toolexec", false, "run as a toolexec binary")
	flag.StringVar(&benchName, "bench-name", "", "for -toolexec")
}

func tmpResultsDir() string {
	return filepath.Join(tmpDir, "results")
}

var benchOpts = []driver.RunOption{
	driver.DoTime(true),
}

func run(pkgPath string) error {
	// Clear any stale results from previous runs and recreate
	// the directory.
	if err := os.RemoveAll(tmpResultsDir()); err != nil {
		return err
	}
	if err := os.MkdirAll(tmpResultsDir(), 0777); err != nil {
		return err
	}

	name := "GoBuild" + strings.Title(filepath.Base(pkgPath))

	cmdArgs := []string{"build", "-a"}

	// Build a command comprised of this binary to pass to -toolexec.
	selfPath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return err
	}
	selfCmd := []string{
		selfPath, "-toolexec",
		"-bench-name", name,
	}
	flag.CommandLine.Visit(func(f *flag.Flag) {
		if f.Name == "go" || f.Name == "bench-name" {
			// No need to pass this along.
			return
		}
		selfCmd = append(selfCmd, "-"+f.Name, f.Value.String())
	})

	cmdArgs = append(cmdArgs, "-toolexec", strings.Join(selfCmd, " "))
	var baseCmd *exec.Cmd
	if driver.ProfilingEnabled(driver.ProfilePerf) {
		baseCmd = exec.Command("perf", append([]string{"record", "-o", filepath.Join(tmpDir, "perf.data"), goTool}, cmdArgs...)...)
	} else {
		baseCmd = exec.Command(goTool, cmdArgs...)
	}
	baseCmd.Dir = pkgPath
	baseCmd.Env = common.NewEnvFromEnviron().MustSet("GOROOT=" + filepath.Dir(filepath.Dir(goTool))).Collapse()
	cmd, err := cgroups.WrapCommand(baseCmd, "test.scope")
	if err != nil {
		return err
	}
	err = driver.RunBenchmark(name, func(d *driver.B) error {
		return cmd.Run()
	}, append(benchOpts, driver.DoAvgRSS(cmd.RSSFunc()))...)
	if err != nil {
		return err
	}

	// Handle any CPU profiles produced, and merge them.
	// Then, write them out to the canonical profiles above.
	if driver.ProfilingEnabled(driver.ProfileCPU) {
		compileProfile, err := mergeProfiles(tmpDir, profilePrefix("compile", driver.ProfileCPU))
		if err != nil {
			return err
		}
		if err := driver.WriteProfile(compileProfile, driver.ProfileCPU, name+"Compile"); err != nil {
			return err
		}

		linkProfile, err := mergeProfiles(tmpDir, profilePrefix("link", driver.ProfileCPU))
		if err != nil {
			return err
		}
		if err := driver.WriteProfile(linkProfile, driver.ProfileCPU, name+"Link"); err != nil {
			return err
		}
	}
	if driver.ProfilingEnabled(driver.ProfileMem) {
		if err := copyProfiles(tmpDir, "compile", driver.ProfileMem, name+"Compile"); err != nil {
			return err
		}
		if err := copyProfiles(tmpDir, "link", driver.ProfileMem, name+"Link"); err != nil {
			return err
		}
	}
	if driver.ProfilingEnabled(driver.ProfilePerf) {
		if err := driver.CopyProfile(filepath.Join(tmpDir, "perf.data"), driver.ProfilePerf, name); err != nil {
			return err
		}
	}
	return printOtherResults(tmpResultsDir())
}

func mergeProfiles(dir, prefix string) (*profile.Profile, error) {
	profiles, err := collectProfiles(dir, prefix)
	if err != nil {
		return nil, err
	}
	return profile.Merge(profiles)
}

func copyProfiles(dir, bin string, typ driver.ProfileType, finalPrefix string) error {
	profiles, err := collectProfiles(dir, profilePrefix(bin, typ))
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if err := driver.WriteProfile(profile, typ, finalPrefix); err != nil {
			return err
		}
	}
	return nil
}

func collectProfiles(dir, prefix string) ([]*profile.Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var profiles []*profile.Profile
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(tmpDir, name)
		if info, err := entry.Info(); err != nil {
			return nil, err
		} else if info.Size() == 0 {
			// Skip zero-sized files, otherwise the pprof package
			// will call it a parsing error.
			continue
		}
		if strings.HasPrefix(name, prefix) {
			p, err := driver.ReadProfile(path)
			if err != nil {
				return nil, err
			}
			profiles = append(profiles, p)
			continue
		}
	}
	return profiles, nil
}

func profilePrefix(bin string, typ driver.ProfileType) string {
	return bin + "-prof." + string(typ)
}

func printOtherResults(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		if strings.HasSuffix(name, ".results") {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(os.Stderr, f); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

func runToolexec() error {
	var benchSuffix string
	benchmark := false
	bin := filepath.Base(flag.Arg(0))
	switch bin {
	case "compile":
	case "link":
		benchSuffix = "Link"
		benchmark = true
	default:
		cmd := exec.Command(flag.Args()[0], flag.Args()[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	var extraFlags []string
	for _, typ := range []driver.ProfileType{driver.ProfileCPU, driver.ProfileMem} {
		if driver.ProfilingEnabled(typ) {
			// Stake a claim for a filename.
			f, err := os.CreateTemp(tmpDir, profilePrefix(bin, typ))
			if err != nil {
				return err
			}
			f.Close()
			extraFlags = append(extraFlags, "-"+string(typ)+"profile", f.Name())
		}
	}
	cmd := exec.Command(flag.Args()[0], append(extraFlags, flag.Args()[1:]...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if benchmark {
		name := benchName + benchSuffix
		f, err := os.Create(filepath.Join(tmpResultsDir(), name+".results"))
		if err != nil {
			return err
		}
		defer f.Close()
		return driver.RunBenchmark(name, func(d *driver.B) error {
			return cmd.Run()
		}, append(benchOpts, driver.WriteResultsTo(f))...)
	}
	return cmd.Run()
}

func main() {
	flag.Parse()

	if toolexec {
		if err := runToolexec(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "error: expected one argument\n")
		os.Exit(1)
	}
	if err := run(flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
