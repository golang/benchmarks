# Sweet: Benchmarking Suite for Go Implementations

Sweet is a set of benchmarks derived from the Go community which are intended
to represent a breadth of real-world applications. The primary use-case of this
suite is to perform an evaluation of the difference in CPU and memory
performance between two Go implementations.

If you use this benchmarking suite for any measurements, please ensure you use
a versioned release and note the version in the release.

## Quickstart

### Supported Platforms

* Linux
* TODO(mknyszek): Support more.

### Dependencies

The `sweet` tool only depends on having a stable version of Go and `git`.
If you're testing out a development version of Go with Sweet, don't build sweet
itself with it! Use what's installed on your system instead, or some known-stable
version.

Some benchmarks, however, have various requirements for building. Notably
they are:

* `make` (tile38)
* `bash` (tile38)
* `binutils` (tile38)

Please ensure your system has these tools installed and available in your
system's PATH.

Furthermore, some benchmarks are able to produce additional information
on some platforms. For instance, running on platforms where systemd is available
adds an average RSS measurement for the go-build benchmark.

#### gVisor

The gVisor benchmark has additional requirements:
* The target platform must be `linux/amd64`. Nothing else is supported or ever
  will be.
* The `ptrace` API must be enabled on your system. Set
  `/proc/sys/kernel/yama/ptrace_scope` appropriately (0 and 1 work, 2 might,
  3 will not).

### Build

```sh
$ go build ./cmd/sweet
```

### Getting Assets

```sh
$ ./sweet get -auth=app-default
```

Note: make sure you run `gcloud auth application-default login` before you run
the above command! 

TODO(mknyszek): Make it so authentication isn't necessary in the future.

### Running the benchmarks

Create a configuration file called `config.toml` with the following contents:

```toml
[[config]]
  name = "myconfig"
  goroot = "<insert some GOROOT here>"
```

Run the benchmarks by running:

```sh
$ ./sweet run -shell config.toml
```

Benchmark results will appear in the `results` directory.

`-shell` will cause the tool to print each action it performs as a shell
command. Note that while the shell commands are valid for many systems, they
may depend on tools being available on your system that `sweet` does not
require (e.g. `git`).

## Tips and Rules of Thumb

* You can expect the benchmarks to take a few hours to run with the default
  settings on a somewhat sizable Linux box.
* If a benchmark fails to build, run with `-shell` and copy and re-run the
  command to get output.
  TODO(mknyszek): Dump the output to the terminal.
* If a benchmark fails to run, the output should have been captured in the
  corresponding results file (e.g. if biogo-igor failed, check
  `/path/to/results/biogo-igor/myconfig.results`) which is really just the
  stderr (and usually stdout too) of the benchmark. You can also try to re-run
  it yourself with the output of `-shell`.

## Memory Requirements

These benchmarks generally try to stress the Go runtime in interesting ways, and
some may end up with very large heaps. Therefore, it's recommended to run the
suite on a system with at least 30 GiB of RAM available to minimize the chance
that results are lost due to an out-of-memory error.

## Configuration Format

The configuration is TOML-based and a more detailed description of fields may
be found in the help docs for the `run` subcommand:

```sh
$ ./sweet help run
```

## Results Format

Results are produced into a single directory containing each benchmark as a
sub-directory. Within each sub-directory is one file per configuration
containing the stderr (and usually combined stdout) of the benchmark run,
which also doubles as the benchmark output format.

All results are reported in the standard Go testing package format, such that
results may be compared using the
[benchstat](https://godoc.org/golang.org/x/perf/cmd/benchstat) tool.

Results then may also be composed together for easy viewing. For example, if
one runs sweet with two configurations named `config1` and `config2`, then to
quickly compare all results, do:

```sh
$ cat results/*/config1.results > config1.results
$ cat results/*/config2.results > config2.results
$ benchstat config1.results config2.results
```

## Noise

This benchmark suite tries to keep noise low in measurements where possible.
* Each measurement is taken against a fresh OS process.
* Benchmarks have been modified to reduce noise from the input.
  * All inputs are deterministic, including implicit inputs, such as querying an
	existing database.
  * Inputs are loaded into memory when possible instead of streamed from disk.
* The suite mitigates external effects we can control (e.g. the suite is aware
  of its co-tenancy with the benchmark and throttles itself when the benchmarks
  are running).

### Tips for Reducing Noise

* Sweet should be run on a system where a [perflock
  daemon](https://github.com/aclements/perflock) is running (to avoid noise due
  to CPU throttling).
* Avoid running these benchmarks in cloud environments if possible. Generally
  the noise inherent to those environments can skew A/B tests and hide small
  changes in performance. See [this paper](https://peerj.com/preprints/3507.pdf)
  for more details. Try to use dedicated hardware instead.

*Do not* compare results produced by separate invocations of the `sweet` tool.

### Caveats

With the current release there are a few notable caveats when it comes to the
results and the noisiness of the benchmarks that will addressed with future
releases. Notably:

* Time for the tile38 benchmarks is quite noisy, but at least consistent.
* RSS numbers for the gopher-lua and biogo-igor benchmarks are quite noisy.
* Peak RSS is currently not very reliable.
* The gVisor startup benchmarks are very noisy.
