# Go benchmark analysis tools

[![Go Reference](https://pkg.go.dev/badge/golang.org/x/perf.svg)](https://pkg.go.dev/golang.org/x/perf)

This subrepository holds tools and packages for analyzing [Go
benchmark results](https://golang.org/design/14313-benchmark-format),
such as the output of [testing package
benchmarks](https://pkg.go.dev/testing).

## Tools

This subrepository contains command-line tools for analyzing benchmark
result data.

[cmd/benchstat](cmd/benchstat) computes statistical summaries and A/B
comparisons of Go benchmarks.

[cmd/benchfilter](cmd/benchfilter) filters the contents of benchmark
result files.

[cmd/benchsave](cmd/benchsave) publishes benchmark results to
[perf.golang.org](https://perf.golang.org).

To install all of these commands, run
`go install golang.org/x/perf/cmd/...@latest`.
You can also manually git clone the repository and run
`go install ./cmd/...`.

## Packages

Underlying the above tools are several packages for working with
benchmark data. These are designed to work together, but can also be
used independently.

[benchfmt](benchfmt) reads and writes the Go benchmark format.

[benchunit](benchunit) manipulates benchmark units and formats numbers
in those units.

[benchproc](benchproc) provides tools for filtering, grouping, and
sorting benchmark results.

[benchmath](benchmath) provides tools for computing statistics over
distributions of benchmark measurements.

## perf.golang.org

This subrepository also contains the implementation of Go's benchmark
data servers, perf.golang.org and perfdata.golang.org.

[storage](storage) contains the https://perfdata.golang.org/ benchmark
result storage system.

[analysis](analysis) contains the https://perf.golang.org/ benchmark
result analysis system.

Both storage and analysis can be run locally; the following commands will run
the complete stack on your machine with an in-memory datastore.

```
go install golang.org/x/perf/storage/localperfdata@latest
go install golang.org/x/perf/analysis/localperf@latest
localperfdata -addr=:8081 -view_url_base=http://localhost:8080/search?q=upload: &
localperf -addr=:8080 -storage=http://localhost:8081
```

The storage system is designed to have a [standardized
API](https://perfdata.golang.org/), and we encourage additional analysis
tools to be written against the API. A client can be found in the
[storage](https://godoc.org/golang.org/x/perf/storage) package.

## Report Issues / Send Patches

This repository uses Gerrit for code changes. To learn how to submit changes to
this repository, see https://golang.org/doc/contribute.html.

The main issue tracker for the perf repository is located at
https://github.com/golang/go/issues. Prefix your issue with "x/perf:" in the
subject line, so it is easy to find.
