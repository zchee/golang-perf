// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"fmt"
	"hash/maphash"
	"strings"

	"golang.org/x/perf/benchfmt"
	"golang.org/x/perf/benchproc/internal/parse"
)

// TODO: If we support comparison operators in filter expressions,
// does it make sense to unify the orders understood by projections
// with the comparison orders supported in filters? One danger is that
// the default order for projections is observation order, but if you
// filter on key<val, you probably want that to be numeric by default
// (it's not clear you ever want a comparison on observation order).

// A ProjectionParser parses projection expressions, which describe
// how to extract components of a benchfmt.Result into a Config and
// how to order the resulting Configs.
//
// If multiple projections are parsed by one ProjectionParser, they
// form a mutually-exclusive group of projections in which specific
// keys in any projection are excluded from group keys in any other
// projection. The group keys for file and per-benchmark configuration
// are ".config" and ".fullname", respectively. For example, given two
// projections ".config" and "commit,date", the specific file
// configuration keys "commit" and "date" are excluded from the group
// key ".config".
type ProjectionParser struct {
	configKeys   map[string]bool // Specific .config keys (excluded from .config)
	fullnameKeys []string        // Specific sub-name keys (excluded from .fullname)
	haveConfig   bool            // .config was projected
	haveFullname bool            // .fullname was projected

	// Fields below here are constructed when the first Result is
	// processed.

	fullExtractor extractor
}

// Parse parses a single projection expression, such as ".name,/size".
// See "go doc golang.org/x/perf/benchproc/syntax" for a description
// of projection syntax.
//
// If the projection expression contains any fixed orders that imply a
// filter, Parse will add these filters to "filter".
func (p *ProjectionParser) Parse(proj string, filter *Filter) (*Schema, error) {
	if p.configKeys == nil {
		p.configKeys = make(map[string]bool)
	}

	s := newSchema()

	// Parse the projection.
	parts, err := parse.ParseProjection(proj)
	if err != nil {
		return nil, err
	}
	var filterParts []filterFn
	for _, part := range parts {
		f, err := p.makeProjection(s, proj, part)
		if err != nil {
			return nil, err
		}
		if f != nil {
			filterParts = append(filterParts, f)
		}
	}
	// Now that we've ensured the projection is valid, add any
	// filter parts to the filter.
	if len(filterParts) > 0 {
		filterParts = append(filterParts, filter.match)
		filter.match = filterOp(parse.OpAnd, filterParts)
	}

	return s, nil
}

// Residue returns a projection for any keys not yet projected by any
// parsed projection. The resulting Schema does not have a meaningful
// order.
func (p *ProjectionParser) Residue() *Schema {
	s := newSchema()

	// The .config and .fullname groups together cover the
	// projection space. If they haven't already been specified,
	// then these groups (with any specific keys excluded) exactly
	// form the remainder.
	if !p.haveConfig {
		p.makeProjection(s, "", parse.Projection{Key: ".config", Order: "first"})
	}
	if !p.haveFullname {
		p.makeProjection(s, "", parse.Projection{Key: ".fullname", Order: "first"})
	}

	return s
}

func (p *ProjectionParser) makeProjection(s *Schema, q string, proj parse.Projection) (filterFn, error) {
	// Construct the order function.
	var initField func(field Field)
	var filter filterFn
	makeFilter := func(ext extractor) {}
	if proj.Order == "fixed" {
		fixedMap := make(map[string]int, len(proj.Fixed))
		for i, s := range proj.Fixed {
			fixedMap[s] = i
		}
		initField = func(field Field) {
			field.cmp = func(a, b string) int {
				return fixedMap[a] - fixedMap[b]
			}
		}
		makeFilter = func(ext extractor) {
			filter = func(res *benchfmt.Result) (mask, bool) {
				_, ok := fixedMap[string(ext(res))]
				return nil, ok
			}
		}
	} else if proj.Order == "first" {
		initField = func(field Field) {
			field.order = make(map[string]int)
			field.cmp = func(a, b string) int {
				return field.order[a] - field.order[b]
			}
		}
	} else if cmp, ok := builtinOrders[proj.Order]; ok {
		initField = func(field Field) {
			field.cmp = cmp
		}
	} else {
		return nil, &parse.SyntaxError{q, proj.OrderOff, fmt.Sprintf("unknown order %q", proj.Order)}
	}

	var project func(*benchfmt.Result, *[]string)
	switch proj.Key {
	case ".config":
		// File configuration, excluding any more
		// specific file keys.
		if proj.Order == "fixed" {
			// Fixed orders don't make sense for a whole tuple.
			return nil, &parse.SyntaxError{q, proj.OrderOff, fmt.Sprintf("fixed order not allowed for .config")}
		}

		p.haveConfig = true
		group := s.addGroup(s.root, ".config")
		seen := make(map[string]Field)
		project = func(r *benchfmt.Result, row *[]string) {
			for _, cfg := range r.FileConfig {
				field, ok := seen[cfg.Key]
				if !ok {
					if p.configKeys[cfg.Key] {
						continue
					}
					field = s.addField(group, cfg.Key)
					initField(field)
					seen[cfg.Key] = field
				}

				(*row)[field.idx] = s.intern(cfg.Value)
			}
		}

	case ".fullname":
		// Full benchmark name, including name config.
		// We want to exclude any more specific keys,
		// including keys from later projections, so
		// we delay constructing the extractor until
		// we process the first Result.
		//
		// TODO: It's possible we need to just remove excluded
		// keys entirely from the name, rather than rewriting
		// to /x=*, since that still distinguishes between
		// present and missing keys.
		p.haveFullname = true
		field := s.addField(s.root, ".fullname")
		initField(field)
		makeFilter(extractFull)

		project = func(r *benchfmt.Result, row *[]string) {
			if p.fullExtractor == nil {
				p.fullExtractor = newExtractorFullName(p.fullnameKeys)
			}
			val := p.fullExtractor(r)
			(*row)[field.idx] = s.intern(val)
		}

	default:
		// This is a specific sub-name or file key. Add it
		// to the excludes.
		if proj.Key == ".name" || strings.HasPrefix(proj.Key, "/") {
			p.fullnameKeys = append(p.fullnameKeys, proj.Key)
		} else {
			p.configKeys[proj.Key] = true
		}
		ext, err := newExtractor(proj.Key)
		if err != nil {
			return nil, &parse.SyntaxError{q, proj.KeyOff, err.Error()}
		}
		field := s.addField(s.root, proj.Key)
		initField(field)
		makeFilter(ext)
		project = func(r *benchfmt.Result, row *[]string) {
			val := ext(r)
			(*row)[field.idx] = s.intern(val)
		}
	}
	s.project = append(s.project, project)
	return filter, nil
}

// A Schema projects some subset of the components in a
// benchmark.Result into a Config. All Configs produced by a Schema
// have the same structure. Configs produced by a Schema will be == if
// they have the same values (notably, this means Configs can be used
// as map keys). A Schema also implies a sort order, which is
// lexicographic based on the order of fields in the Schema, with the
// order of each individual field determined by the projection.
type Schema struct {
	root    Field
	nFields int

	// unitField, if non-nil, is the ".unit" field used to project
	// the values of a benchmark result.
	unitField Field

	// flatCache, if non-nil, contains the flattened sequence of
	// fields.
	flatCache []Field

	// project is a set of functions that project a Result into
	// row.
	//
	// These take a pointer to row because these functions may
	// grow the schema, so the row slice may grow.
	project []func(r *benchfmt.Result, row *[]string)

	// row is the buffer used to construct a projection.
	row []string

	// interns is used to intern []byte to string. These are
	// always referenced in Configs, so this doesn't cause any
	// over-retention.
	interns map[string]string

	// configs are the interned Configs of this Schema.
	configs map[uint64][]*configNode
}

func newSchema() *Schema {
	var s Schema
	s.root.fieldInternal = &fieldInternal{idx: -1}
	s.interns = make(map[string]string)
	s.configs = make(map[uint64][]*configNode)
	return &s
}

func (s *Schema) addField(group Field, name string) Field {
	if group.idx != -1 {
		panic("field's parent is not a group")
	}

	// Assign this field an index.
	field := Field{name, &fieldInternal{schema: s, idx: s.nFields}}
	s.nFields++
	group.sub = append(group.sub, field)
	// Add to the row buffer.
	s.row = append(s.row, "")
	// Clear the current flattening.
	s.flatCache = nil
	return field
}

func (s *Schema) addGroup(group Field, name string) Field {
	field := Field{name, &fieldInternal{schema: s, idx: -1}}
	group.sub = append(group.sub, field)
	return field
}

// AddValues appends a field to this Schema called ".unit" used to
// project out each distinct benchfmt.Value in a benchfmt.Result.
//
// For Schemas that have a .unit field, callers should use
// ProjectValues instead of Project.
//
// Typically, callers need to break out individual benchmark values on
// some dimension of a set of Schemas. Adding a .unit field makes this
// easy.
func (s *Schema) AddValues() Field {
	if s.unitField.fieldInternal != nil {
		panic("Schema already has a .unit field")
	}
	field := s.addField(s.root, ".unit")
	field.order = make(map[string]int)
	field.cmp = func(a, b string) int {
		return field.order[a] - field.order[b]
	}
	s.unitField = field
	return field
}

// Fields returns the fields of s in the order determined by the
// Schema's projection expression. Group projections can result in
// zero or more fields. Calling s.Project can cause more fields to be
// added to s (for example, if the Result has a new file configuration
// key).
//
// The caller must not modify the returned slice.
func (s *Schema) Fields() []Field {
	if s.flatCache != nil {
		return s.flatCache
	}

	s.flatCache = make([]Field, 0, s.nFields)
	var walk func(f Field)
	walk = func(f Field) {
		if f.idx != -1 {
			s.flatCache = append(s.flatCache, f)
		} else {
			for _, sub := range f.sub {
				walk(sub)
			}
		}
	}
	walk(s.root)
	return s.flatCache
}

// A Field is a single dimension of a Schema.
type Field struct {
	Name string
	*fieldInternal
}

// A fieldInternal is the internal representation of a field or group
// in a Schema.
type fieldInternal struct {
	schema *Schema

	// idx gives the index of this field's values in a configNode.
	// Indexes are assigned sequentially as fields are added,
	// regardless of the order of those fields in the Schema. This
	// allows new fields to be added to a schema without
	// invalidating existing Configs.
	//
	// idx is -1 for group nodes.
	idx int
	sub []Field // sub-nodes for groups

	// cmp is the comparison function for values of this field. It
	// returns <0 if a < b, >0 if a > b, or 0 if a == b or a and b
	// are unorderable.
	cmp func(a, b string) int

	// order, if non-nil, records the observation order of this
	// field.
	order map[string]int
}

// String returns the name of Field f.
func (f Field) String() string {
	return f.Name
}

var configSeed = maphash.MakeSeed()

// Project extracts components from benchmark Result r according to
// Schema s and returns them as an immutable Config.
//
// If this Schema includes a .units field, it will be left as "" in
// the resulting Config. The caller should use ProjectValues instead.
func (s *Schema) Project(r *benchfmt.Result) Config {
	s.populateRow(r)
	return s.internRow()
}

// ProjectValues is like Project, but for each benchmark value of
// r.Values individually. The returned slice corresponds to the
// r.Values slice.
//
// If this Schema includes a .units field, it will differ between
// these Configs. If not, then all of the Configs will be identical
// because the benchmark values vary only on .unit.
func (s *Schema) ProjectValues(r *benchfmt.Result) []Config {
	s.populateRow(r)
	out := make([]Config, len(r.Values))
	if s.unitField.fieldInternal == nil {
		// There's no .unit, so the Configs will all be the same.
		cfg := s.internRow()
		for i := range out {
			out[i] = cfg
		}
		return out
	}
	// Vary the .unit field.
	for i, val := range r.Values {
		s.row[s.unitField.idx] = val.Unit
		out[i] = s.internRow()
	}
	return out
}

func (s *Schema) populateRow(r *benchfmt.Result) {
	// Clear the row buffer.
	for i := range s.row {
		s.row[i] = ""
	}

	// Run the projection functions to fill in row.
	for _, proj := range s.project {
		// proj may add fields and grow row.
		proj(r, &s.row)
	}
}

func (s *Schema) internRow() Config {
	// Hash the configuration. This must be invariant to unused
	// trailing fields: the schema can grow, and if those new
	// fields are later cleared, we want configurations from
	// before the growth to equal configurations from after the
	// growth.
	row := s.row
	for len(row) > 0 && row[len(row)-1] == "" {
		row = row[:len(row)-1]
	}
	var h maphash.Hash
	h.SetSeed(configSeed)
	for _, val := range row {
		h.WriteString(val)
	}
	hash := h.Sum64()

	// Check if we already have this configuration.
	configs := s.configs[hash]
	for _, config := range configs {
		if config.equalRow(row) {
			return Config{config}
		}
	}

	// Update observation orders.
	for _, field := range s.Fields() {
		if field.order == nil {
			// Not tracking observation order for this field.
			continue
		}
		var val string
		if field.idx < len(row) {
			val = row[field.idx]
		}
		if _, ok := field.order[val]; !ok {
			field.order[val] = len(field.order)
		}
	}

	// Save the config.
	config := &configNode{s, append([]string(nil), row...)}
	s.configs[hash] = append(s.configs[hash], config)
	return Config{config}
}

func (s *Schema) intern(b []byte) string {
	if str, ok := s.interns[string(b)]; ok {
		return str
	}
	str := string(b)
	s.interns[str] = str
	return str
}

// A Config is an immutable tuple mapping from Fields to strings whose
// structure is given by a Schema. Two Configs are == if they come
// from the same Schema and have identical values.
type Config struct {
	c *configNode
}

// IsZero reports whether c is a zeroed Config with no schema and no fields.
func (c Config) IsZero() bool {
	return c.c == nil
}

// Get returns the value of Field f in this Config.
//
// It panics if Field f does not come from the same Schema as the
// Config.
func (c Config) Get(f Field) string {
	if c.IsZero() {
		panic("zero Config has no fields")
	}
	if c.c.schema != f.schema {
		panic("Config and Field have different Schemas")
	}
	idx := f.idx
	if idx >= len(c.c.vals) {
		return ""
	}
	return c.c.vals[idx]
}

// Schema returns the Schema describing Config c.
func (c Config) Schema() *Schema {
	if c.IsZero() {
		return nil
	}
	return c.c.schema
}

// String returns Config as a space-separated sequence of key:value
// pairs in schema order.
func (c Config) String() string {
	return c.string(true)
}

// StringValues returns Config as a space-separated sequences of
// values in schema order.
func (c Config) StringValues() string {
	return c.string(false)
}

func (c Config) string(keys bool) string {
	if c.IsZero() {
		return "<zero>"
	}
	buf := new(strings.Builder)
	for _, field := range c.c.schema.Fields() {
		if field.idx >= len(c.c.vals) {
			continue
		}
		val := c.c.vals[field.idx]
		if val == "" {
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		if keys {
			buf.WriteString(field.Name)
			buf.WriteByte(':')
		}
		buf.WriteString(val)
	}
	return buf.String()
}

// commonSchema returns the Schema that all configs have, or panics if
// any Config has a different Schema. It returns nil if len(configs)
// == 0.
func commonSchema(configs []Config) *Schema {
	if len(configs) == 0 {
		return nil
	}
	s := configs[0].Schema()
	for _, c := range configs[1:] {
		if c.Schema() != s {
			panic("Configs must all have the same Schema")
		}
	}
	return s
}

// configNode is the internal heap-allocated object backing a Config.
// This allows Config itself to be a value type whose equality is
// determined by the pointer equality of the underlying configNode.
type configNode struct {
	schema *Schema
	// vals are the values in this Config, indexed by
	// schemaNode.idx. Trailing ""s are always trimmed.
	//
	// Notably, this is *not* in the order of the flattened
	// schema. This is because fields can be added in the middle
	// of a schema on-the-fly, and we need to not invalidate
	// existing Configs.
	vals []string
}

func (n *configNode) equalRow(row []string) bool {
	if len(n.vals) != len(row) {
		return false
	}
	for i, v := range n.vals {
		if row[i] != v {
			return false
		}
	}
	return true
}
