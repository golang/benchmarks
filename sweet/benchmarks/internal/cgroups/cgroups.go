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
)

type Cmd struct {
	exec.Cmd
	modified bool
	path     string
	scope    string
}

func WrapCommand(cmd *exec.Cmd, scope string) (*Cmd, error) {
	wrapped := Cmd{Cmd: *cmd}

	// TODO(mknyszek): Maybe make a more stringent check?
	systemdRunPath, err := exec.LookPath("systemd-run")
	if errors.Is(err, exec.ErrNotFound) {
		fmt.Fprintln(os.Stderr, "# warning: systemd-run not available, skipping...")
		return &wrapped, nil
	} else if err != nil {
		return nil, err
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
