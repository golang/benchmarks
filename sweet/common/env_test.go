// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common_test

import (
	"reflect"
	"testing"

	"golang.org/x/benchmarks/sweet/common"
)

func stringSliceToSet(sl []string) map[string]struct{} {
	ss := make(map[string]struct{})
	for _, s := range sl {
		ss[s] = struct{}{}
	}
	return ss
}

func TestEnv(t *testing.T) {
	tryLookup := func(t *testing.T, env *common.Env, try, expect string) {
		if v, ok := env.Lookup(try); !ok {
			t.Fatalf("expected to find variable %q", try)
		} else if v != expect {
			t.Fatalf("expected to find value %q for %q, instead got %q", v, try, expect)
		}
	}
	tryBadLookup := func(t *testing.T, env *common.Env, try string) {
		if v, ok := env.Lookup(try); ok {
			t.Fatalf("expected to not find variable %q, got %q", try, v)
		}
	}
	tryCreate := func(t *testing.T, args ...string) *common.Env {
		env, err := common.NewEnv(args...)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		return env
	}
	trySet := func(t *testing.T, env *common.Env, args ...string) *common.Env {
		env2, err := env.Set(args...)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		return env2
	}

	env := tryCreate(t, "MYVAR=2", "MYVAR2=100")

	t.Run("BadCreate", func(t *testing.T) {
		_, err := common.NewEnv("MYVAR", "MYVAR2=100")
		if err == nil {
			t.Fatal("expected error due to bad input")
		}
	})
	t.Run("Lookup", func(t *testing.T) {
		tryLookup(t, env, "MYVAR", "2")
	})
	t.Run("EmptyLookup", func(t *testing.T) {
		tryBadLookup(t, env, "NOVAR")
	})
	t.Run("BadSet", func(t *testing.T) {
		_, err := env.Set("BADVAR")
		if err == nil {
			t.Fatal("expected error due to bad input")
		}
	})
	exp := stringSliceToSet([]string{"MYVAR=3", "OTHERVAR=6", "MYVAR2=100"})
	t.Run("Set", func(t *testing.T) {
		env2 := trySet(t, env, "MYVAR=3", "OTHERVAR=6")
		tryLookup(t, env2, "MYVAR", "3")
		tryLookup(t, env2, "OTHERVAR", "6")
		tryLookup(t, env2, "MYVAR2", "100")
		tryLookup(t, env, "MYVAR", "2")
		tryBadLookup(t, env, "OTHERVAR")
		l := stringSliceToSet(env2.Collapse())
		if !reflect.DeepEqual(l, exp) {
			t.Fatalf("on collapse got %v, expected %v", l, exp)
		}
	})
	t.Run("DeepSet", func(t *testing.T) {
		env2 := trySet(t, env, "OTHERVAR=6")
		env3 := trySet(t, env2, "MYVAR=3")
		tryLookup(t, env3, "MYVAR", "3")
		tryLookup(t, env3, "MYVAR2", "100")
		tryLookup(t, env3, "OTHERVAR", "6")
		tryLookup(t, env2, "OTHERVAR", "6")
		tryLookup(t, env2, "MYVAR2", "100")
		tryLookup(t, env2, "MYVAR", "2")
		tryLookup(t, env, "MYVAR", "2")
		tryBadLookup(t, env, "OTHERVAR")
		l := stringSliceToSet(env3.Collapse())
		if !reflect.DeepEqual(l, exp) {
			t.Fatalf("on collapse got %v, expected %v", l, exp)
		}
	})
	t.Run("Prefix", func(t *testing.T) {
		env2 := env.Prefix("MYVAR", "3")
		tryLookup(t, env2, "MYVAR", "32")
		tryLookup(t, env, "MYVAR", "2")
	})
}
