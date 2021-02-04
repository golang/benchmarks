### bent

Bent automates downloading, compiling, and running Go tests and benchmarks from various Github repositories.
By default the test/benchmark is run in a Docker container to provide some safety against accidentally making
a mess on the benchmark-running machine.

Installation:
```go get github.com/dr2chase/bent```
Also depends on burntsushi/toml, and expects that Docker is installed and available on the command line.
You can avoid the need for Docker with the `-U` command line flag, if you're okay with running benchmarks outside containers.
Alternately, if you wish to only run those benchmarks that can be compiled into a container (this is platform-dependent)
use the -S flag.

Initial usage :

```
go get github.com/dr2chase/bent
mkdir scratch
cd scratch
bent -I
cp configurations-sample.toml configurations.toml
nano configurations.toml # or use your favorite editor
bent -v # will run default set of ~50 benchmarks using supplied configuration(s)
```

Bent now comes with several shell scripts to automate common uses.
These all run using [`perflock`](https://github.com/aclements/perflock) if it is available, and default to different
numbers of builds (usually 1) and benchmark runs (usually 15) which can be
overridden at invocation.<br>

#### `cmpcl.sh refs/changes/<nn>/<cl>/<patch> [options]`
This checks out a particular version of a CL, and its immediate predecessor, and benchmarks the change.
The `refs/changes/<nn>/<cl>/<patch>` parameter is the same one that appears as a Gerrit download option for the CL.
The default is to build once, benchmark 15 times.  The results are also uploaded with [`benchsave`](https://github.com/golang/perf/tree/master/cmd/benchsave) to perf.golang.org.

#### `cmpjob.sh <branch-or-tag> <branch-or-tag> [options]`
This checks out two particular tag or branches, and benchmarks the difference.
This can be helpful when binary-searching a performance regression.
The default is to build once, benchmark 15 times. The results are also uploaded with [`benchsave`](https://github.com/golang/perf/tree/master/cmd/benchsave) to perf.golang.org.

#### `cronjob.sh [options]`
This checks out the current development tip and the most recent release (e.g. 1.14) and benchmarks
their difference.  This can be helpful for nightly performance monitoring.
The default is to build 25 times and benchmark 25 times.
The results are also uploaded with [`benchsave`](https://github.com/golang/perf/tree/master/cmd/benchsave) to perf.golang.org.
The script also contains glue to tweet the results, but by default this will silently do nothing.

#### `cmpcl-phase.sh refs/changes/<nn>/<cl>/<patch> [options]`
This checks out a particular version of a CL, and its immediate predecessor,
compiles each once with the ssa phase timing flag turned on, does not run benchmarks,
and feeds the log (with all the embedded phase timings) to [phase-times](https://github.com/dr2chase/gc-phase-times)
to help spot any bad performance trends in the new CL.
The resulting CSVs can be [imported into a spreadsheet and graphed](https://docs.google.com/spreadsheets/d/1f1rTX73ett6iKMb5LuNpnG78T7CLucQAHRKBZuI23Q4/edit?usp=sharing)
(select the "Test" sheet and scroll down below the vast table of numbers, there is a pretty chart).

The output binaries are placed in subdirectory testbin, and various
benchmark results (from building, run, and others requested) are
placed in subdirectory bench, and the binaries are also incorporated
into Docker containers if Docker is used. Each benchmark and
configuration has a shortname, and the generated binaries combine
these shortnames, for example `gonum_mat_Tip` and `gonum_mat_Go1.9`.
Benchmark files are prefixed with a run timestamp, and grouped by
configuration, with various suffixes for the various benchmarks.
Run benchmarks appears in files with suffix `.stdout`.
Others are more obviously named, with suffixes `.build`, `.benchsize`, and `.benchdwarf`.

Flags for your use:

| Flag | meaning | example |
| --- | --- | --- |
| -v | print commands as they are run | |
| -N x | benchmark/test repeat count | -N 25 |
| -B file | benchmarks file | -B benchmarks-trial.toml |
| -C file | configurations file | -C conf_1.9_and_tip.toml |
| -S | exclude unsandboxable benchmarks | |
| -U | don't sandbox benchmarks | |
| -b list | run benchmarks in comma-separated list <br> (even if normally "disabled" )| -b uuid,gonum_topo |
| -c list | use configurations from comma-separated list <br> (even if normally "disabled") | -c Tip,Go1.9 |
| -r string | skip get and build, just run. string names Docker image if needed, if not using Docker any non-empty will do. | -r f10cecc3eaac |
| -a N | repeat builds for build benchmarking | -a 10 |
| -s k | (build) shuffle flag, k = 0,1,2,3.<br>Randomizes build orders to reduce sensitivity to other machine load  | -s 2 |
| -g | get benchmarks, but do not build or run | |
| -l | list available benchmarks and configurations, then exit | |
| -T | run tests instead of benchmarks | |
| -W | print benchmark information as a markdown table | |

### Benchmark and Configuration files

Benchmarks and configurations appear in toml format, since that is
somewhat more human-friendly than JSON and in particular allows comments.
A sample benchmark entry:
```
[[Benchmarks]]
  Name = "gonum_topo"
  Repo = "gonum.org/v1/gonum/graph/topo/"
  Tests = "Test"
  Benchmarks = "Benchmark(TarjanSCCGnp_1000_half|TarjanSCCGnp_10_tenth)"
  BuildFlags = ["-tags", "purego"]
  RunWrapper = ["tmpclr"] # this benchmark leaves messes
  # NotSandboxed = true # uncomment if cannot be run in a Docker container
  # Disabled = true # uncomment to disable benchmark
```
Here, `Name` is a short name, `Repo` is where the `go get` will find the benchmark, and `Tests` and `Benchmarks` and the
regular expressions for `go test` specifying which tests or benchmarks to run.

A sample configuration entry with all the options supplied:
```
[[Configurations]]
  Name = "Go-preempt"
  Root = "$HOME/work/go/"
 # Optional flags below
  BuildFlags = ["-gccgoflags=all=-O3 -static-libgo","-tags=noasm"] # for Gollvm
  AfterBuild = ["benchsize", "benchdwarf"]
  GcFlags = "-d=ssa/insert_resched_checks/on"
  GcEnv = ["GOMAXPROCS=1","GOGC=200"]
  RunFlags = ["-test.short"]
  RunEnv = ["GOGC=1000"]
  RunWrapper = ["cpuprofile"]
  Disabled = false
```
The `Gc...` attributes apply to the test or benchmark compilation, the `Run...` attributes apply to the test or benchmark run.
A `RunWrapper` command receives the entire command line as arguments, plus the environment variable `BENT_BINARY` set to the filename
(excluding path) of the binary being run (for example, "uuid_Tip") and `BENT_I` set to the run number for this binary.
One useful example is `cpuprofile`:
```
#!/bin/bash
# Run args as command, but run cpuprofile and then pprof to capture test cpuprofile output
pf="${BENT_BINARY}_${BENT_I}.prof"
"$@" -test.cpuprofile="$pf"
echo cpuprofile in `pwd`/"$mf"
go tool pprof -text -flat -nodecount=20 "$pf"
```

When both configuration and benchmark wrappers are used the configuration wrapper runs the benchmark wrapper runs the actual benchmark, i.e.
```
ConfigWrapper ConfigArg BenchWrapper BenchArg ActualBenchmark
```

The `Disabled` attribute for both benchmarks and configurations removes them from normal use,
but leaves them accessible to explicit request with `-b` or `-c`.
