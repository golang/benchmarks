// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stats

import (
	"container/heap"
	"math"
)

type maxHeap []float64

func (h maxHeap) Len() int           { return len(h) }
func (h maxHeap) Less(i, j int) bool { return h[i] > h[j] }
func (h maxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *maxHeap) Push(x interface{}) {
	*h = append(*h, x.(float64))
}
func (h *maxHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type minHeap []float64

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x interface{}) {
	*h = append(*h, x.(float64))
}
func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// addToHeaps adds a value to the appropriate heap,and keeps their sizes close.
// This is a direct translation of the addToHeaps function described in the
// paper.
func addToHeaps(min *minHeap, max *maxHeap, x float64) {
	// NB: There's an ambiguity in the paper here related to if-then/else
	// evaluation. This structure seems to yield the desired results.
	if min.Len() == 0 || x < (*min)[0] {
		heap.Push(max, x)
	} else {
		heap.Push(min, x)
	}
	if min.Len() > max.Len()+1 {
		heap.Push(max, heap.Pop(min))
	} else if max.Len() > min.Len()+1 {
		heap.Push(min, heap.Pop(max))
	}
}

// getMedian finds the median of the min and max heaps.
// This is a direct translation of the getMedian function described in the
// paper.
func getMedian(min minHeap, max maxHeap) float64 {
	if min.Len() > max.Len() {
		return min[0]
	}
	if max.Len() > min.Len() {
		return max[0]
	}
	return (max[0] + min[0]) / 2
}

// edmx implements the EDM-X algorithm.
// This algorithm runs in place, modifying the data.
func edmx(input []float64, delta int) int {
	input = normalize(input)
	var lmax, rmax maxHeap
	var lmin, rmin minHeap
	heap.Init(&lmax)
	heap.Init(&rmax)
	heap.Init(&lmin)
	heap.Init(&rmin)
	bestStat := math.Inf(-1)
	bestLoc := -1

	for i := 0; i < delta-1; i++ {
		addToHeaps(&lmin, &lmax, input[i])
	}
	for i := delta; i < len(input)-delta+1; i++ {
		addToHeaps(&lmin, &lmax, input[i-1])
		ml := getMedian(lmin, lmax)
		rmax, rmin = rmax[:0], rmin[:0]
		for j := i; j < i+delta-1; j++ {
			addToHeaps(&rmin, &rmax, input[j])
		}
		for j := i + delta; j < len(input)+1; j++ {
			addToHeaps(&rmin, &rmax, input[j-1])
			mr := getMedian(rmin, rmax)
			stat := float64(i*(j-i)) / float64(j) * (ml - mr) * (ml - mr)
			if stat > bestStat {
				bestStat = stat
				bestLoc = i
			}
		}
	}

	return bestLoc
}

// EDMX runs the EDM-X algorithm on a slice of floats.
func EDMX(input []float64, delta int) int {
	// edmx modfies the input, don't do that.
	c := make([]float64, len(input))
	copy(c, input)
	return edmx(c, delta)
}

// EMDXInt runs the EDM-X algorithm on a slice of integer datapoints.
func EDMXInt(input []int, delta int) int {
	return edmx(toFloat(input), delta)
}
