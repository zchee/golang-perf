// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDoc(t *testing.T) {
	// Test examples used in the command documentation.
	// If any of these change, be sure to update the
	// documentation.

	golden(t, "docOldNew", "old.txt", "new.txt")

	golden(t, "docColFormat", "-col", "/format", "new.txt")
	golden(t, "docColFormat2", "-col", "/format", "-row", ".name", "-ignore", ".label", "new.txt")
	golden(t, "docLabels", "A=old.txt", "B=new.txt")
	golden(t, "docRowName", "-row", ".name", "new.txt")

	golden(t, "docSorting", "-col", "/format@(gob json)", "-row", ".name", "-ignore", ".label", "new.txt")
}

func TestCSV(t *testing.T) {
	golden(t, "csvOldNew", "-format", "csv", "old.txt", "new.txt")
	golden(t, "csvErrors", "-format", "csv", "-row", ".name", "new.txt")
}

func TestCRC(t *testing.T) {
	// These have a "note" that "unexpectedly" splits the tables,
	// and also two units.
	golden(t, "crcOldNew", "crc-old.txt", "crc-new.txt")
	// "Fix" the split by note.
	golden(t, "crcIgnore", "-ignore", "note", "crc-old.txt", "crc-new.txt")

	// Filter to aligned, put size on the X axis and poly on the Y axis.
	golden(t, "crcSizeVsPoly", "-filter", "/align:0", "-row", "/size", "-col", "/poly", "crc-new.txt")
}

func TestUnits(t *testing.T) {
	// Test unit metadata. This tests exact assumptions and
	// warnings for inexact distributions.
	golden(t, "units", "-col", "note", "units.txt")
}

func TestZero(t *testing.T) {
	// Test printing of near-zero deltas.
	golden(t, "zero", "-col", "note", "zero.txt")
}

func TestSmallSample(t *testing.T) {
	// These benchmarks don't have enough samples to compute a CI
	// or delta.
	golden(t, "smallSample", "-col", "note", "-ignore", ".label", "smallSample.txt")
}

func TestIssue19565(t *testing.T) {
	// Benchmark sets are inconsistent between columns. We show
	// all results, but warn that the geomeans may not be
	// comparable.
	golden(t, "issue19565", "-col", "note", "-ignore", ".label", "issue19565.txt")
}

func TestIssue19634(t *testing.T) {
	golden(t, "issue19634", "-col", "note", "-ignore", ".label", "issue19634.txt")
}

func golden(t *testing.T, name string, args ...string) {
	t.Helper()
	// TODO: If benchfmt.Files supported fs.FS, we wouldn't need this.
	if err := os.Chdir("testdata"); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir("..")

	// Get the benchstat output.
	var got, gotErr bytes.Buffer
	t.Logf("benchstat %s", strings.Join(args, " "))
	if err := benchstat(&got, &gotErr, args); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Compare to the golden output.
	compare(t, name, "stdout", got.Bytes())
	compare(t, name, "stderr", gotErr.Bytes())
}

func compare(t *testing.T, name, sub string, got []byte) {
	t.Helper()

	wantPath := name + "." + sub
	want, err := ioutil.ReadFile(wantPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Treat a missing file as empty.
			want = nil
		} else {
			t.Fatal(err)
		}
	}

	if bytes.Equal(want, got) {
		return
	}

	// Write a "got" file for reference.
	gotPath := name + ".got-" + sub
	if err := ioutil.WriteFile(gotPath, got, 0666); err != nil {
		t.Fatalf("error writing %s: %s", gotPath, err)
	}

	data, err := exec.Command("diff", "-Nu", wantPath, gotPath).CombinedOutput()
	if len(data) > 0 {
		t.Errorf("diff -Nu %s %s:\n%s", wantPath, gotPath, string(data))
		return
	}
	// Most likely, "diff not found" so print the bad output so there is something.
	t.Errorf("want:\n%sgot:\n%s", string(want), string(got))
}
