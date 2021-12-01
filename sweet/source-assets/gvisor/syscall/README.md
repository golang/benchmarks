# gVisor Syscall Benchmark

This directory contains a linux/amd64 Go binary which measures raw
syscall performance by running getppid (which is usually not cached
in userspace) a fixed number of iterations. It would be ideal to run
for a specified amount of time instead, but plumbing results back
is a pain, and this is good enough. The benchmark source is in the
`src` directory.

The benchmark is loosely based off the syscall benchmark in [gVisor's
repository](https://github.com/google/gvisor/tree/3ad6d30/benchmarks/workloads/syscall).
