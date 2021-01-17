// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package benchtab presents benchmark results as comparison tables.
package benchtab

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"

	"github.com/aclements/go-moremath/stats"
	"golang.org/x/perf/benchfmt"
	"golang.org/x/perf/benchmath"
	"golang.org/x/perf/benchproc"
)

// TODO: Color by good/bad (or nothing for unknown units)

// A Builder collects benchmark results into a Tables set.
type Builder struct {
	tableBy, rowBy, colBy *benchproc.Schema
	residue               *benchproc.Schema

	unitField benchproc.Field

	// tables maps from tableBy to table.
	tables map[benchproc.Config]*table
}

type table struct {
	// Observed row and col configs within this group. Within the
	// group, we show only the row and col labels for the data in
	// the group, but we sort them according to the global
	// observation order for consistency across groups.
	rows map[benchproc.Config]struct{}
	cols map[benchproc.Config]struct{}

	// cells maps from (row, col) to each cell.
	cells map[TableKey]*cell
}

type cell struct {
	// values is the observed values in this cell.
	values []float64
	// configs is the set of residue configs mapped to this cell.
	// It is used to check for non-unique keys.
	configs map[benchproc.Config]struct{}
}

// NewBuilder creates a new Builder for collecting benchmark results
// into tables. Each result will be mapped to a Table by tableBy.
// Within each table, the results are mapped to cells by rowBy and
// colBy. Any results within a single cell that vary by residue will
// be reported as warnings.
func NewBuilder(tableBy, rowBy, colBy, residue *benchproc.Schema) *Builder {
	unitField := tableBy.AddValues()
	return &Builder{
		tableBy: tableBy, rowBy: rowBy, colBy: colBy, residue: residue,
		unitField: unitField,
		tables:    make(map[benchproc.Config]*table),
	}
}

// Add adds all of the values in result to the tables in the Builder.
func (b *Builder) Add(result *benchfmt.Result) {
	// Project the result.
	tableCfgs := b.tableBy.ProjectValues(result)
	rowCfg := b.rowBy.Project(result)
	colCfg := b.colBy.Project(result)
	residueCfg := b.residue.Project(result)
	cellCfg := TableKey{rowCfg, colCfg}

	// Map to tables.
	for unitI, tableCfg := range tableCfgs {
		table := b.tables[tableCfg]
		if table == nil {
			table = b.newTable()
			b.tables[tableCfg] = table
		}

		// Map to a cell.
		c := table.cells[cellCfg]
		if c == nil {
			c = new(cell)
			c.configs = make(map[benchproc.Config]struct{})
			table.cells[cellCfg] = c
			table.rows[rowCfg] = struct{}{}
			table.cols[colCfg] = struct{}{}
		}

		// Add to the cell.
		c.values = append(c.values, result.Values[unitI].Value)
		c.configs[residueCfg] = struct{}{}
	}
}

func (b *Builder) newTable() *table {
	return &table{
		rows:  make(map[benchproc.Config]struct{}),
		cols:  make(map[benchproc.Config]struct{}),
		cells: make(map[TableKey]*cell),
	}
}

// TableOpts provides options for constructing the final analysis
// tables from a Builder.
type TableOpts struct {
	// Confidence is the desired confidence level in summary
	// intervals; e.g., 0.95 for 95%.
	Confidence float64

	// Thresholds is the thresholds to use for statistical tests.
	Thresholds *benchmath.Thresholds

	// Units is the unit metadata. This gives distributional
	// assumptions for units, among other properties.
	Units benchfmt.Units
}

// Tables is a sequence of benchmark statistic tables.
type Tables struct {
	// Tables is a slice of statistic tables. Within a table, all
	// results have the same table config (including unit).
	Tables []*Table
	// Configs is a slice of table configs, corresponding 1:1 to
	// the Tables slice. These configs always end with a ".unit"
	// config giving the unit.
	Configs []benchproc.Config
}

// ToTables finalizes a Builder into a sequence of statistic tables.
func (b *Builder) ToTables(opts TableOpts) *Tables {
	// Sort tables.
	var configs []benchproc.Config
	for k := range b.tables {
		configs = append(configs, k)
	}
	benchproc.SortConfigs(configs)

	// We're going to compute table cells in parallel because the
	// statistics are somewhat expensive. This is entirely
	// CPU-bound, so we put a simple concurrency limit on it.
	limit := make(chan struct{}, 2*runtime.GOMAXPROCS(-1))
	var wg sync.WaitGroup

	// Process each table.
	var tables []*Table
	for _, k := range configs {
		cTable := b.tables[k]

		// Get the configured assumption for this unit.
		unit := k.Get(b.unitField)
		var assumption benchmath.Assumption = benchmath.AssumeNothing
		if dist, ok := opts.Units.Get(unit, "assume"); ok && dist == "exact" {
			assumption = benchmath.AssumeExact
		}

		// Sort the rows and columns.
		rowCfgs, colCfgs := mapConfigs(cTable.rows), mapConfigs(cTable.cols)
		table := &Table{
			Unit:       unit,
			Opts:       opts,
			Assumption: assumption,
			Rows:       rowCfgs,
			Cols:       colCfgs,
			Cells:      make(map[TableKey]*TableCell),
		}
		tables = append(tables, table)

		// Create all TableCells and fill their Samples. This
		// is fast enough it's not worth parallelizing. This
		// enables the second pass to look up baselines and
		// their samples.
		for k, cCell := range cTable.cells {
			table.Cells[k] = &TableCell{
				Sample: benchmath.NewSample(cCell.values, opts.Thresholds),
			}
		}

		// Populate cells.
		baselineCfg := colCfgs[0]
		wg.Add(len(cTable.cells))
		for k, cCell := range cTable.cells {
			cell := table.Cells[k]

			// Look up the baseline.
			if k.Col != baselineCfg {
				base, ok := table.Cells[TableKey{k.Row, baselineCfg}]
				if ok {
					cell.Baseline = base
				}
			}

			limit <- struct{}{}
			cCell := cCell
			go func() {
				summarizeCell(cCell, cell, assumption, opts.Confidence)
				<-limit
				wg.Done()
			}()
		}
	}
	wg.Wait()

	// Add summary rows to each table.
	for _, table := range tables {
		table.SummaryLabel = "geomean"
		table.Summary = make(map[benchproc.Config]*TableSummary)

		// Count the number of baseline benchmarks so we can
		// test if later columns don't match.
		nBase := 0
		baseCol := table.Cols[0]
		for _, row := range table.Rows {
			if _, ok := table.Cells[TableKey{row, baseCol}]; ok {
				nBase++
			}
		}

		for i, col := range table.Cols {
			var s TableSummary
			table.Summary[col] = &s
			isBase := i == 0

			limit <- struct{}{}
			table, col := table, col
			wg.Add(1)
			go func() {
				summarizeCol(table, col, &s, nBase, isBase)
				<-limit
				wg.Done()
			}()
		}
	}
	wg.Wait()

	return &Tables{tables, configs}
}

func mapConfigs(m map[benchproc.Config]struct{}) []benchproc.Config {
	var cs []benchproc.Config
	for k := range m {
		cs = append(cs, k)
	}
	benchproc.SortConfigs(cs)
	return cs
}

func summarizeCell(cCell *cell, cell *TableCell, assumption benchmath.Assumption, confidence float64) {
	cell.Summary = assumption.Summary(cell.Sample, confidence)

	// If there's a baseline, compute comparison.
	if cell.Baseline != nil {
		cell.Comparison = assumption.Compare(cell.Baseline.Sample, cell.Sample)
	}

	// Warn for non-singular configuration values in this cell.
	nsk := benchproc.NonSingularFields(mapConfigs(cCell.configs))
	if len(nsk) > 0 {
		// Emit a warning.
		var warn strings.Builder
		warn.WriteString("benchmarks vary in ")
		for i, field := range nsk {
			if i > 0 {
				warn.WriteString(", ")
			}
			warn.WriteString(field.Name)
		}

		cell.Sample.Warnings = append(cell.Sample.Warnings, errors.New(warn.String()))
	}
}

func summarizeCol(table *Table, col benchproc.Config, s *TableSummary, nBase int, isBase bool) {
	// Collect cells.
	//
	// This computes the geomean of the summary ratios rather than
	// ratio of the summary geomeans. These are identical *if* the
	// benchmark sets are the same. But if the benchmark sets
	// differ, this leads to more sensible ratios because it's
	// still the geomean of the column, rather than being a
	// comparison of two incomparable numbers. It's still easy to
	// misinterpret, but at least it's not meaningless.
	var summaries, ratios []float64
	badRatio := false
	for _, row := range table.Rows {
		cell, ok := table.Cells[TableKey{row, col}]
		if !ok {
			continue
		}
		summaries = append(summaries, cell.Summary.Center)
		if cell.Baseline != nil {
			var ratio float64
			a, b := cell.Summary.Center, cell.Baseline.Summary.Center
			if a == b {
				// Treat 0/0 as 1.
				ratio = 1
			} else if b == 0 {
				badRatio = true
				// Keep nBase check working.
				ratios = append(ratios, 0)
				continue
			} else {
				ratio = a / b
			}
			ratios = append(ratios, ratio)
		}
	}

	// If the number of cells in this column that had a baseline
	// is the same as the total number of baselines, then we know
	// the benchmark sets match. Otherwise, they don't and these
	// numbers are probably misleading.
	if !isBase && nBase != len(ratios) {
		s.Warnings = append(s.Warnings, fmt.Errorf("benchmark set differs from baseline; geomeans may not be comparable"))
	}

	// Summarize centers.
	gm := stats.GeoMean(summaries)
	if math.IsNaN(gm) {
		s.Warnings = append(s.Warnings, fmt.Errorf("summaries must be >0 to compute geomean"))
	} else {
		s.HasSummary = true
		s.Summary = gm
	}

	// Summarize ratios.
	if !isBase && !badRatio {
		gm := stats.GeoMean(ratios)
		if math.IsNaN(gm) {
			s.Warnings = append(s.Warnings, fmt.Errorf("ratios must be >0 to compute geomean"))
		} else {
			s.HasRatio = true
			s.Ratio = gm
		}
	}
}

// ToText renders t to a textual representation, assuming a
// fixed-width font.
func (t *Tables) ToText(w io.Writer, color bool) error {
	return t.printTables(func(hdr string) error {
		_, err := fmt.Fprintf(w, "%s\n", hdr)
		return err
	}, func(table *Table) error {
		return table.ToText(w, color)
	})
}

// ToCSV returns t to CSV (comma-separated values) format.
//
// Warnings are written to a separate stream so as not to interrupt
// the regular format of the CSV table.
func (t *Tables) ToCSV(w, warnings io.Writer) error {
	o := csv.NewWriter(w)
	row := 1

	err := t.printTables(func(hdr string) error {
		o.Write([]string{hdr})
		row++
		return nil
	}, func(table *Table) error {
		nRows := table.ToCSV(o, row, warnings)
		row += nRows
		return nil
	})
	if err != nil {
		return err
	}
	o.Flush()
	return o.Error()
}

func (t *Tables) printTables(hdr func(string) error, cb func(*Table) error) error {
	if len(t.Tables) == 0 {
		return nil
	}

	var prevConfig benchproc.Config
	fields := t.Configs[0].Schema().Fields()

	for i, table := range t.Tables {
		if i > 0 {
			// Blank line between tables.
			if err := hdr(""); err != nil {
				return err
			}
		}

		// Print table config changes (except .unit, which is
		// printed in the table itself)
		config := t.Configs[i]
		for _, f := range fields {
			if f.Name == ".unit" {
				continue
			}
			val := config.Get(f)
			if prevConfig.IsZero() || val != prevConfig.Get(f) {
				if err := hdr(fmt.Sprintf("%s: %s", f.Name, val)); err != nil {
					return err
				}
			}
		}
		prevConfig = config

		// Print table.
		if err := cb(table); err != nil {
			return err
		}
	}

	return nil
}
