// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.16

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Configuration is a structure that holds all the variables necessary to
// initiate a bent run. These structures are read from a .toml file at
// boot-time.
type Configuration struct {
	Name        string   // Short name used for binary names, mention on command line
	Root        string   // Specific Go root to use for this trial
	BuildFlags  []string // BuildFlags supplied to 'go test -c' for building (e.g., "-p 1")
	AfterBuild  []string // Array of commands to run, output of all commands for a configuration (across binaries) is collected in <runstamp>.<config>.<cmd>
	GcFlags     string   // GcFlags supplied to 'go test -c' for building
	GcEnv       []string // Environment variables supplied to 'go test -c' for building
	RunFlags    []string // Extra flags passed to the test binary
	RunEnv      []string // Extra environment variables passed to the test binary
	RunWrapper  []string // (Outermost) Command and args to precede whatever the operation is; may fail in the sandbox.
	Disabled    bool     // True if this configuration is temporarily disabled
	buildStats  []BenchStat
	benchWriter *os.File
	rootCopy    string // The contents of GOROOT are copied here to allow benchmarking of just the test compilation.
}

func (c *Configuration) buildBenchName() string {
	return c.thingBenchName("build")
}

func (c *Configuration) thingBenchName(suffix string) string {
	if strings.ContainsAny(suffix, "/") {
		suffix = suffix[strings.LastIndex(suffix, "/")+1:]
	}
	return benchDir + "/" + runstamp + "." + c.Name + "." + suffix
}

func (c *Configuration) benchName(b *Benchmark) string {
	return b.Name + "_" + c.Name
}

func (c *Configuration) goCommand() string {
	gocmd := "go"
	if c.Root != "" {
		gocmd = c.Root + "bin/" + gocmd
	}
	return gocmd
}

func (c *Configuration) goCommandCopy() string {
	gocmd := "go"
	if c.rootCopy != "" {
		gocmd = c.rootCopy + "bin/" + gocmd
	}
	return gocmd
}

func (config *Configuration) createFilesForLater() {
	if config.Disabled {
		return
	}
	f, err := os.Create(config.buildBenchName())
	if err != nil {
		fmt.Println("Error creating build benchmark file ", config.buildBenchName(), ", err=", err)
		config.Disabled = true
	} else {
		fmt.Fprintf(f, "goos: %s\n", runtime.GOOS)
		fmt.Fprintf(f, "goarch: %s\n", runtime.GOARCH)
		f.Close() // will be appending later
	}

	for _, cmd := range config.AfterBuild {
		tbn := config.thingBenchName(cmd)
		f, err := os.Create(tbn)
		if err != nil {
			fmt.Printf("Error creating %s benchmark file %s, err=%v\n", cmd, config.thingBenchName(cmd), err)
			continue
		} else {
			f.Close() // will be appending later
		}
	}
}

func (config *Configuration) runOtherBenchmarks(b *Benchmark, cwd string) {
	// Run various other "benchmark" commands on the built binaries, e.g., size, quality of debugging information.
	if config.Disabled {
		return
	}

	for _, cmd := range config.AfterBuild {
		tbn := config.thingBenchName(cmd)
		f, err := os.OpenFile(tbn, os.O_WRONLY|os.O_APPEND, os.ModePerm)
		if err != nil {
			fmt.Printf("There was an error opening %s for append, error %v\n", tbn, err)
			continue
		}

		if !strings.ContainsAny(cmd, "/") {
			cmd = cwd + "/" + cmd
		}
		if b.Disabled {
			continue
		}
		testBinaryName := config.benchName(b)
		c := exec.Command(cmd, testBinDir+"/"+testBinaryName, b.Name)

		c.Env = defaultEnv
		if !b.NotSandboxed {
			c.Env = replaceEnv(c.Env, "GOOS", "linux")
		}
		// Match the build environment here.
		c.Env = replaceEnvs(c.Env, b.GcEnv)
		c.Env = replaceEnvs(c.Env, config.GcEnv)

		if verbose > 0 {
			fmt.Println(asCommandLine(cwd, c))
		}
		output, err := c.CombinedOutput()
		if verbose > 0 || err != nil {
			fmt.Println(string(output))
		} else {
			fmt.Print(".")
		}
		if err != nil {
			fmt.Printf("Error running %s\n", cmd)
			continue
		}
		f.Write(output)
		f.Sync()
		f.Close()
	}
}

func (config *Configuration) compileOne(bench *Benchmark, cwd string, count int) string {
	root := config.rootCopy
	gocmd := config.goCommandCopy()
	gopath := cwd + "/gopath"

	if explicitAll != 1 { // clear cache unless "-a[=1]" which requests -a on compilation.
		cmd := exec.Command(gocmd, "clean", "-cache")
		cmd.Env = defaultEnv
		if !bench.NotSandboxed {
			cmd.Env = replaceEnv(cmd.Env, "GOOS", "linux")
		}
		if root != "" {
			cmd.Env = replaceEnv(cmd.Env, "GOROOT", root)
		}
		cmd.Env = replaceEnvs(cmd.Env, bench.GcEnv)
		cmd.Env = replaceEnvs(cmd.Env, config.GcEnv)
		cmd.Dir = gopath // Only want the cache-cleaning effect, not the binary-deleting effect. It's okay to clean gopath.
		s, _ := config.runBinary("", cmd, true)
		if s != "" {
			fmt.Println("Error running go clean -cache, ", s)
		}
	}

	// Prefix with time for build benchmarking:
	cmd := exec.Command("/usr/bin/time", "-p", gocmd, "test", "-vet=off", "-c")
	cmd.Args = append(cmd.Args, bench.BuildFlags...)
	// Do not normally need -a because cache was emptied first and std was -a installed with these flags.
	// But for -a=1, do it anyway
	if explicitAll == 1 {
		cmd.Args = append(cmd.Args, "-a")
	}
	cmd.Args = append(cmd.Args, config.BuildFlags...)
	if config.GcFlags != "" {
		cmd.Args = append(cmd.Args, "-gcflags="+config.GcFlags)
	}
	cmd.Args = append(cmd.Args, ".")
	cmd.Dir = gopath + "/src/" + bench.Repo
	cmd.Env = defaultEnv
	if !bench.NotSandboxed {
		cmd.Env = replaceEnv(cmd.Env, "GOOS", "linux")
	}
	if root != "" {
		cmd.Env = replaceEnv(cmd.Env, "GOROOT", root)
	}
	cmd.Env = replaceEnvs(cmd.Env, bench.GcEnv)
	cmd.Env = replaceEnvs(cmd.Env, config.GcEnv)

	if verbose > 0 {
		fmt.Println(asCommandLine(cwd, cmd))
	} else {
		fmt.Print(".")
	}

	defer cleanup(gopath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		s := ""
		switch e := err.(type) {
		case *exec.ExitError:
			s = fmt.Sprintf("There was an error running 'go test', output = %s", output)
		default:
			s = fmt.Sprintf("There was an error running 'go test', output = %s, error = %v", output, e)
		}
		fmt.Println(s + "DISABLING benchmark " + bench.Name)
		bench.Disabled = true // if it won't compile, it won't run, either.
		return s + "(" + bench.Name + ")\n"
	}
	soutput := string(output)
	// Capture times from the end of the output.
	rbt := extractTime(soutput, "real")
	ubt := extractTime(soutput, "user")
	sbt := extractTime(soutput, "sys")
	config.buildStats = append(config.buildStats,
		BenchStat{Name: bench.Name, RealTime: rbt, UserTime: ubt, SysTime: sbt})

	// Report and record build stats to testbin

	buf := new(bytes.Buffer)
	configGoArch := getenv(config.GcEnv, "GOARCH")
	if configGoArch != runtime.GOARCH && configGoArch != "" {
		s := fmt.Sprintf("goarch: %s-%s\n", runtime.GOARCH, configGoArch)
		if verbose > 0 {
			fmt.Print(s)
		}
		buf.WriteString(s)
	}
	s := fmt.Sprintf("Benchmark%s 1 %d build-real-ns/op %d build-user-ns/op %d build-sys-ns/op\n",
		strings.Title(bench.Name), rbt, ubt, sbt)
	if verbose > 0 {
		fmt.Print(s)
	}
	buf.WriteString(s)
	f, err := os.OpenFile(config.buildBenchName(), os.O_WRONLY|os.O_APPEND, os.ModePerm)
	if err != nil {
		fmt.Printf("There was an error opening %s for append, error %v\n", config.buildBenchName(), err)
		cleanup(gopath)
		os.Exit(2)
	}
	f.Write(buf.Bytes())
	f.Sync()
	f.Close()

	// Move generated binary to well-known place.
	from := cmd.Dir + "/" + bench.testBinaryName()
	to := testBinDir + "/" + config.benchName(bench)
	err = os.Rename(from, to)
	if err != nil {
		fmt.Printf("There was an error renaming %s to %s, %v\n", from, to, err)
		cleanup(gopath)
		os.Exit(1)
	}
	// Trim /usr/bin/time info from soutput, it's ugly
	if verbose > 0 {
		fmt.Println("mv " + from + " " + to + "")
		i := strings.LastIndex(soutput, "real")
		if i >= 0 {
			soutput = soutput[:i]
		}
		fmt.Print(soutput)
	}

	// Do this here before any cleanup.
	if count == 0 {
		config.runOtherBenchmarks(bench, cwd)
	}

	return ""
}

// runBinary runs cmd and displays the output.
// If the command returns an error, returns an error string.
func (c *Configuration) runBinary(cwd string, cmd *exec.Cmd, printWorkingDot bool) (string, int) {
	line := asCommandLine(cwd, cmd)
	if verbose > 0 {
		fmt.Println(line)
	} else {
		if printWorkingDot {
			fmt.Print(".")
		}
	}

	rc := 0

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Sprintf("Error [stdoutpipe] running '%s', %v", line, err), rc
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Sprintf("Error [stderrpipe] running '%s', %v", line, err), rc
	}
	err = cmd.Start()
	if err != nil {
		return fmt.Sprintf("Error [command start] running '%s', %v", line, err), rc
	}

	var mu = &sync.Mutex{}

	f := func(r *bufio.Reader, done chan error) {
		for {
			bytes, err := r.ReadBytes('\n')
			n := len(bytes)
			if n > 0 {
				mu.Lock()
				nw, err := c.benchWriter.Write(bytes[0:n])
				if err != nil {
					fmt.Printf("Error writing, err = %v, nwritten = %d, nrequested = %d\n", err, nw, n)
				}
				c.benchWriter.Sync()
				fmt.Print(string(bytes[0:n]))
				mu.Unlock()
			}
			if err == io.EOF || n == 0 {
				break
			}
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}

	doneS := make(chan error)
	doneE := make(chan error)

	go f(bufio.NewReader(stdout), doneS)
	go f(bufio.NewReader(stderr), doneE)

	errS := <-doneS
	errE := <-doneE

	err = cmd.Wait()
	rc = cmd.ProcessState.ExitCode()

	if err != nil {
		switch e := err.(type) {
		case *exec.ExitError:
			return fmt.Sprintf("Error running '%s', stderr = %s, rc = %d", line, e.Stderr, rc), rc
		default:
			return fmt.Sprintf("Error running '%s', %v, rc = %d", line, e, rc), rc

		}
	}
	if errS != nil {
		return fmt.Sprintf("Error [read stdout] running '%s', %v, rc = %d", line, errS, rc), rc
	}
	if errE != nil {
		return fmt.Sprintf("Error [read stderr] running '%s', %v, rc = %d", line, errE, rc), rc
	}
	return "", rc
}

// extractTime extracts a time (from /usr/bin/time -p) based on the tag
// and returns the time converted to nanoseconds.  Missing times and bad
// data result in NaN.
func extractTime(output, label string) int64 {
	// find tag in first column
	li := strings.LastIndex(output, label)
	if li < 0 {
		return -1
	}
	output = output[li+len(label):]
	// lose intervening white space
	li = strings.IndexAny(output, "0123456789-.eEdD")
	if li < 0 {
		return -1
	}
	output = output[li:]
	li = strings.IndexAny(output, "\n\r\t ")
	if li >= 0 { // failing to find EOL is a special case of done.
		output = output[:li]
	}
	x, err := strconv.ParseFloat(output, 64)
	if err != nil {
		return -1
	}
	return int64(x * 1000 * 1000 * 1000)
}
