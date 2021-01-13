// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package benchproc provides tools for filtering, grouping, and
// sorting benchmark results.
//
// This package supports a pipeline processing model based around
// domain-specific languages for filtering and projecting benchmark
// results. These languages are described in "go doc
// golang.org/x/perf/benchproc/syntax".
//
// The typical steps for processing a stream of benchmark
// results are:
//
// 1. Transform each benchfmt.Result to add or modify keys according
// to a particular tool's needs. Custom keys often start with "." to
// distinguish them from file or sub-name keys. For example,
// benchfmt.Files adds a ".label" key.
//
// 2. Filter each benchfmt.Result according to a user predicate parsed
// by NewFilter. Filters can keep entire Results, or just particular
// measurements from a Result.
//
// 3. Project each benchfmt.Result according to one or more user
// projection expressions parsed by ProjectionParser. Projecting a
// Result extracts a subset of the Result's configuration into a
// Config, which is an immutable tuple of strings whose structure is
// described by a Schema. Identical Configs compare == and hence can
// be used as map keys. Generally, tools will group Results by Config
// using one or more maps and then process each of these groups at the
// end of the stream.
//
// 4. Sort the observed Configs at the end of the Results stream. A
// projection expression also describes a sort order for Configs
// produced by that projection.
package benchproc
