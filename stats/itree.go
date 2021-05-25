// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stats

import (
	"math"
)

// IntervalTree is a structure used to make a calculation of running medians quick.
// The structure is described in the Section 8/Appendix of the paper.
type IntervalTree struct {
	d    int
	vals []int
}

// NewIntervalTree creates a new IntervalTree of depth d.
func NewIntervalTree(d int) *IntervalTree {
	if d < 0 {
		panic("invalid depth")
	}
	return &IntervalTree{
		d:    d,
		vals: make([]int, (1<<(d+1))-1),
	}
}

// walk is a generic function on interval trees to add or remove elements.
func (it *IntervalTree) walk(v float64, update int) {
	v = math.Abs(v)
	mid, inc := 0.5, 0.25
	idx := 0
	// Update the levels in the tree.
	for i := 0; i <= it.d; i++ {
		it.vals[idx] += update
		idx = idx*2 + 1
		if v > mid {
			mid += inc
			idx += 1
		} else {
			mid -= inc
		}
		inc /= 2.
	}
}

// Insert puts an element in an interval tree.
func (it *IntervalTree) Insert(v float64) {
	it.walk(v, 1)
}

// Remove removes an element from an interview tree.
func (it *IntervalTree) Remove(v float64) {
	it.walk(v, -1)
}

// Median returns the current median.
func (it *IntervalTree) Median() float64 {
	// If empty, special case and return 0.
	numElements := it.NumElements()
	if numElements == 0 {
		return 0
	}

	l, u := 0., 1.
	k := int(math.Ceil(float64(numElements) / 2.))
	for i := 0; i < len(it.vals); {
		j := 2*i + 1
		if j >= len(it.vals) {
			break
		}
		if it.vals[i] == k {
			kf := float64(k)
			a, b := float64(it.vals[j])/kf, float64(it.vals[j+1])/kf
			x := (l + (l+u)/2.) / 2.
			y := (u + (l+u)/2.) / 2.
			return (a*x + b*y) / (a + b)
		}
		if v := it.vals[j]; v >= k {
			i = j
			u = (l + u) / 2.
		} else {
			k -= v
			i = j + 1
			l = (l + u) / 2.
		}
	}
	return (u-l)/2. + l
}

// NumElements returns the number of elements in the tree.
func (it *IntervalTree) NumElements() int {
	return it.vals[0]
}
