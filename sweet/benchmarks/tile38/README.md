# Tile38 Benchmark

The benchmark consists of stress-testing the [Tile38](https://tile38.com) server
with three different kinds of commands:
* `WITHIN` -- Each request looks for points or regions in the database which
  lie wholly within 100km of a given input point.
* `INTERSECTS` -- Same as `WITHIN`, but bounds may simply overlap with the
  search space, rather than be wholly enclosed by it.
* `NEARBY` -- Each request looks for the 100-nearest-neighbors to a given input
  point.

Each command is run for the amount of time specified in the CLI (default 20
seconds).

Much of the idea for the benchmarks is derived from the `tile38-benchmark`
program built as part of building tile38 from the [upstream
repository](https://github.com/tidwall/tile38/tree/master/cmd/tile38-benchmark).

This implementation is custom and not derived via source modification from the
`tile38-benchmark` tool.
