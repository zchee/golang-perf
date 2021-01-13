// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

// A ConfigHeader is a node in a Config header tree. It represents a
// subslice of a slice of Configs that are all equal up to some
// prefix.
//
// Specifically, given a Config slice configs and ConfigHeader node n,
// configs[n.Start:n.Start+n.Len] are equal for all fields from 0 to
// n.Field.
type ConfigHeader struct {
	// Field is the index of the Schema field represented by this
	// node.
	Field int

	// Start is the index of the first Config covered by this
	// node.
	Start int
	// Len is the number of Configs in the sequence represented by
	// this node. Visually, this is also the cell span of this
	// node.
	Len int

	// Value is the value that all Configs have in common for
	// Field.
	Value string
}

// NewConfigHeader combines a sequence of Configs by common prefixes.
//
// This is intended to visually present a sequence of Configs in a
// compact form; for example, as a header over a table where each
// column is keyed by a Config.
//
// For example, given four configs:
//
//   a:1 b:1 c:1
//   a:1 b:1 c:2
//   a:2 b:2 c:2
//   a:2 b:3 c:3
//
// NewConfigHeader will form the following levels, where each cell is
// a *ConfigHeader:
//
//           +-----------+-----------+
//   Level 0 |    a:1    |    a:2    |
//           +-----------+-----+-----+
//   Level 1 |    b:1    | b:2 | b:3 |
//           +-----+-----+-----+-----+
//   Level 2 | c:1 | c:2 | c:2 | c:3 |
//           +-----+-----+-----+-----+
//
// All Configs must have the same Schema. In the result, levels[i]
// corresponds to field i of this Schema. In each levels[i], the
// ConfigHeader nodes partition the whole configs slice. Furthermore,
// each level is a stricter partitioning than the previous level, so
// the ConfigHeaders logically form a tree.
func NewConfigHeader(configs []Config) (levels [][]*ConfigHeader) {
	if len(configs) == 0 {
		return nil
	}

	fields := commonSchema(configs).Fields()

	levels = make([][]*ConfigHeader, len(fields))
	prevLevel := []*ConfigHeader{&ConfigHeader{-1, 0, len(configs), ""}}
	// Walk through the levels of the tree, subdividing the nodes
	// from the previous level.
	for i, field := range fields {
		for _, parent := range prevLevel {
			var node *ConfigHeader
			for j, config := range configs[parent.Start : parent.Start+parent.Len] {
				val := config.Get(field)
				if node != nil && val == node.Value {
					node.Len++
				} else {
					node = &ConfigHeader{i, parent.Start + j, 1, val}
					levels[i] = append(levels[i], node)
				}
			}
		}
		prevLevel = levels[i]
	}
	return levels
}
