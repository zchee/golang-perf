// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parse

import (
	"fmt"
	"strings"
)

// A Projection is one component in a projection expression. It
// represents extracting a single dimension of a benchmark result and
// applying an order to it.
type Projection struct {
	Key string

	// Order is the sort order for this field. This can be
	// "first", meaning to sort by order of first appearance;
	// "fixed", meaning to use the explicit value order in Fixed;
	// or a named sort order.
	Order string

	// Fixed gives the explicit value order for "fixed" ordering.
	// If a record's value is not in this list, the record should
	// be filtered out. Otherwise, values should be sorted
	// according to their order in this list.
	Fixed []string

	// KeyOff and OrderOff give the byte offsets of the key and
	// order, for error reporting.
	KeyOff, OrderOff int
}

// String returns Projection as a valid projection expression.
func (p Projection) String() string {
	switch p.Order {
	case "first":
		return quoteWord(p.Key)
	case "fixed":
		words := make([]string, 0, len(p.Fixed))
		for _, word := range p.Fixed {
			words = append(words, quoteWord(word))
		}
		return fmt.Sprintf("%s@(%s)", quoteWord(p.Key), strings.Join(words, " "))
	}
	return fmt.Sprintf("%s@%s", quoteWord(p.Key), quoteWord(p.Order))
}

// ParseProjection parses a projection expression into a tuple of
// Projections.
func ParseProjection(q string) ([]Projection, error) {
	// Parse each projection.
	var projs []Projection
	toks := newTokenizer(q)
	for {
		// Peek at the next token.
		tok, toks2 := toks.key()
		if tok.Kind == 0 {
			// No more projections.
			break
		} else if tok.Kind == ',' && len(projs) > 0 {
			// Consume optional separating comma.
			toks = toks2
		}

		var proj Projection
		proj, toks = parseProjection1(toks)
		projs = append(projs, proj)
	}
	toks.end()
	if toks.errt.err != nil {
		return nil, toks.errt.err
	}
	return projs, nil
}

func parseProjection1(toks tokenizer) (Projection, tokenizer) {
	var p Projection

	// Consume key.
	key, toks2 := toks.key()
	if !(key.Kind == 'w' || key.Kind == 'q') {
		_, toks = toks.error("expected key")
		return p, toks
	}
	toks = toks2
	p.Key = key.Tok
	p.KeyOff = key.Off

	// Consume optional sort order.
	p.Order = "first"
	p.OrderOff = key.Off + len(key.Tok)
	sep, toks2 := toks.key()
	if sep.Kind != '@' {
		// No sort order.
		return p, toks
	}
	toks = toks2

	// Is it a named sort order?
	order, toks2 := toks.key()
	p.OrderOff = order.Off
	if order.Kind == 'w' || order.Kind == 'q' {
		p.Order = order.Tok
		return p, toks2
	}
	// Or a fixed sort order?
	if order.Kind == '(' {
		p.Order = "fixed"
		toks = toks2
		for {
			t, toks2 := toks.key()
			if t.Kind == 'w' || t.Kind == 'q' {
				toks = toks2
				p.Fixed = append(p.Fixed, t.Tok)
			} else if t.Kind == ')' {
				if len(p.Fixed) == 0 {
					_, toks = toks.error("nothing to match")
				} else {
					toks = toks2
				}
				break
			} else {
				_, toks = toks.error("missing )")
				break
			}
		}
		return p, toks
	}
	// Bad sort order syntax.
	_, toks = toks.error("expected named sort order or parenthesized list")
	return p, toks
}
