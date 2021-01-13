// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

import (
	"reflect"
	"testing"
)

func TestNonSingularFields(t *testing.T) {
	s, _ := mustParse(t, ".config")

	var cfgs []Config
	check := func(want ...string) {
		t.Helper()
		got := NonSingularFields(cfgs)
		var gots []string
		for _, f := range got {
			gots = append(gots, f.Name)
		}
		if !reflect.DeepEqual(want, gots) {
			t.Errorf("want %v, got %v", want, gots)
		}
	}

	cfgs = []Config{}
	check()

	cfgs = []Config{
		p(t, s, "", "a", "1", "b", "1"),
	}
	check()

	cfgs = []Config{
		p(t, s, "", "a", "1", "b", "1"),
		p(t, s, "", "a", "1", "b", "1"),
		p(t, s, "", "a", "1", "b", "1"),
	}
	check()

	cfgs = []Config{
		p(t, s, "", "a", "1", "b", "1"),
		p(t, s, "", "a", "2", "b", "1"),
	}
	check("a")

	cfgs = []Config{
		p(t, s, "", "a", "1", "b", "1"),
		p(t, s, "", "a", "2", "b", "1"),
		p(t, s, "", "a", "1", "b", "2"),
	}
	check("a", "b")
}
