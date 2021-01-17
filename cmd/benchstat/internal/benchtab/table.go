// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchtab

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"golang.org/x/perf/benchmath"
	"golang.org/x/perf/benchproc"
	"golang.org/x/perf/benchunit"
	"golang.org/x/perf/cmd/benchstat/internal/texttab"
)

// A Table summarizes and compares benchmark results in a 2D grid.
// Each cell summarizes a Sample of results with identical row and
// column Configs. Comparisons are done within each row between the
// Sample in the first column and the Samples in any remaining
// columns.
type Table struct {
	// Opts is the configuration options for this table.
	Opts TableOpts

	// Unit is the benchmark unit of all samples in this Table.
	Unit string

	// Assumption is the distributional assumption used for all
	// samples in this table.
	Assumption benchmath.Assumption

	// Rows and Cols give the sequence of row and column Configs
	// in this table. All row Configs have the same schema and all
	// col Configs have the same schema.
	Rows, Cols []benchproc.Config

	// Cells is the cells in the body of this table. Each key in
	// this map is a pair of some Config from Rows and some Config
	// from Cols. However, not all Pairs may be present in the
	// map.
	Cells map[TableKey]*TableCell

	// Summary is the final row of this table, which gives summary
	// information across all benchmarks in this table. It is
	// keyed by Cols.
	Summary map[benchproc.Config]*TableSummary

	// SummaryLabel is the label for the summary row.
	SummaryLabel string
}

// TableKey is a map key used to index a single cell in a Table.
type TableKey struct {
	Row, Col benchproc.Config
}

// TableCell is a single cell in a Table. It represents a sample of
// benchmark results with the same row and column Config.
type TableCell struct {
	// Sample is the set of benchmark results in this cell.
	Sample *benchmath.Sample

	// Summary is the summary of Sample, as computed by the
	// Table's distributional assumption.
	Summary benchmath.Summary

	// Baseline is the baseline cell used for comparisons with
	// this cell, or nil if there is no comparison. This is the
	// cell in the first column of this cell's row, if any.
	Baseline *TableCell

	// Comparison is the comparison with the Baseline cell, as
	// computed by the Table's distributional assumption. If
	// Baseline is nil, this value is meaningless.
	Comparison benchmath.Comparison
}

// TableSummary summarizes a column of a Table.
type TableSummary struct {
	// HasSummary indicates that Summary is valid.
	HasSummary bool
	// Summary summarizes all of the TableCell.Summary values in
	// this column.
	Summary float64

	// HasRatio indicates that Ratio is valid.
	HasRatio bool
	// Ratio summarizes all of the TableCell.Comparison values in
	// this column.
	Ratio float64

	// Warnings is a list of warnings for this summary cell.
	Warnings []error
}

// RowValues returns the summary values for every sample in row.
//
// This is useful when computing a common scale for a row using
// benchunit.CommonScale.
func (t *Table) RowValues(row benchproc.Config) []float64 {
	var out []float64
	for _, col := range t.Cols {
		cell, ok := t.Cells[TableKey{row, col}]
		if ok {
			out = append(out, cell.Summary.Center)
		}
	}
	return out
}

// ToText renders t to a textual representation, assuming a
// fixed-width font.
func (t *Table) ToText(w io.Writer, color bool) error {
	var o texttab.Table

	// Each logical column expands to centerCols columns, plus
	// deltaCols columns if there's a baseline.
	const labelCols = 1
	const centerCols = 3 // <center ±> <CI> <warnings>
	const deltaCols = 3  // <P%> <(p=0.PPP n=N)> <warnings>

	// startCol returns the index of the first centerCol of
	// logical column exp.
	startCol := func(exp int) int {
		if exp == 0 {
			return labelCols
		}
		// The width of experiment 0 is just centerCols. All
		// later experiments are centerCols+deltaCols.
		return labelCols + centerCols + (exp-1)*(centerCols+deltaCols)
	}

	var warningList []string
	warningSet := make(map[string]int)
	warn := func(msgs ...[]error) {
		var footnotes []string
		for _, msgs1 := range msgs {
			for _, msg := range msgs1 {
				s := msg.Error()
				i, ok := warningSet[s]
				if !ok {
					i = len(warningList)
					warningSet[s] = i
					warningList = append(warningList, s)
				}
				footnotes = append(footnotes, superscript(i+1))
			}
		}
		s := strings.Join(footnotes, " ")
		o.Cell(s)
	}

	// Construct the header.
	//
	// TODO: If this spans the delta column, then the label is
	// sort of floating over a wide set of columns. If we only put
	// it over the center columns, then anything that spans
	// multiple centers looks weird. Maybe we need to mark the
	// boundaries of these cells, e.g., with vertical rules?
	hdr := benchproc.NewConfigHeader(t.Cols)
	rEdge := startCol(len(t.Cols) + 1)
	for _, hdrRow := range hdr {
		o.Row()
		for _, hdrCell := range hdrRow {
			l := startCol(hdrCell.Start)
			r := startCol(hdrCell.Start + hdrCell.Len)
			// Configuration headers can span a lot of
			// columns, so we add a vertical rule to more
			// clearly delineate the columns they span. We
			// also add some space so that each logical
			// column in the rest of the table is better
			// separated.
			o.Col(l).Span(r-l, hdrCell.Value, texttab.Center, texttab.LeftMargin(" │ "))
		}
		// Add a vertical bar down the right side to match the other
		// separators.
		o.Col(rEdge).Cell("", texttab.LeftMargin(" │"))
	}

	// Add the column labels row, set margins, and create stretch
	// columns.
	o.Row()
	for i := range t.Cols {
		l := startCol(i)
		o.Col(l)

		// Show the unit over the center column group, since
		// these are values in that unit.
		o.Span(centerCols, t.Unit, texttab.Center, texttab.LeftMargin(" │ "))

		if i > 0 {
			// All but the first column will have A/B
			// comparisons.
			//
			// Separate center and delta column groups by
			// 2 spaces.
			o.Span(deltaCols, "vs base", texttab.Left, texttab.LeftMargin("  "))
		}

		// Make all of the interior columns in this column
		// group shrink columns, leaving on the leftmost and
		// rightmost to stretch.
		for j := l + 1; j < o.CurCol(); j++ {
			o.SetShrink(j, true)
		}
	}
	o.Col(rEdge).Cell("", texttab.LeftMargin(" │"))

	// Emit measurements.
	unitClass := benchunit.ClassOf(t.Unit)
	for _, row := range t.Rows {
		o.Row()

		// TODO: Should I put each row config value in a
		// column? With the keys as headers?
		o.Cell(row.StringValues())

		// Get a common scalar across this row.
		scalar := benchunit.CommonScale(t.RowValues(row), unitClass)

		for exp, col := range t.Cols {
			cell, ok := t.Cells[TableKey{row, col}]
			if !ok {
				continue
			}

			o.Col(startCol(exp))
			o.Cell(scalar.Format(cell.Summary.Center), texttab.Right)
			// Put ± in the margin so 1) the ±s line up,
			// 2) the geomean value (which doesn't have ±)
			// aligns with the summary column, 3) we can
			// right align the range column.
			o.Cell(cell.Summary.PctRangeString(), texttab.Right, texttab.LeftMargin(" ± "))
			warn(cell.Sample.Warnings, cell.Summary.Warnings)
			if exp > 0 && cell.Baseline != nil {
				d := cell.Comparison.FormatDelta(cell.Baseline.Summary.Center, cell.Summary.Center)
				// TODO: Color the delta for whether
				// it's good or bad.
				o.Cell(d, texttab.Right)
				o.Cell("(" + cell.Comparison.String() + ")")
				warn(cell.Comparison.Warnings)
			}
		}
	}

	// Emit summary row.
	if len(t.Rows) > 1 {
		o.Row()
		o.Cell(t.SummaryLabel)
		for exp, col := range t.Cols {
			tsum, ok := t.Summary[col]
			if !ok {
				continue
			}

			if tsum.HasSummary {
				o.Col(startCol(exp))
				o.Cell(benchunit.Scale(tsum.Summary, unitClass), texttab.Right)
			}
			if exp > 0 {
				o.Col(startCol(exp) + centerCols)
				if tsum.HasRatio {
					o.Cell(fmt.Sprintf("%+.2f%%", (tsum.Ratio-1)*100), texttab.Right)
				} else {
					o.Cell("?")
				}
			}

			o.Col(startCol(exp+1) - 1)
			warn(tsum.Warnings)
		}
	}

	// Emit table.
	if err := o.Format(w); err != nil {
		return err
	}

	// Emit warnings.
	if len(warningList) > 0 {
		for i, msg := range warningList {
			if _, err := fmt.Fprintf(w, "%s %s\n", superscript(i+1), msg); err != nil {
				return err
			}
		}
	}

	return nil
}

var superDigits = []rune("⁰¹²³⁴⁵⁶⁷⁸⁹")

func superscript(i int) string {
	if i == 0 {
		return string(superDigits[0])
	}

	var buf [20]rune
	pos := len(buf)
	for i > 0 && pos > 0 {
		pos--
		buf[pos] = superDigits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}

// ToCSV renders t to CSV format. Warnings are written in text format
// to the "warnings" Writer, and prefixed with spreadsheet-style cell
// references. These references assume the table begins on row
// "startRow".
func (t *Table) ToCSV(o *csv.Writer, startRow int, warnings io.Writer) (rowCount int) {
	const labelCols = 1
	const centerCols = 2 // <center> <CI>
	const deltaCols = 2  // <P%> <(p=0.PPP n=N)>
	startCol := func(exp int) int {
		if exp == 0 {
			// Baseline, so no delta.
			return labelCols
		}
		// Center and delta columns.
		l := labelCols + centerCols + (exp-1)*(centerCols+deltaCols)
		return l
	}
	row := make([]string, startCol(len(t.Cols)))
	row = row[:0]
	clearTo := func(col int) {
		for len(row) < col {
			row = append(row, "")
		}
	}
	emit := func() {
		o.Write(row)
		row = row[:0]
		rowCount++
	}
	warn := func(msgs []error) {
		// Construct a spreadsheet-style cell label.
		colName := make([]byte, 10)
		colNamePos := len(colName)
		for x := len(row); x > 0; {
			colNamePos--
			colName[colNamePos] = 'A' + byte(x%26)
			x /= 26
		}
		if colNamePos == len(colName) {
			colNamePos--
			colName[colNamePos] = 'A'
		}
		colName = colName[colNamePos:]
		// Print warnings.
		for _, msg := range msgs {
			fmt.Fprintf(warnings, "%s%d: %s\n", colName, startRow+rowCount, msg)
		}
	}

	// Emit column configurations header.
	colSchema := t.Cols[0].Schema()
	for _, field := range colSchema.Fields() {
		for exp, cfg := range t.Cols {
			clearTo(startCol(exp))
			row = append(row, cfg.Get(field))
		}
		emit()
	}

	// Emit column headers.
	for exp := range t.Cols {
		clearTo(startCol(exp))
		row = append(row, t.Unit, "CI")
		if exp > 0 {
			row = append(row, "vs base", "P")
		}
	}
	emit()

	// Emit table.
	for _, rowCfg := range t.Rows {
		row = append(row, rowCfg.StringValues())
		for exp, colCfg := range t.Cols {
			cell, ok := t.Cells[TableKey{rowCfg, colCfg}]
			if !ok {
				continue
			}

			clearTo(startCol(exp))
			warn(cell.Sample.Warnings)
			warn(cell.Summary.Warnings)
			row = append(row,
				fmt.Sprint(cell.Summary.Center),
				cell.Summary.PctRangeString(),
			)
			if exp > 0 && cell.Baseline != nil {
				warn(cell.Comparison.Warnings)
				row = append(row,
					cell.Comparison.FormatDelta(cell.Baseline.Summary.Center, cell.Summary.Center),
					cell.Comparison.String(),
				)
			}
		}
		emit()
	}

	// Emit summary row.
	row = append(row, t.SummaryLabel)
	for exp, cfg := range t.Cols {
		tsum, ok := t.Summary[cfg]
		if !ok {
			continue
		}

		clearTo(startCol(exp))
		warn(tsum.Warnings)
		if tsum.HasSummary {
			row = append(row, fmt.Sprint(tsum.Summary))
		}
		if exp > 0 {
			clearTo(startCol(exp) + centerCols)
			if tsum.HasRatio {
				row = append(row, fmt.Sprintf("%+.2f%%", (tsum.Ratio-1)*100))
			} else {
				row = append(row, "?")
			}
		}
	}
	emit()

	return
}
