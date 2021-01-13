// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Less reports whether c comes before o in the sort order implied by
// their schema. It panics if c and o have different schemas.
func (c Config) Less(o Config) bool {
	if c.c.schema != o.c.schema {
		panic("cannot compare Configs from different Schemas")
	}
	return less(c.c.schema.Fields(), c.c.vals, o.c.vals)
}

func less(flat []Field, a, b []string) bool {
	// Walk the tuples in schema order.
	for _, node := range flat {
		var aa, bb string
		if node.idx < len(a) {
			aa = a[node.idx]
		}
		if node.idx < len(b) {
			bb = b[node.idx]
		}
		if aa != bb {
			cmp := node.cmp(aa, bb)
			if cmp != 0 {
				return cmp < 0
			}
			// The values are equal/unordered according to
			// the comparison function, but the strings
			// differ. Because Configs are only == if
			// their string representations are ==, this
			// means we have to fall back to a secondary
			// comparison that is only == if the strings
			// are ==.
			return aa < bb
		}
	}

	// Tuples are equal.
	return false
}

// SortConfigs sorts a slice of Configs using Config.Less.
// All configs must have the same Schema.
//
// This is equivalent to using Config.Less with the sort package but
// more efficient.
func SortConfigs(configs []Config) {
	// Check all the schemas so we don't have to do this on every
	// comparison.
	if len(configs) == 0 {
		return
	}
	s := commonSchema(configs)
	flat := s.Fields()

	sort.Slice(configs, func(i, j int) bool {
		return less(flat, configs[i].c.vals, configs[j].c.vals)
	})
}

// builtinOrders is the built-in comparison functions.
var builtinOrders = map[string]func(a, b string) int{
	"alpha": func(a, b string) int {
		return strings.Compare(a, b)
	},
	"num": func(a, b string) int {
		aa, erra := parseNum(a)
		bb, errb := parseNum(b)
		if erra == nil && errb == nil {
			// Sort numerically, and put NaNs after other
			// values.
			if aa < bb || (!math.IsNaN(aa) && math.IsNaN(bb)) {
				return -1
			}
			if aa > bb || (math.IsNaN(aa) && !math.IsNaN(bb)) {
				return 1
			}
			// The values are unordered.
			return 0
		}
		if erra != nil && errb != nil {
			// The values are unordered.
			return 0
		}
		// Put floats before non-floats.
		if erra == nil {
			return -1
		}
		return 1
	},
}

const numPrefixes = `KMGTPEZY`

var numRe = regexp.MustCompile(`([0-9.]+)([k` + numPrefixes + `]i?)?[bB]?`)

// parseNum is a fuzzy number parser. It supports common patterns,
// such as SI prefixes.
func parseNum(x string) (float64, error) {
	// Try parsing as a regular float.
	v, err := strconv.ParseFloat(x, 64)
	if err == nil {
		return v, nil
	}

	// Try a suffixed number.
	subs := numRe.FindStringSubmatch(x)
	if subs != nil {
		v, err := strconv.ParseFloat(subs[1], 64)
		if err == nil {
			exp := 0
			if len(subs[2]) > 0 {
				pre := subs[2][0]
				if pre == 'k' {
					pre = 'K'
				}
				exp = 1 + strings.IndexByte(numPrefixes, pre)
			}
			iec := strings.HasSuffix(subs[2], "i")
			if iec {
				return v * math.Pow(1024, float64(exp)), nil
			}
			return v * math.Pow(1000, float64(exp)), nil
		}
	}

	return 0, strconv.ErrSyntax
}
