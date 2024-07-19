// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/cgroups"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

var (
	goTool    string
	tmpDir    string
	toolexec  bool
	benchName string

	diag *driver.Diagnostics
)

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.StringVar(&goTool, "go", "", "path to cmd/go binary")
	flag.StringVar(&tmpDir, "tmp", "", "work directory (cleared before use)")
	flag.BoolVar(&toolexec, "toolexec", false, "run as a toolexec binary")
	flag.StringVar(&benchName, "bench-name", "", "for -toolexec")
	flag.Func("diagnostics", "for -toolexec", func(s string) error {
		diag = new(driver.Diagnostics)
		return diag.UnmarshalText([]byte(s))
	})
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

	cmdArgs := []string{goTool, "build", "-a"}

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

	diag = driver.NewDiagnostics(name)
	diagFlag, err := diag.MarshalText()
	if err != nil {
		return err
	}
	selfCmd = append(selfCmd, "-diagnostics", string(diagFlag))

	cmdArgs = append(cmdArgs, "-toolexec", strings.Join(selfCmd, " "))

	if df, err := diag.Create(diagnostics.Perf); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create %s diagnostics: %s\n", diagnostics.Perf, err)
	} else if df != nil {
		df.Close()
		defer df.Commit()

		perfArgs := []string{"perf", "record", "-o", df.Name()}
		perfArgs = append(perfArgs, driver.PerfFlags()...)
		perfArgs = append(perfArgs, cmdArgs...)
		cmdArgs = perfArgs
	}

	baseCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	baseCmd.Dir = pkgPath
	baseCmd.Env = common.NewEnvFromEnviron().MustSet("GOROOT=" + filepath.Dir(filepath.Dir(goTool))).Collapse()
	baseCmd.Stdout = os.Stdout
	baseCmd.Stderr = os.Stderr
	cmd, err := cgroups.WrapCommand(baseCmd, "test.scope")
	if err != nil {
		return err
	}
	err = driver.RunBenchmark(name, func(d *driver.B) error {
		defer diag.Commit(d)
		return cmd.Run()
	}, append(benchOpts, driver.DoAvgRSS(cmd.RSSFunc()))...)
	if err != nil {
		return err
	}
	return printOtherResults(tmpResultsDir())
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
		if bin == "link" && typ == diagnostics.Trace {
			// TODO(mknyszek): Traces are not supported for the linker.
			continue
		}

		subName := ""
		if !typ.CanMerge() {
			// Create a unique name for this diagnostic file.
			subName = fmt.Sprintf("%08x", rand.Uint32())
		}
		df, err := diag.CreateNamed(typ, subName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s diagnostics: %s\n", typ, err)
			continue
		} else if df != nil {
			df.Close()
			defer df.Commit()

			flag := "-" + string(typ)
			if typ == diagnostics.Trace {
				flag = "-traceprofile"
			}
			extraFlags = append(extraFlags, flag, df.Name())
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
