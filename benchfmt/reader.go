// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchfmt

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"unicode"
	"unicode/utf8"

	"golang.org/x/perf/benchfmt/internal/bytesconv"
	"golang.org/x/perf/benchunit"
)

// A Reader reads the Go benchmark format.
//
// Its API is modeled on bufio.Scanner. To minimize allocation, a
// Reader retains ownership of everything it creates; a caller should
// copy anything it needs to retain.
//
// The zero value of the Reader is a valid Reader, but the user must
// call Reset before using it.
type Reader struct {
	s        *bufio.Scanner
	fileName string
	lineNum  int
	err      error // current I/O error

	result    Result
	resultErr error

	interns map[string]string
}

// A SyntaxError represents a syntax error on a particular line of a
// benchmark results file.
type SyntaxError struct {
	FileName string
	Line     int
	Msg      string
}

func (s *SyntaxError) Error() string {
	return fmt.Sprintf("%s:%d: %s", s.FileName, s.Line, s.Msg)
}

var noResult = errors.New("Reader.Scan has not been called")

// NewReader constructs a reader to parse the Go benchmark format from r.
// fileName is used in error messages; it is purely diagnostic.
func NewReader(r io.Reader, fileName string) *Reader {
	reader := new(Reader)
	reader.Reset(r, fileName)
	return reader
}

// Reset resets the reader to begin reading from a new input.
// It also resets all of the file-level configuration values.
// It does NOT reset unit metadata because it carries across files.
//
// initConfig is an alternating sequence of keys and values.
// Reset will install these as the initial file-level configuration
// before any results are read from the input file.
func (r *Reader) Reset(ior io.Reader, fileName string, initConfig ...string) {
	r.s = bufio.NewScanner(ior)
	if fileName == "" {
		fileName = "<unknown>"
	}
	r.fileName = fileName
	r.lineNum = 0
	r.err = nil
	r.resultErr = noResult
	if r.interns == nil {
		r.interns = make(map[string]string)
	}

	// Wipe the Result.
	r.result.FileConfig = r.result.FileConfig[:0]
	r.result.Name = r.result.Name[:0]
	r.result.Iters = 0
	r.result.Values = r.result.Values[:0]
	for k := range r.result.configPos {
		delete(r.result.configPos, k)
	}

	// Set up initial configuration.
	if len(initConfig)%2 != 0 {
		panic("len(initConfig) must be a multiple of 2")
	}
	for i := 0; i < len(initConfig); i += 2 {
		r.result.SetFileConfig(initConfig[i], initConfig[i+1])
	}
}

var (
	benchmarkPrefix = []byte("Benchmark")
	unitPrefix      = []byte("Unit")
)

// Scan advances the reader to the next result and reports whether a
// result was read.
// The caller should use the Result method to get the result.
// If Scan reaches EOF or an I/O error occurs, it returns false,
// in which case the caller should use the Err method to check for errors.
func (r *Reader) Scan() bool {
	if r.err != nil {
		return false
	}

	for r.s.Scan() {
		r.lineNum++
		// We do everything in byte buffers to avoid allocation.
		line := r.s.Bytes()
		// Most lines are benchmark lines, and we can check
		// for that very quickly, so start with that.
		if bytes.HasPrefix(line, benchmarkPrefix) {
			// At this point we commit to this being a
			// benchmark line. If it's malformed, we treat
			// that as an error.
			r.resultErr = r.parseBenchmarkLine(line)
			return true
		}
		if len(line) > 0 && line[0] == 'U' {
			// Try parsing a unit metadata line.
			if ok, err := r.parseUnitLine(line); ok {
				if err != nil {
					// Report malformed unit line.
					r.resultErr = err
					return true
				}
				continue
			}
		}
		if key, val, ok := parseKeyValueLine(line); ok {
			// Intern key, since there tend to be few
			// unique keys.
			keyStr := r.intern(key)
			if len(val) == 0 {
				r.result.deleteFileConfig(keyStr)
			} else {
				cfg := r.result.ensureFileConfig(keyStr)
				cfg.Value = append(cfg.Value[:0], val...)
			}
			continue
		}
		// Ignore the line.
	}

	if err := r.s.Err(); err != nil {
		r.err = fmt.Errorf("%s:%d: %w", r.fileName, r.lineNum, err)
		return false
	}
	r.err = nil
	return false
}

// parseKeyValueLine attempts to parse line as a key: val pair,
// with ok reporting whether the line could be parsed.
func parseKeyValueLine(line []byte) (key, val []byte, ok bool) {
	for i := 0; i < len(line); {
		r, n := utf8.DecodeRune(line[i:])
		// key begins with a lower case character ...
		if i == 0 && !unicode.IsLower(r) {
			return
		}
		// and contains no space characters nor upper case
		// characters.
		if unicode.IsSpace(r) || unicode.IsUpper(r) {
			return
		}
		if i > 0 && r == ':' {
			key, val = line[:i], line[i+1:]
			break
		}

		i += n
	}
	if len(key) == 0 {
		return
	}
	// Value can be omitted entirely, in which case the colon must
	// still be present, but need not be followed by a space.
	if len(val) == 0 {
		ok = true
		return
	}
	// One or more ASCII space or tab characters separate "key:"
	// from "value."
	for len(val) > 0 && (val[0] == ' ' || val[0] == '\t') {
		val = val[1:]
		ok = true
	}
	return
}

// parseBenchmarkLine parses line as a benchmark result and updates r.result.
// The caller must have already checked that line begins with "Benchmark".
func (r *Reader) parseBenchmarkLine(line []byte) error {
	var f []byte
	var err error

	// Skip "Benchmark"
	line = line[len("Benchmark"):]

	// Read the name.
	r.result.Name, line = splitField(line)

	// Read the iteration count.
	f, line = splitField(line)
	if len(f) == 0 {
		return &SyntaxError{r.fileName, r.lineNum, "missing iteration count"}
	}
	r.result.Iters, err = bytesconv.Atoi(f)
	switch err := err.(type) {
	case nil:
		// ok
	case *bytesconv.NumError:
		return &SyntaxError{r.fileName, r.lineNum, "parsing iteration count: " + err.Err.Error()}
	default:
		return &SyntaxError{r.fileName, r.lineNum, err.Error()}
	}

	// Read value/unit pairs.
	r.result.Values = r.result.Values[:0]
	for {
		f, line = splitField(line)
		if len(f) == 0 {
			if len(r.result.Values) > 0 {
				break
			}
			return &SyntaxError{r.fileName, r.lineNum, "missing measurements"}
		}
		val, err := atof(f)
		switch err := err.(type) {
		case nil:
			// ok
		case *bytesconv.NumError:
			return &SyntaxError{r.fileName, r.lineNum, "parsing measurement: " + err.Err.Error()}
		default:
			return &SyntaxError{r.fileName, r.lineNum, err.Error()}
		}
		f, line = splitField(line)
		if len(f) == 0 {
			return &SyntaxError{r.fileName, r.lineNum, "missing units"}
		}
		unit := r.intern(f)

		// Tidy the value.
		tidyUnit, factor := benchunit.Tidy(unit)
		var v Value
		if factor == 1 {
			v = Value{Value: val, Unit: unit}
		} else {
			v = Value{Value: val * factor, Unit: tidyUnit, OrigValue: val, OrigUnit: unit}
		}

		r.result.Values = append(r.result.Values, v)
	}

	return nil
}

// parseUnitLine attempts to parse line as a unit metadata line,
// with ok reporting whether the line was a unit metadata line.
// If line is a unit metadata line but contains errors,
// it returns true and a *SyntaxError.
func (r *Reader) parseUnitLine(line []byte) (ok bool, err error) {
	var f []byte
	// Is this a unit metadata line?
	f, line = splitField(line)
	if !bytes.Equal(f, unitPrefix) {
		return false, nil
	}

	// Consume unit.
	f, line = splitField(line)
	if len(f) == 0 {
		return true, &SyntaxError{r.fileName, r.lineNum, "missing unit"}
	}
	unit := r.intern(f)

	// Consume key=value pairs. If we encounter any errors, we
	// report the first one but keep trying to parse the line.
	for {
		f, line = splitField(line)
		if len(f) == 0 {
			break
		}
		eq := bytes.IndexByte(f, '=')
		if eq <= 0 {
			if err == nil {
				err = &SyntaxError{r.fileName, r.lineNum, "expected key=value"}
			}
			continue
		}
		key := r.intern(f[:eq])
		value := r.intern(f[eq+1:])
		if err1 := r.result.Units.Set(unit, key, value); err1 != nil && err == nil {
			err = &SyntaxError{r.fileName, r.lineNum, err1.Error()}
		}
	}
	return true, err
}

func (r *Reader) intern(x []byte) string {
	const maxIntern = 1024
	if s, ok := r.interns[string(x)]; ok {
		return s
	}
	if len(r.interns) >= maxIntern {
		// Evict a random item from the interns table.
		for k := range r.interns {
			delete(r.interns, k)
			break
		}
	}
	s := string(x)
	r.interns[s] = s
	return s
}

// Result returns the last result read, or an error if the result was
// malformed.
//
// Parse errors are non-fatal, so the caller can continue to call
// Scan.
//
// The caller should not retain the Result object, as it will be
// overwritten by the next call to Scan.
func (r *Reader) Result() (*Result, error) {
	if r.resultErr != nil {
		return nil, r.resultErr
	}
	return &r.result, nil
}

// Err returns the first non-EOF I/O error that was encountered by the
// Reader.
func (r *Reader) Err() error {
	return r.err
}

// Units returns the latest unit metadata.
//
// This is useful for consumers that wish to consume an entire stream
// of benchmark results and then consult unit metadata. Unit metadata
// can change between the last result and EOF, so this may differ from
// the last Result().Units after Scan returns false.
func (r *Reader) Units() Units {
	return r.result.Units
}

// Parsing helpers.
//
// These are designed to leverage common fast paths. The ASCII fast
// path is especially important, and more than doubles the performance
// of the parser.

// atof is a wrapper for bytesconv.ParseFloat that optimizes for
// numbers that are usually integers.
func atof(x []byte) (float64, error) {
	// Try parsing as an integer.
	var val int64
	for _, ch := range x {
		digit := ch - '0'
		if digit >= 10 {
			goto fail
		}
		if val > (math.MaxInt64-10)/10 {
			goto fail // avoid int64 overflow
		}
		val = (val * 10) + int64(digit)
	}
	return float64(val), nil

fail:
	// The fast path failed. Parse it as a float.
	return bytesconv.ParseFloat(x, 64)
}

const isSpace uint64 = 1<<'\t' | 1<<'\n' | 1<<'\v' | 1<<'\f' | 1<<'\r' | 1<<' '

// splitField consumes and returns non-whitespace in x as field,
// consumes whitespace following the field, and then returns the
// remaining bytes of x.
func splitField(x []byte) (field, rest []byte) {
	// Collect non-whitespace into field.
	var i int
	for i = 0; i < len(x); {
		if x[i] < utf8.RuneSelf {
			// Fast path for ASCII
			if (isSpace>>x[i])&1 != 0 {
				rest = x[i+1:]
				break

			}
			i++
		} else {
			// Slow path for Unicode
			r, n := utf8.DecodeRune(x[i:])
			if unicode.IsSpace(r) {
				rest = x[i+n:]
				break
			}
			i += n
		}
	}
	field = x[:i]

	// Strip whitespace from rest.
	for len(rest) > 0 {
		if rest[0] < utf8.RuneSelf {
			if (isSpace>>rest[0])&1 == 0 {
				break
			}
			rest = rest[1:]
		} else {
			r, n := utf8.DecodeRune(rest)
			if !unicode.IsSpace(r) {
				break
			}
			rest = rest[n:]
		}
	}
	return
}
