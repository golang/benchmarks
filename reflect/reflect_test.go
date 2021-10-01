// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflect_test

import (
	"fmt"
	"reflect"
	"testing"
)

// BenchmarkMap is a copy of reflect_test.BenchmarkMap from the Go standard
// library, placed here for basic benchmarking testing.
func BenchmarkMap(b *testing.B) {
	type V *int
	value := reflect.ValueOf((V)(nil))
	stringKeys := []string{}
	mapOfStrings := map[string]V{}
	uint64Keys := []uint64{}
	mapOfUint64s := map[uint64]V{}
	for i := 0; i < 100; i++ {
		stringKey := fmt.Sprintf("key%d", i)
		stringKeys = append(stringKeys, stringKey)
		mapOfStrings[stringKey] = nil

		uint64Key := uint64(i)
		uint64Keys = append(uint64Keys, uint64Key)
		mapOfUint64s[uint64Key] = nil
	}

	tests := []struct {
		label          string
		m, keys, value reflect.Value
	}{
		{"StringKeys", reflect.ValueOf(mapOfStrings), reflect.ValueOf(stringKeys), value},
		{"Uint64Keys", reflect.ValueOf(mapOfUint64s), reflect.ValueOf(uint64Keys), value},
	}

	for _, tt := range tests {
		b.Run(tt.label, func(b *testing.B) {
			b.Run("MapIndex", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for j := tt.keys.Len() - 1; j >= 0; j-- {
						tt.m.MapIndex(tt.keys.Index(j))
					}
				}
			})
			b.Run("SetMapIndex", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for j := tt.keys.Len() - 1; j >= 0; j-- {
						tt.m.SetMapIndex(tt.keys.Index(j), tt.value)
					}
				}
			})
		})
	}
}
