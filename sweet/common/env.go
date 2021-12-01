// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import (
	"fmt"
	"os"
	"strings"
)

type Env struct {
	parent *Env
	data   map[string]string
}

func varsToMap(vars ...string) (map[string]string, error) {
	env := make(map[string]string)
	for _, v := range vars {
		s := strings.SplitN(v, "=", 2)
		if len(s) != 2 {
			return nil, fmt.Errorf("%q is not a valid environment variable", v)
		}
		env[s[0]] = s[1]
	}
	return env, nil
}

func NewEnvFromEnviron() *Env {
	env, err := NewEnv(os.Environ()...)
	if err != nil {
		panic(err)
	}
	return env
}

func NewEnv(vars ...string) (*Env, error) {
	m, err := varsToMap(vars...)
	if err != nil {
		return nil, err
	}
	return &Env{data: m}, nil
}

func (e *Env) Set(vars ...string) (*Env, error) {
	m, err := varsToMap(vars...)
	if err != nil {
		return nil, err
	}
	return &Env{
		data:   m,
		parent: e,
	}, nil
}

func (e *Env) MustSet(vars ...string) *Env {
	env, err := e.Set(vars...)
	if err != nil {
		panic(err)
	}
	return env
}

func (e *Env) Lookup(name string) (string, bool) {
	t := e
	for t != nil {
		if v, ok := t.data[name]; ok {
			return v, true
		}
		t = t.parent
	}
	return "", false
}

func (e *Env) Prefix(name, prefix string) *Env {
	var (
		n   *Env
		err error
	)
	if v, ok := e.Lookup(name); ok {
		n, err = e.Set(fmt.Sprintf("%s=%s%s", name, prefix, v))
	} else {
		n, err = e.Set(fmt.Sprintf("%s=%s", name, prefix))
	}
	// If we actually get an error out of Set here, then
	// something went very wrong. Panic.
	if err != nil {
		panic(err.Error())
	}
	return n
}

func (e *Env) Collapse() []string {
	t := e
	c := make(map[string]string)
	for t != nil {
		for k, v := range t.data {
			if _, ok := c[k]; !ok {
				c[k] = v
			}
		}
		t = t.parent
	}
	env := make([]string, 0, len(c))
	for k, v := range c {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}
