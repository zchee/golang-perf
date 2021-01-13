// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"reflect"
	"testing"

	"golang.org/x/perf/benchfmt"
	"golang.org/x/perf/benchproc/internal/parse"
)

// mustParse parses a single projection to a Schema.
func mustParse(t *testing.T, proj string) (*Schema, *Filter) {
	f, err := NewFilter("*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, err := (&ProjectionParser{}).Parse(proj, f)
	if err != nil {
		t.Fatalf("unexpected error parsing %q: %v", proj, err)
	}
	return s, f
}

// r constructs a benchfmt.Result with the given full name and file
// config, which is specified as alternating key/value pairs. The
// result has 1 iteration and no values.
func r(t *testing.T, fullName string, fileConfig ...string) *benchfmt.Result {
	res := &benchfmt.Result{
		Name:  benchfmt.Name(fullName),
		Iters: 1,
	}

	if len(fileConfig)%2 != 0 {
		t.Fatal("fileConfig must be alternating key/value pairs")
	}
	for i := 0; i < len(fileConfig); i += 2 {
		cfg := benchfmt.Config{Key: fileConfig[i], Value: []byte(fileConfig[i+1])}
		res.FileConfig = append(res.FileConfig, cfg)
	}

	return res
}

// p constructs a benchfmt.Result like r, then projects it using s.
func p(t *testing.T, s *Schema, fullName string, fileConfig ...string) Config {
	res := r(t, fullName, fileConfig...)
	return s.Project(res)
}

func fieldNames(s *Schema) []string {
	var names []string
	for _, f := range s.Fields() {
		names = append(names, f.Name)
	}
	return names
}

func TestProjectionBasic(t *testing.T) {
	check := func(cfg Config, want string) {
		t.Helper()
		got := cfg.String()
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}

	var s *Schema

	// Sub-name config.
	s, _ = mustParse(t, ".fullname")
	check(p(t, s, "Name/a=1"), ".fullname:Name/a=1")
	s, _ = mustParse(t, "/a")
	check(p(t, s, "Name/a=1"), "/a:1")
	s, _ = mustParse(t, ".name")
	check(p(t, s, "Name/a=1"), ".name:Name")

	// Fixed file config.
	s, _ = mustParse(t, "a")
	check(p(t, s, "", "a", "1", "b", "2"), "a:1")
	check(p(t, s, "", "b", "2"), "") // Missing values are omitted
	check(p(t, s, "", "a", "", "b", "2"), "")

	// Variable file config.
	s, _ = mustParse(t, ".config")
	check(p(t, s, "", "a", "1", "b", "2"), "a:1 b:2")
	check(p(t, s, "", "c", "3"), "c:3")
	check(p(t, s, "", "c", "3", "a", "2"), "a:2 c:3")
}

func TestProjectionIntern(t *testing.T) {
	s, _ := mustParse(t, "a,b")

	c12 := p(t, s, "", "a", "1", "b", "2")

	if c12 != p(t, s, "", "a", "1", "b", "2") {
		t.Errorf("Configs should be equal")
	}

	if c12 == p(t, s, "", "a", "1", "b", "3") {
		t.Errorf("Configs should not be equal")
	}

	if c12 != p(t, s, "", "a", "1", "b", "2", "c", "3") {
		t.Errorf("Configs should be equal")
	}
}

func TestProjectionParsing(t *testing.T) {
	// Basic parsing is tested by the syntax package. Here we test
	// additional processing done by this package.

	check := func(proj string, want ...string) {
		t.Helper()
		s, _ := mustParse(t, proj)
		got := fieldNames(s)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got fields %v, want %v", proj, got, want)
		}
	}
	checkErr := func(proj, error string, pos int) {
		t.Helper()
		f, _ := NewFilter("*")
		_, err := (&ProjectionParser{}).Parse(proj, f)
		if se, _ := err.(*parse.SyntaxError); se == nil || se.Msg != error || se.Off != pos {
			t.Errorf("%s: want error %s at %d; got %s", proj, error, pos, err)
		}
	}

	check("a,b,c", "a", "b", "c")
	check("a,.config,c", "a", "c") // Group won't appear in fields list.
	check("a,.fullname,c", "a", ".fullname", "c")
	check("a,.name,c", "a", ".name", "c")
	check("a,/b,c", "a", "/b", "c")

	checkErr("a@foo", "unknown order \"foo\"", 2)

	checkErr(".config@(1 2)", "fixed order not allowed for .config", 8)
}

func TestProjectionFiltering(t *testing.T) {
	_, f := mustParse(t, "a@(a b c)")
	check := func(val string, want bool) {
		t.Helper()
		res := r(t, "", "a", val)
		got := f.Apply(res)
		if want != got {
			t.Errorf("%s: want %v, got %v", val, want, got)
		}
	}
	check("a", true)
	check("b", true)
	check("aa", false)
	check("z", false)
}

func TestProjectionExclusion(t *testing.T) {
	// The underlying name normalization has already been tested
	// thoroughly in benchfmt/extract_test.go, so here we just
	// have to test that it's being plumbed right.

	check := func(cfg Config, want string) {
		t.Helper()
		got := cfg.String()
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}

	// Create the main projection.
	var pp ProjectionParser
	f, _ := NewFilter("*")
	s, err := pp.Parse(".fullname,.config", f)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	// Parse specific keys that should be excluded from fullname
	// and config.
	_, err = pp.Parse(".name,/a,exc", f)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	check(p(t, s, "Name"), ".fullname:*")
	check(p(t, s, "Name/a=1"), ".fullname:*/a=*")
	check(p(t, s, "Name/a=1/b=2"), ".fullname:*/a=*/b=2")

	check(p(t, s, "Name", "exc", "1"), ".fullname:*")
	check(p(t, s, "Name", "exc", "1", "abc", "2"), ".fullname:* abc:2")
	check(p(t, s, "Name", "abc", "2"), ".fullname:* abc:2")
}

func TestProjectionResidue(t *testing.T) {
	check := func(mainProj string, want string) {
		t.Helper()

		// Get the residue of mainProj.
		var pp ProjectionParser
		f, _ := NewFilter("*")
		_, err := pp.Parse(mainProj, f)
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
		s := pp.Residue()

		// Project a test result.
		cfg := p(t, s, "Name/a=1/b=2", "x", "3", "y", "4")
		got := cfg.String()
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}

	// Full residue.
	check("", "x:3 y:4 .fullname:Name/a=1/b=2")
	// Empty residue.
	check(".config,.fullname", "")
	// Partial residues.
	check("x,/a", "y:4 .fullname:Name/a=*/b=2")
	check(".config", ".fullname:Name/a=1/b=2")
	check(".fullname", "x:3 y:4")
	check(".name", "x:3 y:4 .fullname:*/a=1/b=2")
}

func TestProjectionValues(t *testing.T) {
	s, _ := mustParse(t, "x")
	unit := s.AddValues()

	check := func(cfg Config, want, wantUnit string) {
		t.Helper()
		got := cfg.String()
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
		gotUnit := cfg.Get(unit)
		if gotUnit != wantUnit {
			t.Errorf("got unit %s, want %s", gotUnit, wantUnit)
		}
	}

	res := r(t, "Name", "x", "1")
	res.Values = []benchfmt.Value{{Value: 100, Unit: "ns/op"}, {Value: 1.21, Unit: "gigawatts"}}
	cfgs := s.ProjectValues(res)
	if len(cfgs) != len(res.Values) {
		t.Fatalf("got %d configs, want %d", len(cfgs), len(res.Values))
	}

	check(cfgs[0], "x:1 .unit:ns/op", "ns/op")
	check(cfgs[1], "x:1 .unit:gigawatts", "gigawatts")
}
