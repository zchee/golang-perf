// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"strings"
	"testing"
)

func checkHeader(t *testing.T, hdr [][]*ConfigHeader, want string) {
	t.Helper()
	got := renderHeader(hdr)
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}

	// Check the structure of the header.
	var width int
	for level, row := range hdr {
		prevEnd := 0
		for _, cell := range row {
			if cell.Field != level {
				t.Errorf("want level %d, got %d", level, cell.Field)
			}
			if cell.Start != prevEnd {
				t.Errorf("want start %d, got %d", prevEnd, cell.Start)
			}
			prevEnd = cell.Start + cell.Len
		}
		if level == 0 {
			width = prevEnd
		} else if width != prevEnd {
			t.Errorf("want width %d, got %d", width, prevEnd)
		}
	}
}

func renderHeader(hdr [][]*ConfigHeader) string {
	buf := new(strings.Builder)
	for _, row := range hdr {
		buf.WriteByte('\n')
		for i, cell := range row {
			if i > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(cell.Value)
			for j := 1; j < cell.Len; j++ {
				buf.WriteString(" --")
			}
		}
	}
	return buf.String()
}

func TestConfigHeader(t *testing.T) {
	// Test basic merging.
	t.Run("basic", func(t *testing.T) {
		s, _ := mustParse(t, ".config")
		c1 := p(t, s, "", "a", "a1", "b", "b1")
		c2 := p(t, s, "", "a", "a1", "b", "b2")
		hdr := NewConfigHeader([]Config{c1, c2})
		checkHeader(t, hdr, `
a1 --
b1 b2`)
	})

	// Test that higher level differences prevent lower levels
	// from being merged, even if the lower levels match.
	t.Run("noMerge", func(t *testing.T) {
		s, _ := mustParse(t, ".config")
		c1 := p(t, s, "", "a", "a1", "b", "b1")
		c2 := p(t, s, "", "a", "a2", "b", "b1")
		hdr := NewConfigHeader([]Config{c1, c2})
		checkHeader(t, hdr, `
a1 a2
b1 b1`)
	})

	// Test mismatched tuple lengths.
	t.Run("missingValues", func(t *testing.T) {
		s, _ := mustParse(t, ".config")
		c1 := p(t, s, "", "a", "a1")
		c2 := p(t, s, "", "a", "a1", "b", "b1")
		c3 := p(t, s, "", "a", "a1", "b", "b1", "c", "c1")
		hdr := NewConfigHeader([]Config{c1, c2, c3})
		checkHeader(t, hdr, `
a1 -- --
 b1 --
  c1`)
	})

	// Test no configs.
	t.Run("none", func(t *testing.T) {
		hdr := NewConfigHeader([]Config{})
		if hdr != nil {
			t.Fatalf("wanted nil, got %v", hdr)
		}
	})

	// Test empty configs.
	t.Run("empty", func(t *testing.T) {
		s, _ := mustParse(t, ".config")
		c1 := p(t, s, "")
		c2 := p(t, s, "")
		hdr := NewConfigHeader([]Config{c1, c2})
		checkHeader(t, hdr, "")
	})
}
