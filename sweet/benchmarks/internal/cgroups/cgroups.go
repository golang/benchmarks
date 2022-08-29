// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cgroups

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func findSystemdRun() (string, error) {
	bin, err := exec.LookPath("systemd-run")
	if errors.Is(err, exec.ErrNotFound) {
		return "", fmt.Errorf("systemd-run binary not found")
	} else if err != nil {
		return "", fmt.Errorf("error looking for systemd-run: %w", err)
	}

	scope := fmt.Sprintf("systemd-run-test-%d.scope", time.Now().UnixNano())

	cmd := exec.Command(bin, "--user", "--scope", "--unit", scope, "/bin/true")
	sout, serr := cmd.CombinedOutput()
	if serr == nil {
		// It works!
		return bin, nil
	}

	var context strings.Builder
	fmt.Fprintf(&context, "\noutput: %s", string(sout))

	// Failed. systemd-run probably just said to look at journalctl;
	// collect that additional context.
	cmd = exec.Command("journalctl", "--catalog", "--user", "--unit", scope)
	jout, jerr := cmd.CombinedOutput()
	if jerr != nil {
		fmt.Fprintf(&context, "\njournalctl error: %v\noutout: %s", jerr, string(jout))
	} else {
		fmt.Fprintf(&context, "\njournalctl output: %s", string(jout))
	}

	// Attempt to cleanup unit.
	cmd = exec.Command("systemctl", "--user", "reset-failed", scope)
	scout, scerr := cmd.CombinedOutput()
	if scerr != nil {
		fmt.Fprintf(&context, "\nsystemctl cleanup error: %v\noutput: %s", scerr, string(scout))
	}

	return "", fmt.Errorf("system-run failed: %w%s", serr, context.String())
}

var (
	systemdOnce     sync.Once
	systemdRunPath  string
	systemdRunError error
)

type Cmd struct {
	exec.Cmd
	modified bool
	path     string
	scope    string
}

func WrapCommand(cmd *exec.Cmd, scope string) (*Cmd, error) {
	wrapped := Cmd{Cmd: *cmd}

	systemdOnce.Do(func() {
		systemdRunPath, systemdRunError = findSystemdRun()
	})

	if systemdRunError != nil {
		fmt.Fprintf(os.Stderr, "# warning: systemd-run not available: %v\n# skipping cgroup wrapper...\n", systemdRunError)
		return &wrapped, nil
	}

	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	wrapped.Cmd.Args = append([]string{
		systemdRunPath, "--user", "--scope", "--unit=" + scope,
	}, wrapped.Cmd.Args...)
	wrapped.Cmd.Path = systemdRunPath
	wrapped.modified = true
	wrapped.path = fmt.Sprintf("user-%s.slice/user@%s.service/app.slice", u.Uid, u.Uid)
	wrapped.scope = scope

	return &wrapped, nil
}

func (c *Cmd) RSSFunc() func() (uint64, error) {
	if !c.modified {
		return nil
	}
	memPath := filepath.Join("/sys/fs/cgroup/user.slice", c.path, c.scope, "memory.current")
	return func() (uint64, error) {
		data, err := os.ReadFile(memPath)
		if err != nil {
			return 0, err
		}
		return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	}
}

