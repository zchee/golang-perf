// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/perf/benchfmt"
)

// An extractor returns some component of a benchmark result. The
// result may be a view into a mutable []byte in *benchfmt.Result, so
// it may change if the Result is modified.
type extractor func(*benchfmt.Result) []byte

// newExtractor returns a function that extracts some component of a
// benchmark result.
//
// The key must be one of the following:
//
// - ".name" for the benchmark name (excluding per-benchmark
// configuration).
//
// - ".fullname" for the full benchmark name (including per-benchmark
// configuration).
//
// - "/{key}" for a benchmark sub-name key. This may be "/gomaxprocs"
// and the extractor will normalize the name as needed.
//
// - Any other string is a file configuration key.
func newExtractor(key string) (extractor, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("key must not be empty")
	}

	switch {
	case key == ".name":
		return extractName, nil

	case key == ".fullname":
		return extractFull, nil

	case strings.HasPrefix(key, "/"):
		// Construct the byte prefix to search for.
		prefix := make([]byte, len(key)+1)
		copy(prefix, key)
		prefix[len(prefix)-1] = '='
		isGomaxprocs := key == "/gomaxprocs"
		return func(res *benchfmt.Result) []byte {
			return extractNamePart(res, prefix, isGomaxprocs)
		}, nil
	}

	return func(res *benchfmt.Result) []byte {
		return extractFileKey(res, key)
	}, nil
}

// newExtractorFullName returns an extractor for the full name of a
// benchmark, but optionally with the base name or sub-name
// configuration keys excluded. Any excluded sub-name keys will be
// normalized to "/key=*" (or "-*" for gomaxprocs). If ".name" is
// excluded, the name will be normalized to "*". This will ignore
// anything in the exclude list that isn't in the form of a /-prefixed
// sub-name key or ".name".
func newExtractorFullName(exclude []string) extractor {
	// Extract the sub-name keys, turn them into substrings and
	// construct their normalized replacement.
	var replace [][]byte
	excName := false
	excGomaxprocs := false
	for _, k := range exclude {
		if k == ".name" {
			excName = true
		}
		if !strings.HasPrefix(k, "/") {
			continue
		}
		replace = append(replace, append([]byte(k), '='))
		if k == "/gomaxprocs" {
			excGomaxprocs = true
		}
	}
	if len(replace) == 0 && !excName && !excGomaxprocs {
		return extractFull
	}
	return func(res *benchfmt.Result) []byte {
		return extractFullExcluded(res, replace, excName, excGomaxprocs)
	}
}

func extractName(res *benchfmt.Result) []byte {
	return res.Name.Base()
}

func extractFull(res *benchfmt.Result) []byte {
	return res.Name.Full()
}

func extractFullExcluded(res *benchfmt.Result, replace [][]byte, excName, excGomaxprocs bool) []byte {
	name := res.Name.Full()
	found := false
	if excName {
		found = true
	}
	if !found {
		for _, k := range replace {
			if bytes.Contains(name, k) {
				found = true
				break
			}
		}
	}
	if !found && excGomaxprocs && bytes.IndexByte(name, '-') >= 0 {
		found = true
	}
	if !found {
		// No need to transform name.
		return name
	}

	// Normalize excluded keys from the name.
	base, parts := res.Name.Parts()
	var newName []byte
	if excName {
		newName = append(newName, '*')
	} else {
		newName = append(newName, base...)
	}
outer:
	for _, part := range parts {
		for _, k := range replace {
			if bytes.HasPrefix(part, k) {
				newName = append(append(newName, k...), '*')
				continue outer
			}
		}
		if excGomaxprocs && part[0] == '-' {
			newName = append(newName, "-*"...)
			continue outer
		}
		newName = append(newName, part...)
	}
	return newName
}

func extractNamePart(res *benchfmt.Result, prefix []byte, isGomaxprocs bool) []byte {
	_, parts := res.Name.Parts()
	if isGomaxprocs && len(parts) > 0 {
		last := parts[len(parts)-1]
		if last[0] == '-' {
			// GOMAXPROCS specified as "-N" suffix.
			return last[1:]
		}
	}
	// Search for the prefix.
	for _, part := range parts {
		if bytes.HasPrefix(part, prefix) {
			return part[len(prefix):]
		}
	}
	// Not found.
	return nil
}

func extractFileKey(res *benchfmt.Result, key string) []byte {
	pos, ok := res.FileConfigIndex(key)
	if !ok {
		return nil
	}
	return res.FileConfig[pos].Value
}
