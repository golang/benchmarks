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
	"golang.org/x/benchmarks/sweet/common/diagnostics"
	sprofile "golang.org/x/benchmarks/sweet/common/profile"
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
		if f.Name == "go" || f.Name == "bench-name" || strings.HasPrefix(f.Name, "perf") {
			// No need to pass this along.
			return
		}
		selfCmd = append(selfCmd, "-"+f.Name, f.Value.String())
	})

	cmdArgs = append(cmdArgs, "-toolexec", strings.Join(selfCmd, " "))
	var baseCmd *exec.Cmd
	if driver.DiagnosticEnabled(diagnostics.Perf) {
		perfArgs := []string{"record", "-o", filepath.Join(tmpDir, "perf.data")}
		perfArgs = append(perfArgs, driver.PerfFlags()...)
		perfArgs = append(perfArgs, goTool)
		perfArgs = append(perfArgs, cmdArgs...)
		baseCmd = exec.Command("perf", perfArgs...)
	} else {
		baseCmd = exec.Command(goTool, cmdArgs...)
	}
	baseCmd.Dir = pkgPath
	baseCmd.Env = common.NewEnvFromEnviron().MustSet("GOROOT=" + filepath.Dir(filepath.Dir(goTool))).Collapse()
	baseCmd.Stdout = os.Stdout
	baseCmd.Stderr = os.Stderr
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
	if driver.DiagnosticEnabled(diagnostics.CPUProfile) {
		compileProfile, err := mergePprofProfiles(tmpDir, profilePrefix("compile", diagnostics.CPUProfile))
		if err != nil {
			return err
		}
		if err := driver.WritePprofProfile(compileProfile, diagnostics.CPUProfile, name+"Compile"); err != nil {
			return err
		}

		linkProfile, err := mergePprofProfiles(tmpDir, profilePrefix("link", diagnostics.CPUProfile))
		if err != nil {
			return err
		}
		if err := driver.WritePprofProfile(linkProfile, diagnostics.CPUProfile, name+"Link"); err != nil {
			return err
		}
	}
	if driver.DiagnosticEnabled(diagnostics.MemProfile) {
		if err := copyPprofProfiles(tmpDir, "compile", diagnostics.MemProfile, name+"Compile"); err != nil {
			return err
		}
		if err := copyPprofProfiles(tmpDir, "link", diagnostics.MemProfile, name+"Link"); err != nil {
			return err
		}
	}
	if driver.DiagnosticEnabled(diagnostics.Perf) {
		if err := driver.CopyDiagnosticData(filepath.Join(tmpDir, "perf.data"), diagnostics.Perf, name); err != nil {
			return err
		}
	}
	if driver.DiagnosticEnabled(diagnostics.Trace) {
		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), profilePrefix("compile", diagnostics.Trace)) {
				continue
			}
			if err := driver.CopyDiagnosticData(filepath.Join(tmpDir, entry.Name()), diagnostics.Trace, name+"Compile"); err != nil {
				return err
			}
		}
	}
	return printOtherResults(tmpResultsDir())
}

func mergePprofProfiles(dir, prefix string) (*profile.Profile, error) {
	profiles, err := sprofile.ReadDirPprof(dir, func(name string) bool {
		return strings.HasPrefix(name, prefix)
	})
	if err != nil {
		return nil, err
	}
	return profile.Merge(profiles)
}

func copyPprofProfiles(dir, bin string, typ diagnostics.Type, finalPrefix string) error {
	prefix := profilePrefix(bin, typ)
	profiles, err := sprofile.ReadDirPprof(dir, func(name string) bool {
		return strings.HasPrefix(name, prefix)
	})
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if err := driver.WritePprofProfile(profile, typ, finalPrefix); err != nil {
			return err
		}
	}
	return nil
}

func profilePrefix(bin string, typ diagnostics.Type) string {
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
	for _, typ := range []diagnostics.Type{diagnostics.CPUProfile, diagnostics.MemProfile, diagnostics.Trace} {
		if driver.DiagnosticEnabled(typ) {
			if bin == "link" && typ == diagnostics.Trace {
				// TODO(mknyszek): Traces are not supported for the linker.
				continue
			}
			// Stake a claim for a filename.
			f, err := os.CreateTemp(tmpDir, profilePrefix(bin, typ))
			if err != nil {
				return err
			}
			f.Close()
			flag := "-" + string(typ)
			if typ == diagnostics.Trace {
				flag += "profile" // The compiler flag is -traceprofile.
			}
			extraFlags = append(extraFlags, flag, f.Name())
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
