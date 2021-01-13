// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parse

import "strconv"

// ParseFilter parses a filter expression into a Filter tree.
func ParseFilter(q string) (Filter, error) {
	toks := newTokenizer(q)
	p := parser{}
	query, toks := p.expr(toks)
	toks.end()
	if toks.errt.err != nil {
		return nil, toks.errt.err
	}
	return query, nil
}

type parser struct{}

func (p *parser) error(toks tokenizer, msg string) tokenizer {
	_, toks = toks.error(msg)
	return toks
}

func (p *parser) expr(toks tokenizer) (Filter, tokenizer) {
	var terms []Filter
	for {
		var q Filter
		q, toks = p.andExpr(toks)
		terms = append(terms, q)
		op, toks2 := toks.key()
		if op.Kind != 'O' {
			break
		}
		toks = toks2
	}
	if len(terms) == 1 {
		return terms[0], toks
	}
	return &FilterOp{OpOr, terms}, toks
}

func (p *parser) andExpr(toks tokenizer) (Filter, tokenizer) {
	var q Filter
	q, toks = p.match(toks)
	terms := []Filter{q}
loop:
	for {
		op, toks2 := toks.key()
		switch op.Kind {
		case 'A':
			// "AND" between matches is the same as no
			// operator. Skip.
			toks = toks2
			continue
		case '(', '-', '*', 'w', 'q':
			q, toks = p.match(toks)
			terms = append(terms, q)
		case ')', 'O', 0:
			break loop
		default:
			return nil, p.error(toks, "unexpected "+strconv.Quote(op.Tok))
		}
	}
	if len(terms) == 1 {
		return terms[0], toks
	}
	return &FilterOp{OpAnd, terms}, toks
}

func (p *parser) match(start tokenizer) (Filter, tokenizer) {
	tok, rest := start.key()
	switch tok.Kind {
	case '(':
		q, rest := p.expr(rest)
		op, toks2 := rest.key()
		if op.Kind != ')' {
			return nil, p.error(rest, "missing \")\"")
		}
		return q, toks2
	case '-':
		q, rest := p.match(rest)
		q = &FilterOp{OpNot, []Filter{q}}
		return q, rest
	case '*':
		q := &FilterOp{OpAnd, nil}
		return q, rest
	case 'w', 'q':
		off := tok.Off
		key := tok.Tok
		op, toks2 := rest.key()
		if op.Kind != ':' {
			// TODO: Support other operators
			return nil, p.error(start, "expected key:value")
		}
		rest = toks2
		val, rest := rest.value()
		switch val.Kind {
		default:
			return nil, p.error(start, "expected key:value")
		case 'w', 'q', 'r':
			return p.mkMatch(off, key, val), rest
		case '(':
			var terms []Filter
			for {
				val, toks2 := rest.value()
				switch val.Kind {
				default:
					return nil, p.error(rest, "expected value")
				case ')':
					if len(terms) == 0 {
						return nil, p.error(rest, "nothing to match")
					}
					return &FilterOp{OpOr, terms}, toks2
				case 'w', 'q', 'r':
					terms = append(terms, p.mkMatch(off, key, val))
				}
				rest = toks2
			}
		}
	}
	return nil, p.error(start, "expected key:value or subexpression")
}

func (p *parser) mkMatch(off int, key string, val tok) Filter {
	switch val.Kind {
	case 'w', 'q':
		// Literal match.
		return &FilterMatch{key, nil, val.Tok, off}
	case 'r':
		// Regexp match.
		return &FilterMatch{key, val.Regexp, "", off}
	default:
		panic("non-word token")
	}
}
