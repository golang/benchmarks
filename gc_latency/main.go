// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// Gc_latency is a modified version of a program that tickled multiple
// latency glitches in the Go GC/runtime.  This version reports the time
// of the worst observed glitches so that they can be easily located in
// a trace file and debugged.  This program can also be run as a benchmark
// to allow easier automated performance monitoring; the benchmark doesn't
// report worst case times because those are too noisy.
//
// Usage:
//
//	 gc_latency [flags]
//
//	 Flags (as main):
//		 -fluff
//		insert 'fluff' into allocation runs to break up sweeps
//		 -how string
//		how the buffer is allocated = {stack,heap,global} (default "stack")
//		 -trace string
//		name of trace file to create
func main() {
	howAllocated, doFluffFlag := flags()
	lb := &LB{howAllocated: howAllocated, doFluff: doFluffFlag}
	lb.bench(nil)
}
