// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/cgroups"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

var (
	esbuildBin string
	esbuildSrc string
	tmpDir     string
	benchName  string
)

func init() {
	driver.SetFlags(flag.CommandLine)
	flag.StringVar(&esbuildBin, "bin", "", "path to esbuild binary")
	flag.StringVar(&esbuildSrc, "src", "", "path to JS/TS to pack")
	flag.StringVar(&tmpDir, "tmp", "", "work directory (cleared before use)")
	flag.StringVar(&benchName, "bench", "", "benchmark name")
}

func main() {
	flag.Parse()
	if esbuildBin == "" {
		fmt.Fprintln(os.Stderr, "expected non-empty bin flag")
		os.Exit(1)
	}
	if esbuildSrc == "" {
		fmt.Fprintln(os.Stderr, "expected non-empty src flag")
		os.Exit(1)
	}
	if tmpDir == "" {
		fmt.Fprintln(os.Stderr, "expected non-empty tmp flag")
		os.Exit(1)
	}
	if err := run(benchName, esbuildBin, esbuildSrc, tmpDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var benchArgsFuncs = map[string]func(src, tmp string) []string{
	// Args taken from https://github.com/evanw/esbuild/blob/main/Makefile.
	"ThreeJS": func(src, tmp string) []string {
		return []string{"--bundle",
			"--global-name=THREE",
			"--sourcemap",
			"--minify",
			"--timing",
			"--outfile=" + filepath.Join(tmp, "out-three.js"),
			filepath.Join(src, "src", "entry.js"),
		}
	},
	"RomeTS": func(src, tmp string) []string {
		return []string{"--bundle",
			"--platform=node",
			"--sourcemap",
			"--minify",
			"--timing",
			"--outfile=" + filepath.Join(tmp, "out-rome.js"),
			filepath.Join(src, "src", "entry.ts"),
		}
	},
	"ReactAdminJS": func(src, tmp string) []string {
		return []string{
			"--alias:data-generator-retail=" + filepath.Join(src, "repo/examples/data-generator/src"),
			"--alias:ra-core=" + filepath.Join(src, "repo/packages/ra-core/src"),
			"--alias:ra-data-fakerest=" + filepath.Join(src, "repo/packages/ra-data-fakerest/src"),
			"--alias:ra-data-graphql-simple=" + filepath.Join(src, "repo/packages/ra-data-graphql-simple/src"),
			"--alias:ra-data-graphql=" + filepath.Join(src, "repo/packages/ra-data-graphql/src"),
			"--alias:ra-data-simple-rest=" + filepath.Join(src, "repo/packages/ra-data-simple-rest/src"),
			"--alias:ra-i18n-polyglot=" + filepath.Join(src, "repo/packages/ra-i18n-polyglot/src"),
			"--alias:ra-input-rich-text=" + filepath.Join(src, "repo/packages/ra-input-rich-text/src"),
			"--alias:ra-language-english=" + filepath.Join(src, "repo/packages/ra-language-english/src"),
			"--alias:ra-language-french=" + filepath.Join(src, "repo/packages/ra-language-french/src"),
			"--alias:ra-ui-materialui=" + filepath.Join(src, "repo/packages/ra-ui-materialui/src"),
			"--alias:react-admin=" + filepath.Join(src, "repo/packages/react-admin/src"),
			"--bundle",
			"--define:process.env.REACT_APP_DATA_PROVIDER=null",
			"--format=esm",
			"--loader:.png=file",
			"--loader:.svg=file",
			"--minify",
			"--sourcemap",
			"--splitting",
			"--target=esnext",
			"--timing",
			"--outdir=" + filepath.Join(tmp, "out-readmin"),
			filepath.Join(src, "repo/examples/demo/src/index.tsx"),
		}
	},
}

func run(name, bin, src, tmp string) error {
	// Get the args for this benchmark.
	argsFunc, ok := benchArgsFuncs[name]
	if !ok {
		return fmt.Errorf("unknown benchmark %s", name)
	}
	cmdArgs := append([]string{bin}, argsFunc(src, tmp)...)

	// Add prefix to benchmark name.
	name = "ESBuild" + name

	// Set up diagnostics.
	var diagFiles []*driver.DiagnosticFile
	diag := driver.NewDiagnostics(name)
	if df, err := diag.Create(diagnostics.Perf); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create %s diagnostics: %s\n", diagnostics.Perf, err)
	} else if df != nil {
		df.Close()
		diagFiles = append(diagFiles, df)

		perfArgs := []string{"perf", "record", "-o", df.Name()}
		perfArgs = append(perfArgs, driver.PerfFlags()...)
		perfArgs = append(perfArgs, cmdArgs...)
		cmdArgs = perfArgs
	}
	for _, typ := range []diagnostics.Type{diagnostics.CPUProfile, diagnostics.MemProfile, diagnostics.Trace} {
		df, err := diag.Create(typ)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s diagnostics: %s\n", typ, err)
			continue
		} else if df != nil {
			df.Close()
			diagFiles = append(diagFiles, df)

			flag := "--" + string(typ)
			if typ == diagnostics.MemProfile {
				flag = "--heap"
			}
			// N.B. Flags in esbuild are fairly idiosyncratic. Flags that accept a parameter
			// need to appear after an "=" character without spaces between the flag or the
			// parameter.
			cmdArgs = append(cmdArgs, flag+"="+df.Name())
		}
	}

	baseCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	baseCmd.Stdout = os.Stderr // Redirect all tool output to stderr.
	baseCmd.Stderr = os.Stderr
	cmd, err := cgroups.WrapCommand(baseCmd, "test.scope")
	if err != nil {
		return err
	}
	return driver.RunBenchmark(name, func(d *driver.B) error {
		defer diag.Commit(d)
		defer func() {
			for _, df := range diagFiles {
				df.Commit()
			}
		}()
		defer d.StopTimer()
		return cmd.Run()
	}, []driver.RunOption{driver.DoTime(true), driver.DoAvgRSS(cmd.RSSFunc())}...)
}
