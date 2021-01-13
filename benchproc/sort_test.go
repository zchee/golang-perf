// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestSort(t *testing.T) {
	check := func(cfgs []Config, want ...string) {
		SortConfigs(cfgs)
		var got []string
		for _, cfg := range cfgs {
			got = append(got, cfg.String())
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	}

	// Observation order.
	s, _ := mustParse(t, "a")
	c := []Config{
		p(t, s, "", "a", "1"),
		p(t, s, "", "a", "3"),
		p(t, s, "", "a", "2"),
	}
	check(c, "a:1", "a:3", "a:2")

	// Tuple observation order.
	s, _ = mustParse(t, "a,b")
	// Prepare order.
	p(t, s, "", "a", "1")
	p(t, s, "", "a", "2")
	p(t, s, "", "b", "1")
	p(t, s, "", "b", "2")
	c = []Config{
		p(t, s, "", "a", "2", "b", "1"),
		p(t, s, "", "a", "1", "b", "2"),
	}
	check(c, "a:1 b:2", "a:2 b:1")

	// Alphabetic
	s, _ = mustParse(t, "a@alpha")
	c = []Config{
		p(t, s, "", "a", "c"),
		p(t, s, "", "a", "b"),
		p(t, s, "", "a", "a"),
	}
	check(c, "a:a", "a:b", "a:c")

	// Numeric.
	s, _ = mustParse(t, "a@num")
	c = []Config{
		p(t, s, "", "a", "100"),
		p(t, s, "", "a", "20"),
		p(t, s, "", "a", "3"),
	}
	check(c, "a:3", "a:20", "a:100")

	// Numeric with strings.
	s, _ = mustParse(t, "a@num")
	c = []Config{
		p(t, s, "", "a", "b"),
		p(t, s, "", "a", "a"),
		p(t, s, "", "a", "100"),
		p(t, s, "", "a", "20"),
	}
	check(c, "a:20", "a:100", "a:a", "a:b")

	// Numeric with weird cases.
	s, _ = mustParse(t, "a@num")
	c = []Config{
		p(t, s, "", "a", "1"),
		p(t, s, "", "a", "-inf"),
		p(t, s, "", "a", "-infinity"),
		p(t, s, "", "a", "inf"),
		p(t, s, "", "a", "infinity"),
		p(t, s, "", "a", "1.0"),
		p(t, s, "", "a", "NaN"),
		p(t, s, "", "a", "nan"),
	}
	// Shuffle the slice to exercise any instabilities.
	for try := 0; try < 10; try++ {
		for i := 1; i < len(c); i++ {
			p := rand.Intn(i)
			c[p], c[i] = c[i], c[p]
		}
		check(c, "a:-inf", "a:-infinity", "a:1", "a:1.0", "a:inf", "a:infinity", "a:NaN", "a:nan")
	}

	// Fixed.
	s, _ = mustParse(t, "a@(c b a)")
	c = []Config{
		p(t, s, "", "a", "a"),
		p(t, s, "", "a", "b"),
		p(t, s, "", "a", "c"),
	}
	check(c, "a:c", "a:b", "a:a")
}

func TestParseNum(t *testing.T) {
	check := func(x string, want float64) {
		t.Helper()
		got, err := parseNum(x)
		if err != nil {
			t.Errorf("%s: want %v, got error %s", x, want, err)
		} else if want != got {
			t.Errorf("%s: want %v, got %v", x, want, got)
		}
	}

	check("1", 1)
	check("1B", 1)
	check("1b", 1)
	check("100.5", 100.5)
	check("1k", 1000)
	check("1K", 1000)
	check("1ki", 1024)
	check("1kiB", 1024)
	check("1M", 1000000)
	check("1Mi", 1<<20)
	check("1G", 1000000000)
	check("1T", 1000000000000)
	check("1P", 1000000000000000)
	check("1E", 1000000000000000000)
	check("1Z", 1000000000000000000000)
	check("1Y", 1000000000000000000000000)
}
