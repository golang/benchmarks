// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stats

import (
	"math"
)

// edm carries data to calculate e-divisive with medians.
type edm struct {
	z           []float64
	delta       int
	bestStat    float64
	bestIdx     int
	tau         int
	ta, tb, tab *IntervalTree
}

// normalize normalizes a slice of floats in place.
func normalize(input []float64) []float64 {
	// gracefully handle empty inputs.
	if len(input) == 0 {
		return input
	}
	min, max := input[0], input[0]
	for _, v := range input {
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}
	for i, v := range input {
		input[i] = (v - min) / (max - min)
	}
	return input
}

// medianResolution finds a good compromise for the approximate median, as
// described in the paper.
func medianResolution(l int) int {
	d := 10 // min resolution 1/(1<<d).
	if l := int(math.Ceil(math.Log2(float64(l)))); l > d {
		d = l
	}
	return d
}

// toFloat converts an slice of integers to float64.
func toFloat(input []int) []float64 {
	output := make([]float64, len(input))
	for i, v := range input {
		output[i] = float64(v)
	}
	return output
}

// stat calculates the edx stat for a given location, saving it if it's better
// than the currently stored statistic.
func (e *edm) stat(tau2 int) float64 {
	a, b, c := e.ta.Median(), e.tb.Median(), e.tab.Median()
	a, b, c = a*a, b*b, c*c
	stat := 2*c - a - b
	stat *= float64(e.tau*(tau2-e.tau)) / float64(tau2)
	if stat > e.bestStat {
		e.bestStat = stat
		e.bestIdx = e.tau
	}
	return stat
}

// calc handles the brunt of the E-divisive with median algorithm described in:
// https://courses.cit.cornell.edu/nj89/docs/edm.pdf.
func (e *edm) calc() int {
	normalize(e.z)

	e.bestStat = math.Inf(-1)
	e.tau = e.delta
	tau2 := 2 * e.delta

	d := medianResolution(len(e.z))
	e.ta = NewIntervalTree(d)
	e.tb = NewIntervalTree(d)
	e.tab = NewIntervalTree(d)

	for i := 0; i < e.tau; i++ {
		for j := i + 1; j < e.tau; j++ {
			e.ta.Insert(e.z[i] - e.z[j])
		}
	}

	for i := e.tau; i < tau2; i++ {
		for j := i + 1; j < tau2; j++ {
			e.tb.Insert(e.z[i] - e.z[j])
		}
	}

	for i := 0; i < e.tau; i++ {
		for j := e.tau; j < tau2; j++ {
			e.tab.Insert(e.z[i] - e.z[j])
		}
	}

	tau2 += 1
	for ; tau2 < len(e.z)+1; tau2++ {
		e.tb.Insert(e.z[tau2-1] - e.z[tau2-2])
		e.stat(tau2)
	}

	forward := false
	for e.tau < len(e.z)-e.delta {
		if forward {
			e.forwardUpdate()
		} else {
			e.backwardUpdate()
		}
		forward = !forward
	}

	return e.bestIdx
}

func (e *edm) forwardUpdate() {
	tau2 := e.tau + e.delta
	e.tau += 1

	for i := e.tau - e.delta; i < e.tau-1; i++ {
		e.ta.Insert(e.z[i] - e.z[e.tau-1])
	}
	for i := e.tau - e.delta; i < e.tau; i++ {
		e.ta.Remove(e.z[i] - e.z[e.tau-e.delta-1])
	}
	e.ta.Insert(e.z[e.tau-e.delta-1] - e.z[e.tau-e.delta])

	e.tab.Remove(e.z[e.tau-1] - e.z[e.tau-e.delta-1])
	for i := e.tau; i < tau2; i++ {
		e.tab.Remove(e.z[i] - e.z[e.tau-e.delta-1])
		e.tab.Insert(e.z[i] - e.z[e.tau-1])
	}
	for i := e.tau - e.delta; i < e.tau-1; i++ {
		e.tab.Remove(e.z[i] - e.z[e.tau-1])
		e.tab.Insert(e.z[i] - e.z[tau2])
	}
	e.tab.Insert(e.z[e.tau-1] - e.z[tau2])

	for i := e.tau; i < tau2; i++ {
		e.tb.Remove(e.z[i] - e.z[e.tau-1])
		e.tb.Insert(e.z[i] - e.z[tau2])
	}

	tau2 += 1
	for ; tau2 < len(e.z)+1; tau2++ {
		e.tb.Insert(e.z[tau2-1] - e.z[tau2-2])
		e.stat(tau2)
	}
}

func (e *edm) backwardUpdate() {
	tau2 := e.tau + e.delta
	e.tau += 1

	for i := e.tau - e.delta; i < e.tau-1; i++ {
		e.ta.Insert(e.z[i] - e.z[e.tau-1])
	}
	for i := e.tau - e.delta; i < e.tau; i++ {
		e.ta.Remove(e.z[i] - e.z[e.tau-e.delta-1])
	}
	e.ta.Insert(e.z[e.tau-e.delta-1] - e.z[e.tau-e.delta])

	e.tab.Remove(e.z[e.tau-1] - e.z[e.tau-e.delta-1])
	for i := e.tau; i < tau2; i++ {
		e.tab.Remove(e.z[i] - e.z[e.tau-e.delta-1])
		e.tab.Insert(e.z[i] - e.z[e.tau-1])
	}
	for i := e.tau - e.delta; i < e.tau-1; i++ {
		e.tab.Remove(e.z[i] - e.z[e.tau-1])
		e.tab.Insert(e.z[i] - e.z[tau2])
	}
	e.tab.Insert(e.z[e.tau-1] - e.z[tau2])

	for i := e.tau; i < e.tau+e.delta-1; i++ {
		e.tb.Insert(e.z[e.tau+e.delta-1] - e.z[i])
		e.tb.Remove(e.z[i] - e.z[e.tau-1])
	}

	for tau2 = len(e.z); tau2 >= e.tau+e.delta; tau2-- {
		e.tb.Remove(e.z[tau2-1] - e.z[tau2-2])
		e.stat(tau2)
	}
}

// EDM performs the approximate E-divisive with means calculation as described
// in https://courses.cit.cornell.edu/nj89/docs/edm.pdf.
//
// EDM is an algorithm for finding breakout points in a time-series data set.
// The function accepts the input vector, and a window width that describes the
// search window during which the median breakout must occur. The window, ∂,
// is described in the paper.
//
// Note this algorithm uses the interval tree median approach as described in
// the paper. The medians used will have a resolution of 1/(2^d), where d is
// max(log2(len(input)), 10). That is the value recommended in the paper.
func EDM(input []float64, delta int) int {
	// edm modifies the input, we don't want to do that to our callers.
	c := make([]float64, len(input))
	copy(c, input)
	e := &edm{z: c, delta: delta}
	return e.calc()
}

// EDMInt is the integer version of EDM.
func EDMInt(input []int, delta int) int {
	e := &edm{z: toFloat(input), delta: delta}
	return e.calc()
}
