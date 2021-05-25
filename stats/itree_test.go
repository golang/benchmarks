// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stats

import (
	"reflect"
	"testing"
)

func TestIntervalTree(t *testing.T) {
	t.Parallel()
	inserts := []struct {
		v      float64
		tree   []int
		median float64
	}{
		// These values were pulled from the appendix of the edm paper where the
		// interval tree is described.
		{0.09, []int{1, 1, 0, 1, 0, 0, 0}, 0.25},
		{0.42, []int{2, 2, 0, 1, 1, 0, 0}, 0.125},
		{0.99, []int{3, 2, 1, 1, 1, 0, 1}, 0.25},
		{0.36, []int{4, 3, 1, 1, 2, 0, 1}, 0.375},
	}
	tree := NewIntervalTree(2)
	if l := tree.NumElements(); l != 0 {
		t.Errorf("tree.Length() = %d, expected 0", l)
	}
	if v := tree.Median(); v != 0 {
		t.Errorf("tree.Median() = %f, expected 0", v)
	}
	for i, ins := range inserts {
		tree.Insert(ins.v)
		if !reflect.DeepEqual(tree.vals, ins.tree) {
			t.Errorf("[%d] tree.Insert(%v) = %v, expected %v", i, ins.v, tree.vals, ins.tree)
		}
		if l := tree.NumElements(); l != i+1 {
			t.Errorf("[%d] tree.Length() = %d, expected %d", i, l, i+1)
		}
		if v := tree.Median(); v != ins.median {
			t.Errorf("[%d] tree.Median() = %f, expected %f", i, v, ins.median)
		}
	}
}
