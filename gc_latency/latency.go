// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/trace"
	"sort"
	"testing"
	"time"
	"unsafe"
)

const (
	bufferLen   = 200_000
	warmupCount = 1_000_000
	runCount    = 5_000_000
)

type kbyte []byte
type circularBuffer [bufferLen]kbyte

type LB struct {
	// Performance measurement stuff
	delays []time.Duration // delays observed (for distribution)
	worst  time.Duration   // worst delay observed

	// For making sense of the bad outcome.
	total        time.Duration // total time spent in allocations
	allStart     time.Time     // time (very nearly) at which the trace begins
	worstIndex   int           // index of worst case allocation delay
	worstElapsed time.Duration // duration of worst case allocation delay

	sink *circularBuffer // assign a pointer here to ensure heap allocation

	// How to allocate

	// "Fluff" refers to allocating a small fraction of extra quickly-dead objects
	// to break up long runs on not-free objects that were once a cause of allocation latency.
	doFluff bool
	// "Fluff" allocations are all assigned to fluff, so that they are on-heap, but only the last one is live.
	fluff kbyte

	// The circular buffer can be on the heap, in a global, or on stack.
	// This choice affects allocation latency.
	howAllocated string
}

// globalBuffer is the globally-allocated circular buffer,
// for measuring the cost of scanning large global objects.
var globalBuffer circularBuffer

// These three methods pass a differently-allocated circularBuffer
// to the benchmarked "work" to see how that affects allocation tail latency.

//go:noinline
func (lb *LB) global(count int) {
	lb.work(&globalBuffer, count)
	for i := range globalBuffer {
		globalBuffer[i] = nil
	}
}

//go:noinline
func (lb *LB) heap(count int) {
	c := &circularBuffer{}
	lb.sink = c // force to heap
	lb.work(c, count)
	lb.sink = nil
}

//go:noinline
func (lb *LB) stack(count int) {
	var c circularBuffer
	lb.work(&c, count)
}

// newSlice allocates a 1k slice of bytes and initializes them all to byte(n)
func (lb *LB) newSlice(n int) kbyte {
	m := make(kbyte, 1024)
	if lb.doFluff && n&63 == 0 {
		lb.fluff = make(kbyte, 1024)
	}
	for i := range m {
		m[i] = byte(n)
	}
	return m
}

// storeSlice stores a newly allocated 1k slice of bytes at c[count%len(c)]
// It also checks the time needed to do this and records the worst case.
func (lb *LB) storeSlice(c *circularBuffer, count int) {
	start := time.Now()
	c[count%len(c)] = lb.newSlice(count)
	elapsed := time.Since(start)

	candElapsed := time.Since(lb.allStart) // Record location of worst in trace
	if elapsed > lb.worst {
		lb.worst = elapsed
		lb.worstIndex = count
		lb.worstElapsed = candElapsed
	}
	lb.total = time.Duration(lb.total.Nanoseconds() + elapsed.Nanoseconds())
	lb.delays = append(lb.delays, elapsed)
}

//go:noinline
func (lb *LB) work(c *circularBuffer, count int) {
	for i := 0; i < count; i++ {
		lb.storeSlice(c, i)
	}
}

func (lb *LB) doAllocations(count int) {
	switch lb.howAllocated {
	case "stack":
		lb.stack(count)
	case "heap":
		lb.heap(count)
	case "global":
		lb.global(count)
	}
}

var traceFile string

func flags() (string, bool) {
	var howAllocated = "stack"
	var doFluff bool
	flag.StringVar(&traceFile, "trace", traceFile, "name of trace file to create")
	flag.StringVar(&howAllocated, "how", howAllocated, "how the buffer is allocated = {stack,heap,global}")
	flag.BoolVar(&doFluff, "fluff", doFluff, "insert 'fluff' into allocation runs to break up sweeps")

	flag.Parse()

	switch howAllocated {
	case "stack", "heap", "global":
		break
	default:
		fmt.Fprintf(os.Stderr, "-how needs to be one of 'heap', 'stack' or 'global', saw '%s' instead\n", howAllocated)
		os.Exit(1)
	}
	return howAllocated, doFluff
}

var reportWorstFlag bool

func (lb0 *LB) bench(b *testing.B) {
	if b != nil {
		b.StopTimer()
	}

	var c *circularBuffer = &circularBuffer{}
	lb0.sink = c // force heap allocation
	lb0.delays = make([]time.Duration, 0, runCount)
	// Warm up heap, virtual memory, address space, etc.
	lb0.work(c, warmupCount)
	c, lb0.sink = nil, nil
	runtime.GC() // Start fresh, GC with all the timers turned off.

	lb := &LB{doFluff: lb0.doFluff, howAllocated: lb0.howAllocated, delays: lb0.delays[:0]}
	count := runCount

	// Confine tracing and timing defers to a small block.
	run := func() {
		if traceFile != "" {
			f, err := os.Create(traceFile)
			if err != nil {
				if b != nil {
					b.Fatalf("Could not create trace file '%s'\n", traceFile)
				} else {
					fmt.Fprintf(os.Stderr, "Could not create trace file '%s'\n", traceFile)
					os.Exit(1)
				}
			}
			defer f.Close()
			trace.Start(f)
			defer trace.Stop()
		}
		lb.allStart = time.Now() // this is for trace file navigation, not benchmark timing.

		if b != nil {
			count = b.N * count
			if b.N > 1 {
				lb.delays = make([]time.Duration, 0, count)
			}
			b.StartTimer()
			defer b.StopTimer()
		}
		lb.doAllocations(count)
	}
	run()

	sort.Slice(lb.delays, func(i, j int) bool { return lb.delays[i] < lb.delays[j] })
	delays := lb.delays
	delayLen := float64(len(delays))
	average, median := time.Duration(lb.total.Nanoseconds()/int64(count)), delays[len(delays)/2]
	p29, p39, p49, p59, p69 := lb.delays[int(0.99*delayLen)], delays[int(0.999*delayLen)], delays[int(0.9999*delayLen)], delays[int(0.99999*delayLen)], delays[int(0.999999*delayLen)]
	if b != nil {
		if testing.Short() {
			b.ReportMetric(0, "ns/op")
			b.ReportMetric(float64(p59), "p99.999-ns")
			b.ReportMetric(float64(p69), "p99.9999-ns")
		} else {
			b.ReportMetric(float64(average.Nanoseconds()), "ns/op")
			b.ReportMetric(float64(median), "p50-ns")
			b.ReportMetric(float64(p29), "p99-ns")
			b.ReportMetric(float64(p39), "p99.9-ns")
			b.ReportMetric(float64(p49), "p99.99-ns")
			b.ReportMetric(float64(p59), "p99.999-ns")
			b.ReportMetric(float64(p69), "p99.9999-ns")
		}
		if reportWorstFlag {
			b.ReportMetric(float64(lb.worst), "worst")
		}
		// Don't report worst case, it is ultra-noisy.
	} else {
		fmt.Printf("GC latency benchmark, how=%s, fluff=%v\n", lb.howAllocated, lb.doFluff)
		fmt.Println("Worst allocation latency:", lb.worst)
		fmt.Println("Worst allocation index:", lb.worstIndex)
		fmt.Println("Worst allocation occurs at run elapsed time:", lb.worstElapsed)
		fmt.Println("Average allocation latency:", average)
		fmt.Println("Median allocation latency:", median)
		fmt.Println("99% allocation latency:", p29)
		fmt.Println("99.9% allocation latency:", p39)
		fmt.Println("99.99% allocation latency:", p49)
		fmt.Println("99.999% allocation latency:", p59)
		fmt.Println("99.9999% allocation latency:", p69)
		fmt.Println("Sizeof(circularBuffer) =", unsafe.Sizeof(*c))
		fmt.Println("Approximate live memory =", unsafe.Sizeof(*c)+bufferLen*1024)
	}
}
