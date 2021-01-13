// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package syntax documents the syntax used by benchmark filter and
// projection expressions.
//
// These expressions work with benchmark data in the Go benchmark
// format (https://golang.org/design/14313-benchmark-format). Each
// benchmark result (a line beginning with "Benchmark") consists of
// several components, including a name, name-based configuration, and
// file configuration pairs ("key: value" lines).
//
// Filters and projections share a common syntax for referring to
// these components of a benchmark result:
//
// - ".name" refers to the benchmark name, excluding per-benchmark
// configuration. For example, the ".name" of the
// "BenchmarkCopy/size=4k-16" benchmark is "Copy".
//
// - ".fullname" refers to the full benchmark name, including
// per-benchmark configuration, but excluding the "Benchmark" prefix.
// For example, the ".fullname" of "BenchmarkCopy/size=4k-16" is
// "Copy/size=4k-16".
//
// - "/{key}" refers to value of {key} from the benchmark name
// configuration. For example, the "/size" of
// "BenchmarkCopy/size=4k-16", is "4k". As a special case,
// "/gomaxprocs" recognizes both a literal "/gomaxprocs=" in the name,
// and the "-N" convention. For the above example, "/gomaxprocs" is
// "16".
//
// - Any name NOT prefixed with "/" or "." refers to the value of a
// file configuration key. For example, the "testing" package
// automatically emits a few file configuration keys, including "pkg",
// "goos", and "goarch", so the projection "pkg" extracts the package
// path of a benchmark.
//
// - ".unit" (only in filters) refers to individual measurements in a
// result, such as the "ns/op" measurement. The filter ".unit:ns/op"
// extracts just the ns/op measurement of a result. This will match
// both original units (e.g., "ns/op") and tidied units (e.g.,
// "sec/op").
//
// - ".config" (only in projections) refers to the full file
// configuration of a benchmark. This isn't a string like the other
// components, but rather a tuple.
//
// - ".label" refers to the input file provided on the command line
// (for command-line tools that use benchfmt.Files).
//
// Filters
//
// Filters are boolean expressions that match or exclude benchmark
// results or individual measurements.
//
// A basic "key:value" filter matches results for which the value of
// "key" is "value". Keys and values can be bare words if they don't
// contain any special characters, or double-quoted strings using Go
// syntax. Values can also be regular expressions surrounded by "/"s,
// such as "key:/regexp?/". Basic filters can be extended to
// "key:(value1 value2 ...)", which will match if any of the values
// match. Finally, the basic filter "*" matches everything.
//
// Filters can be combined into more complex expressions. Filters can
// be prefixed with "-" to negate them, or combined with "AND" and
// "OR" operators and parenthesis to build up expressions. The "AND"
// operator can be omitted, so "a:b AND c:d" is equivalent to "a:b
// c:d".
//
// Detailed syntax:
//
//   expr     = andExpr {"OR" andExpr}
//   andExpr  = match {"AND"? match}
//   match    = "(" expr ")"
//            | "-" match
//            | "*"
//            | key ":" value
//            | key ":" "(" value {value} ")"
//   key      = word
//   value    = word
//            | "/" regexp "/"
//
// Projections
//
// A projection expresses how to extract a tuple of data from a
// benchmark result, as well as a sort order for projected tuples.
//
// A projection is a comma- or space-separated list of components.
// Each component specifies a key and optionally a sort order and a
// filter as follows:
//
// - "key" extracts the named component and orders it using the order
// values of this key are first observed in the data.
//
// - "key@order" specifies one of the built-in named sort orders. This
// can be "alpha" or "num" for alphabetic or numeric sorting. "num"
// understands basic use of metric and IEC prefixes like "2k" and
// "1Mi".
//
// - "key@(value value ...)" specifies a fixed value order for key.
// It also specifies a filter: if key has a value that isn't any of
// the specified values, the result is filtered out.
//
// Syntax:
//
//   expr     = part {","? part}
//   part     = key
//            | key "@" order
//            | key "@" "(" word {word} ")"
//   key      = word
//   order    = word
//
// Common syntax
//
// Filters and projections share the following common base syntax:
//
//   word     = bareWord
//            | double-quoted Go string
//   bareWord = [^-*"():@,][^ ():@,]*
package syntax
