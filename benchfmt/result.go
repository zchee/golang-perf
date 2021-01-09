// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package benchfmt provides a high-performance reader and writer for
// the Go benchmark format.
//
// This implements the format documented at
// https://golang.org/design/14313-benchmark-format.
//
// The reader and writer are structured as streaming operations to
// allow incremental processing and avoid dictating a data model. This
// allows consumers of these APIs to provide their own data model best
// suited to its needs. The reader also performs in-place updates to
// reduce allocation, enabling it to parse millions of benchmark
// results per second on a typical laptop.
//
// This package is designed to be used with the higher-level packages
// benchunit, benchmath, and benchproc.
package benchfmt

import "bytes"

// A Result is a single benchmark result and all of its measurements.
//
// Results are designed to be mutated in place and reused to reduce
// allocation.
type Result struct {
	// FileConfig is the set of file-level key/value pairs in
	// effect for this result.
	//
	// This slice is mutable, as are the values in the slice.
	// Result internally maintains an index of the keys of this slice,
	// so callers must use SetFileConfig to add or delete keys,
	// but may modify values in place. There is one exception to this:
	// for convenience, new Results can be initialized directly,
	// e.g., using a struct literal.
	//
	// SetFileConfig appends new keys to this slice and updates
	// existing ones in place. To delete a key, it swaps the deleted
	// key with the final slice element. This way, the order of
	// these keys is deterministic.
	FileConfig []Config

	// Name is the full name of this benchmark, including all
	// sub-benchmark configuration.
	Name Name

	// Iters is the number of iterations this benchmark's results
	// were averaged over.
	Iters int

	// Values is this benchmark's measurements and their units.
	Values []Value

	// configPos maps from Config.Key to index in FileConfig. This
	// may be nil, which indicates the index needs to be
	// constructed.
	configPos map[string]int
}

// A Config is a single key/value configuration pair.
type Config struct {
	Key   string
	Value []byte
}

// Note: I tried many approaches to Config. Using two strings is nice
// for the API, but forces a lot of allocation in extractors (since
// they either need to convert strings to []byte or vice-versa). Using
// a []byte for Value makes it slightly harder to use, but is good for
// reusing space efficiently (Value is likely to have more distinct
// values than Key) and lets all extractors work in terms of []byte
// views. Making Key a []byte is basically all downside.

// A Value is a single value/unit measurement from a benchmark result.
//
// Values should be tidied to use base units like "sec" and "B" when
// constructed. Reader ensures this.
type Value struct {
	Value float64
	Unit  string

	// OrigValue and OrigUnit, if non-zero, give the original,
	// untidied value and unit, typically as read from the
	// original input. OrigUnit may be "", indicating that the
	// value wasn't transformed.
	OrigValue float64
	OrigUnit  string
}

// Clone makes a copy of Result that shares no state with r.
func (r *Result) Clone() *Result {
	r2 := &Result{
		FileConfig: make([]Config, len(r.FileConfig)),
		Name:       append([]byte(nil), r.Name...),
		Iters:      r.Iters,
		Values:     append([]Value(nil), r.Values...),
	}
	for i, cfg := range r.FileConfig {
		r2.FileConfig[i].Key = cfg.Key
		r2.FileConfig[i].Value = append([]byte(nil), cfg.Value...)
	}
	return r2
}

// SetFileConfig sets file configuration key to value, overriding or
// adding the configuration as necessary. If value is "",
// SetFileConfig deletes key.
func (r *Result) SetFileConfig(key, value string) {
	if value == "" {
		r.deleteFileConfig(key)
	} else {
		cfg := r.ensureFileConfig(key)
		cfg.Value = append(cfg.Value[:0], value...)
	}
}

func (r *Result) ensureFileConfig(key string) *Config {
	pos, ok := r.FileConfigIndex(key)
	if ok {
		return &r.FileConfig[pos]
	}
	// Add key. Reuse old space if possible.
	r.configPos[key] = len(r.FileConfig)
	if len(r.FileConfig) < cap(r.FileConfig) {
		r.FileConfig = r.FileConfig[:len(r.FileConfig)+1]
		cfg := &r.FileConfig[len(r.FileConfig)-1]
		cfg.Key = key
		return cfg
	}
	r.FileConfig = append(r.FileConfig, Config{key, nil})
	return &r.FileConfig[len(r.FileConfig)-1]
}

func (r *Result) deleteFileConfig(key string) {
	pos, ok := r.FileConfigIndex(key)
	if !ok {
		return
	}
	// Delete key.
	cfg := &r.FileConfig[pos]
	cfg2 := &r.FileConfig[len(r.FileConfig)-1]
	*cfg, *cfg2 = *cfg2, *cfg
	r.configPos[cfg.Key] = pos
	r.FileConfig = r.FileConfig[:len(r.FileConfig)-1]
	delete(r.configPos, key)
}

// GetFileConfig returns the value of a file configuration key, or ""
// if not present.
func (r *Result) GetFileConfig(key string) string {
	pos, ok := r.FileConfigIndex(key)
	if !ok {
		return ""
	}
	return string(r.FileConfig[pos].Value)
}

// FileConfigIndex returns the index in r.FileConfig of key.
func (r *Result) FileConfigIndex(key string) (pos int, ok bool) {
	if r.configPos == nil {
		// This is a fresh Result. Construct the index.
		r.configPos = make(map[string]int)
		for i, cfg := range r.FileConfig {
			r.configPos[cfg.Key] = i
		}
	}

	pos, ok = r.configPos[key]
	return
}

// Value returns the measurement for the given unit.
func (r *Result) Value(unit string) (float64, bool) {
	for _, v := range r.Values {
		if v.Unit == unit {
			return v.Value, true
		}
	}
	return 0, false
}

// A Name is a full benchmark name, including all sub-benchmark
// configuration.
type Name []byte

// String returns the full benchmark name as a string.
func (n Name) String() string {
	return string(n)
}

// Full returns the full benchmark name as a []byte. This is simply
// the value of n, but helps with code readability.
func (n Name) Full() []byte {
	return n
}

// Base returns the base part of a full benchmark name, without any
// configuration keys or GOMAXPROCS.
func (n Name) Base() []byte {
	slash := bytes.IndexByte(n.Full(), '/')
	if slash >= 0 {
		return n[:slash]
	}
	base, _ := n.splitGomaxprocs()
	return base
}

// Parts splits a benchmark name into the base name and sub-benchmark
// configuration parts. Each sub-benchmark configuration part is one
// of three forms:
//
// 1. "/<key>=<value>" indicates a key/value configuration pair.
//
// 2. "/<string>" indicates a positional configuration pair.
//
// 3. "-<gomaxprocs>" indicates the GOMAXPROCS of this benchmark. This
// component can only appear last.
//
// Concatenating the base name and the configuration parts
// reconstructs the full name.
func (n Name) Parts() (baseName []byte, parts [][]byte) {
	// First pull off any GOMAXPROCS.
	buf, gomaxprocs := n.splitGomaxprocs()
	// Split the remaining parts.
	var nameParts [][]byte
	prev := 0
	for i, c := range buf {
		if c == '/' {
			nameParts = append(nameParts, buf[prev:i])
			prev = i
		}
	}
	nameParts = append(nameParts, buf[prev:])
	if gomaxprocs != nil {
		nameParts = append(nameParts, gomaxprocs)
	}
	return nameParts[0], nameParts[1:]
}

func (n Name) splitGomaxprocs() (prefix, gomaxprocs []byte) {
	for i := len(n) - 1; i >= 0; i-- {
		if n[i] == '-' && i < len(n)-1 {
			return n[:i], n[i:]
		}
		if !('0' <= n[i] && n[i] <= '9') {
			// Not a digit.
			break
		}
	}
	return n, nil
}
