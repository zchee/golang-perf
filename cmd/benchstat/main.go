// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Benchstat computes statistical summaries and A/B comparisons of Go
// benchmarks.
//
// Usage:
//
//	benchstat [flags] inputs...
//
// Each input file should be in the Go benchmark format
// (https://golang.org/design/14313-benchmark-format), such as the
// output of ``go test -bench .''. Typically, there should be two (or
// more) inputs files for before and after some change (or series of
// changes) to be measured. Each benchmark should be run at least 10
// times to gather a statistically significant sample of results. For
// each benchmark, benchstat computes the median and the confidence
// interval for the median. By default, if there are two or more
// inputs files, it compares each benchmark in the first file to the
// same benchmark in each subsequent file and reports whether there
// was a statistically significant difference, though it can be
// configured to compare on other dimensions.
//
// Example
//
// Suppose we collect benchmark results from running
// ``go test -run=^$ -bench=Encode -count=10''
// before and after a particular change.
//
// The file old.txt contains:
//
//	goos: linux
//	goarch: amd64
//	pkg: golang.org/x/perf/cmd/benchstat/testdata
//	BenchmarkEncode/format=json-48         	  690848	      1726 ns/op
//	BenchmarkEncode/format=json-48         	  684861	      1723 ns/op
//	BenchmarkEncode/format=json-48         	  693285	      1707 ns/op
//	BenchmarkEncode/format=json-48         	  677692	      1707 ns/op
//	BenchmarkEncode/format=json-48         	  692130	      1713 ns/op
//	BenchmarkEncode/format=json-48         	  684164	      1729 ns/op
//	BenchmarkEncode/format=json-48         	  682500	      1736 ns/op
//	BenchmarkEncode/format=json-48         	  677509	      1707 ns/op
//	BenchmarkEncode/format=json-48         	  687295	      1705 ns/op
//	BenchmarkEncode/format=json-48         	  695533	      1774 ns/op
//	BenchmarkEncode/format=gob-48          	  372699	      3069 ns/op
//	BenchmarkEncode/format=gob-48          	  394740	      3075 ns/op
//	BenchmarkEncode/format=gob-48          	  391335	      3069 ns/op
//	BenchmarkEncode/format=gob-48          	  383588	      3067 ns/op
//	BenchmarkEncode/format=gob-48          	  385885	      3207 ns/op
//	BenchmarkEncode/format=gob-48          	  389970	      3064 ns/op
//	BenchmarkEncode/format=gob-48          	  393361	      3064 ns/op
//	BenchmarkEncode/format=gob-48          	  393882	      3058 ns/op
//	BenchmarkEncode/format=gob-48          	  396171	      3059 ns/op
//	BenchmarkEncode/format=gob-48          	  397812	      3062 ns/op
//
// The file new.txt contains:
//
//	goos: linux
//	goarch: amd64
//	pkg: golang.org/x/perf/cmd/benchstat/testdata
//	BenchmarkEncode/format=json-48         	  714387	      1423 ns/op
//	BenchmarkEncode/format=json-48         	  845445	      1416 ns/op
//	BenchmarkEncode/format=json-48         	  815714	      1411 ns/op
//	BenchmarkEncode/format=json-48         	  828824	      1413 ns/op
//	BenchmarkEncode/format=json-48         	  834070	      1412 ns/op
//	BenchmarkEncode/format=json-48         	  828123	      1426 ns/op
//	BenchmarkEncode/format=json-48         	  834493	      1422 ns/op
//	BenchmarkEncode/format=json-48         	  838406	      1424 ns/op
//	BenchmarkEncode/format=json-48         	  836227	      1447 ns/op
//	BenchmarkEncode/format=json-48         	  830835	      1425 ns/op
//	BenchmarkEncode/format=gob-48          	  394441	      3075 ns/op
//	BenchmarkEncode/format=gob-48          	  393207	      3065 ns/op
//	BenchmarkEncode/format=gob-48          	  392374	      3059 ns/op
//	BenchmarkEncode/format=gob-48          	  396037	      3065 ns/op
//	BenchmarkEncode/format=gob-48          	  393255	      3060 ns/op
//	BenchmarkEncode/format=gob-48          	  382629	      3081 ns/op
//	BenchmarkEncode/format=gob-48          	  389558	      3186 ns/op
//	BenchmarkEncode/format=gob-48          	  392668	      3135 ns/op
//	BenchmarkEncode/format=gob-48          	  392313	      3087 ns/op
//	BenchmarkEncode/format=gob-48          	  394274	      3062 ns/op
//
// The order of the lines in the file does not matter, except that the
// output lists benchmarks in order of appearance.
//
// If we run ``benchstat old.txt new.txt'', it will summarize the
// benchmarks and compare the before and after results:
//
//	goos: linux
//	goarch: amd64
//	pkg: golang.org/x/perf/cmd/benchstat/testdata
//	                      │   old.txt   │               new.txt               │
//	                      │   sec/op    │   sec/op     vs base                │
//	Encode/format=json-48   1.718µ ± 1%   1.423µ ± 1%  -17.20% (p=0.000 n=10)
//	Encode/format=gob-48    3.066µ ± 0%   3.070µ ± 2%        ~ (p=0.446 n=10)
//	geomean                 2.295µ        2.090µ        -8.94%
//
// Before the comparison table, we see common file-level
// configuration. If there are benchmarks with different configuration
// (for example, from different packages), benchstat will print
// separate tables for each configuration.
//
// The table then compares the two input files for each benchmark. It
// shows the median and 95% confidence interval summaries for each
// benchmark before and after the change, and an A/B comparison under
// "vs base". The comparison shows that Encode/format=json got 17.20%
// faster with a p-value of 0.000 and 10 samples from each input file.
// The p-value measures how likely it is that any differences were due
// to random chance (i.e., noise). In this case, it's extremely
// unlikely the difference between the medians was due to chance. For
// Encode/format=gob, the "~" means benchstat did not detect a
// statistically significant difference between the two inputs. In
// this case, we see a p-value of 0.446, meaning it's very likely the
// differences for this benchmark are simply due to random chance.
//
// Note that "statistically significant" is not the same as "large":
// with enough low-noise data, even very small changes can be
// distinguished from noise and considered statistically significant.
// It is, of course, generally easier to distinguish large changes
// from noise.
//
// Finally, the last row of the table shows the geometric mean of each
// column, giving an overall picture of how the benchmarks moved.
// Proportional changes in the geomean reflect proportional changes in
// the benchmarks. For example, given n benchmarks, if sec/op for one
// of them increases by a factor of 2, then the sec/op geomean will
// increase by a factor of ⁿ√2.
//
//
// Configuring comparisons
//
// benchstat has a very flexible system of configuring exactly which
// benchmarks are summarized and compared. Its inputs can be filtered
// using the -filter flag, which supports the same filter expressions
// as the benchfilter tool (see
// https://pkg.go.dev/golang.org/x/perf/cmd/benchfilter).
//
// After filtering, it treats its inputs as a multi-dimensional
// database, where each benchmark result is associated with its name,
// file-level configuration, and sub-name configuration. These
// dimensions are flattened into a set of two dimensional tables using
// "projections", which can be configured via flags.
//
// Each result has the following dimensions:
//
// 	.name         - The base name of a benchmark
// 	.fullname     - The full name of a benchmark (including configuration)
// 	.label        - The name of the input file or user-provided file label
// 	/{name-key}   - Per-benchmark sub-name configuration key
// 	{file-key}    - File-level configuration key
//	.config       - All file-level configuration keys
//
// A projection is a comma- or space-separated list of dimensions,
// each of which may have an optional sort order. See
// https://pkg.go.dev/golang.org/x/perf/benchproc/syntax for details
// of the projection syntax.
//
// benchstat first splits its inputs into tables according to the
// -table projection. This defaults to ".config"; that is, each
// distinct file-level configuration will get a separate table.
//
// Within each table, benchstat groups results into rows and columns
// according to the -row and -col projections. The -row flag defaults
// to ".fullname"; that is, the name of each benchmark, including its
// sub-name configuration. The above example showed how benchmark
// grouped the results into "Encode/format=json" and
// "Encode/format=gob".
//
// The -col flag defaults to ".label"; that is, the file name provided
// on the command line. These labels can be overridden by specifying
// an input argument of the form "label=path" instead of just "path".
// This is particularly useful for shortening long file names.
//
// When projections overlap, benchstat assigns dimensions to the most
// specific projection. For example, if the table projection is the
// full file-level configuration ".config", and the column projection
// is the specific file key "goarch", benchstat will omit "goarch"
// from ".config".
//
// Finally, the -ignore projection tells benchstat to group results
// *despite* any differences in the ignored keys.
//
//
// Projection example
//
// Suppose we want to compare json encoding to gob encoding from
// new.txt in the above example.
//
//	$ benchstat -col /format new.txt
//	.label: new.txt
//	goos: linux
//	goarch: amd64
//	pkg: golang.org/x/perf/cmd/benchstat/testdata
//	                   │    json     │                 gob                  │
//	                   │   sec/op    │   sec/op     vs base                 │
//	Encode/format=*-48   1.423µ ± 1%   3.070µ ± 2%  +115.82% (p=0.000 n=10)
//
// The columns are now labeled by the "/format" configuration from the
// benchmark name. benchstat still compares columns even though we've
// only provided a single input file. We also see that /format has
// been stubbed-out in the benchmark name to make a single row.
//
// We can simplify the output by ignoring .label and grouping rows by
// just the benchmark name, rather than the full name:
//
//	$ benchstat -col /format -row .name -ignore .label new.txt
//	goos: linux
//	goarch: amd64
//	pkg: golang.org/x/perf/cmd/benchstat/testdata
//	        │    json     │                 gob                  │
//	        │   sec/op    │   sec/op     vs base                 │
//	Encode    1.423µ ± 1%   3.070µ ± 2%  +115.82% (p=0.000 n=10)
//
// We can also control exactly that's used for .label from the command
// line. For example, support we do want to compare two files, but
// give them different (perhaps shorter) labels than the full file
// names:
//
//	$ benchstat A=old.txt B=new.txt
//	goos: linux
//	goarch: amd64
//	pkg: golang.org/x/perf/cmd/benchstat/testdata
//	                      │      A      │                  B                  │
//	                      │   sec/op    │   sec/op     vs base                │
//	Encode/format=json-48   1.718µ ± 1%   1.423µ ± 1%  -17.20% (p=0.000 n=10)
//	Encode/format=gob-48    3.066µ ± 0%   3.070µ ± 2%        ~ (p=0.446 n=10)
//	geomean                 2.295µ        2.090µ        -8.94%
//
// benchstat will attempt to detect and warn if projections strip away
// too much information. For example, here we group together json and
// gob results into a single row:
//
//	$ benchstat  -row .name new.txt
//	goos: linux
//	goarch: amd64
//	pkg: golang.org/x/perf/cmd/benchstat/testdata
//	       │    new.txt     │
//	       │     sec/op     │
//	Encode   2.253µ ± 37% ¹
//	¹ benchmarks vary in .fullname
//
// Since this is probably not a meaningful comparison, benchstat warns
// that the benchmarks it grouped together vary in a hidden dimension.
// If this really were our intent, we could -ignore .fullname.
//
//
// Sorting
//
// By default, benchstat sorts each dimension according to the order
// in which it first observes each value of that dimension. This can
// be overridden in each projection using the following syntax:
//
// {key}@{order} - specifies one of the built-in named sort orders.
// This can be "alpha" or "num" for alphabetic or numeric sorting.
// "num" understands basic use of metric and IEC prefixes like "2k"
// and "1Mi".
//
// {key}@({value} {value} ...) - specifies a fixed value order for
// key. It also specifies a filter: if key has a value that isn't any
// of the specified values, the result is filtered out.
//
// For example, we can use a fixed order to compare the improvement of
// json over gob rather than the other way around:
//
//	$ benchstat -col "/format@(gob json)" -row .name -ignore .label new.txt
//	goos: linux
//	goarch: amd64
//	pkg: golang.org/x/perf/cmd/benchstat/testdata
//	       │     gob     │                json                 │
//	       │   sec/op    │   sec/op     vs base                │
//	Encode   3.070µ ± 2%   1.423µ ± 1%  -53.66% (p=0.000 n=10)
//
//
// Units
//
// benchstat normalizes the units "ns" to "sec" and "MB" to "B" to
// avoid creating nonsense units like "µns/op". These appear in the
// testing package's default metrics and are also common in custom
// metrics.
//
// benchstat supports custom unit metadata (see
// https://golang.org/design/14313-benchmark-format). In particular,
// "assume" metadata is useful for controlling the statistics used by
// benchstat. By default, units use "assume=nothing", so benchstat
// uses non-parametric statistics: median for summaries, and the
// Mann-Whitney U-test for A/B comparisons.
//
// Some benchmarks measure things that have no noise, such as the size
// of a binary produced by a compiler. These do not benefit from
// repeated measurements or non-parametric statistics. For these
// units, it's useful to set "assume=exact". This will cause benchstat
// to warn if there's any variation in the measured values, and to
// show A/B comparisons even if there's only one before and after
// measurement.
//
//
// Tips
//
// Reducing noise and/or increasing the number of benchmark runs will
// enable benchstat to discern smaller changes as "statistically
// significant". To reduce noise, make sure you run benchmarks on an
// otherwise idle machine, ideally one that isn't running on battery
// and isn't likely to be affected by thermal throttling.
// https://llvm.org/docs/Benchmarking.html has many good tips on
// reducing noise in benchmarks.
//
// It's also important that noise is evenly distributed across
// benchmark runs. The best way to do this is to interleave before and
// after runs, rather than running, say, 10 iterations of the before
// benchmark, and then 10 iterations of the after benchmark. For Go
// benchmarks, you can often speed up this process by using "go test
// -c" to pre-compile the benchmark binary.
//
// Pick a number of benchmark runs (at least 10, ideally 20) and stick
// to it. If benchstat reports no statistically significant change,
// avoid simply rerunning your benchmarks until it reports a
// significant change. This is known as "multiple testing" and is a
// common statistical error. By default, benchstat uses an ɑ threshold
// of 0.05, which means it is *expected* to show a difference 5% of
// the time even if there is no difference. Hence, if you rerun
// benchmarks looking for a change, benchstat will probably eventually
// say there is a change, even if there isn't, which creates a
// statistical bias.
//
// As an extension of this, if you compare a large number of
// benchmarks, you should expect that about 5% of them will report a
// statistically significant change even if there is no difference
// between the before and after.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/perf/benchfmt"
	"golang.org/x/perf/benchmath"
	"golang.org/x/perf/benchproc"
	"golang.org/x/perf/cmd/benchstat/internal/benchtab"
)

// TODO: Add a flag to perform Holm–Bonferroni correction.

// TODO: -unit flag.

// TODO: Support sorting by commit order.

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: benchstat [flags] inputs...

benchstat computes statistical summaries and A/B comparisons of Go
benchmarks. It shows benchmark medians in a table with a row for each
benchmark and a column for each input file. If there is more than one
input file, it also shows A/B comparisons between the files. If a
difference is likely to be noise, it shows "~".

For details, see https://pkg.go.dev/golang.org/x/perf/cmd/benchstat.
`)
	flag.PrintDefaults()
}

func main() {
	if err := benchstat(os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "benchstat: %s\n", err)
	}
}

func benchstat(w, wErr io.Writer, args []string) error {
	flags := flag.FlagSet{Usage: usage}
	thresholds := benchmath.DefaultThresholds
	flagTable := flags.String("table", ".config", "split results into tables by distinct values of `projection`")
	flagRow := flags.String("row", ".fullname", "split results into rows by distinct values of `projection`")
	flagCol := flags.String("col", ".label", "split results into columns by distinct values of `projection`")
	flagIgnore := flags.String("ignore", "", "ignore variations in `keys`")
	flagFilter := flags.String("filter", "*", "use only benchmarks matching benchfilter `query`")
	flags.Float64Var(&thresholds.CompareAlpha, "alpha", thresholds.CompareAlpha, "consider change significant if p < `α`")
	// TODO: Support -confidence none to disable CI column? This
	// would be equivalent to benchstat v1's -norange for CSV.
	flagConfidence := flags.Float64("confidence", 0.95, "confidence `level` for ranges")
	flagFormat := flags.String("format", "text", "print results in `format`:\n  text - plain text\n  csv  - comma-separated values (warnings will be written to stderr)\n")
	flags.Parse(args)

	if flags.NArg() == 0 {
		usage()
		os.Exit(2)
	}

	filter, err := benchproc.NewFilter(*flagFilter)
	if err != nil {
		return fmt.Errorf("parsing -filter: %s", err)
	}

	var parser benchproc.ProjectionParser
	var parseErr error
	mustParse := func(name, val string) *benchproc.Schema {
		schema, err := parser.Parse(val, filter)
		if err != nil && parseErr == nil {
			parseErr = fmt.Errorf("parsing %s: %s", name, err)
		}
		return schema
	}
	tableBy := mustParse("-table", *flagTable)
	rowBy := mustParse("-row", *flagRow)
	colBy := mustParse("-col", *flagCol)
	mustParse("-ignore", *flagIgnore)
	residue := parser.Residue()
	if parseErr != nil {
		return parseErr
	}

	if thresholds.CompareAlpha < 0 || thresholds.CompareAlpha > 1 {
		return fmt.Errorf("-alpha must be in range [0, 1]")
	}
	if *flagConfidence < 0 || *flagConfidence > 1 {
		return fmt.Errorf("-confidence must be in range [0, 1]")
	}
	var format func(t *benchtab.Tables) error
	switch *flagFormat {
	default:
		return fmt.Errorf("-format must be text or csv")
	case "text":
		format = func(t *benchtab.Tables) error { return t.ToText(w, false) }
	case "csv":
		format = func(t *benchtab.Tables) error { return t.ToCSV(w, wErr) }
	}

	stat := benchtab.NewBuilder(tableBy, rowBy, colBy, residue)
	files := benchfmt.Files{Paths: flags.Args(), AllowStdin: true, AllowLabels: true}
	for files.Scan() {
		res, err := files.Result()
		if err != nil {
			// Non-fatal result parse error. Warn
			// but keep going.
			fmt.Fprintln(wErr, err)
			continue
		}

		if !filter.Apply(res) {
			continue
		}

		stat.Add(res)
	}
	if err := files.Err(); err != nil {
		return err
	}

	tables := stat.ToTables(benchtab.TableOpts{
		Confidence: *flagConfidence,
		Thresholds: &thresholds,
		Units:      files.Units(),
	})
	return format(tables)
}
