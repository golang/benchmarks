// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package par

import (
	"reflect"
	"sync"
)

type Funcs struct {
	funcs []func()
}

func (f *Funcs) Add(fn any) {
	if fn, ok := fn.(func()); ok {
		// Easy
		f.funcs = append(f.funcs, fn)
		return
	}

	rv := reflect.ValueOf(fn)
	if rv.Kind() != reflect.Func || rv.Type().NumIn() != 0 {
		panic("fn must be a function with zero arguments")
	}
	f.funcs = append(f.funcs, func() { rv.Call(nil) })
}

func (f *Funcs) Run() {
	var wg sync.WaitGroup
	for _, fn := range f.funcs {
		wg.Add(1)
		go func(fn func()) {
			defer wg.Done()
			fn()
		}(fn)
	}
	f.funcs = nil
	wg.Wait()
}
