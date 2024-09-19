// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	shellquote "github.com/kballard/go-shellquote"
)

var (
	cmdLog, actLog *log.Logger
	cmdOn, actOn   = false, false
	envMap         map[string]string
)

func init() {
	cmdLog = log.New(os.Stdout, "[shell] ", 0)
	actLog = log.New(os.Stderr, "[sweet] ", 0)
	envMap = makeEnvironMap()
}

func makeEnvironMap() map[string]string {
	env := os.Environ()
	envmap := make(map[string]string)
	for _, e := range env {
		s := strings.SplitN(e, "=", 2)
		if len(s) != 2 {
			continue
		}
		envmap[s[0]] = s[1]
	}
	return envmap
}

func SetCommandTrace(on bool) {
	cmdOn = on
}

func SetActivityLog(on bool) {
	actOn = on
}

func filterAndQuoteEnviron(env []string) []string {
	fenv := make([]string, 0, len(env))
	for _, e := range env {
		s := strings.SplitN(e, "=", 2)
		if len(s) != 2 {
			continue
		}
		if v, ok := envMap[s[0]]; ok && v == s[1] {
			continue
		}
		fenv = append(fenv, fmt.Sprintf("%s=%s", s[0], shellquote.Join(s[1])))
	}
	return fenv
}

func TraceCommand(cmd *exec.Cmd, background bool) {
	if !cmdOn {
		return
	}
	senv := ""
	if len(cmd.Env) != 0 {
		senv = strings.Join(filterAndQuoteEnviron(cmd.Env), " ")
	}
	if cmd.Dir != "" {
		cmdLog.Printf("pushd %s", cmd.Dir)
	}
	sbg := ""
	if background {
		sbg = " &"
	}
	sarg := shellquote.Join(cmd.Args...)
	if senv != "" {
		cmdLog.Printf("%s %s%s", senv, sarg, sbg)
	} else {
		cmdLog.Printf("%s%s", sarg, sbg)
	}
	if cmd.Dir != "" {
		cmdLog.Printf("popd")
	}
}

func TraceKill(cmd *exec.Cmd) {
	if !cmdOn {
		return
	}
	cmdLog.Printf("killall -SIGINT %s", filepath.Base(cmd.Path))
}

func CommandPrintf(format string, args ...interface{}) {
	if !cmdOn {
		return
	}
	cmdLog.Printf(format, args...)
}

func Printf(format string, args ...interface{}) {
	if !actOn {
		return
	}
	actLog.Printf(format, args...)
}

func Print(args ...interface{}) {
	if !actOn {
		return
	}
	actLog.Print(args...)
}

func Error(err error) {
	actLog.Printf("error: %v", err)
	if e, ok := err.(*exec.ExitError); ok {
		actLog.Printf("output:\n%s", string(e.Stderr))
	}
}
