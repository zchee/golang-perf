// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchproc

// NonSingularFields returns the subset of Schema fields for which at
// least two of configs have different values.
//
// This is useful for warning the user if aggregating a set of results
// has resulted in potentially hiding important configuration
// differences. Typically these configurations are "residue"
// configurations produced by ProjectionParser.Residue.
func NonSingularFields(configs []Config) []Field {
	// TODO: This is generally used on residue configs, but those
	// might just have ".fullname" (generally with implicit
	// exclusions). Telling the user that a set of benchmarks
	// varies in ".fullname" isn't nearly as useful as listing out
	// the specific subfields. This API can't do that (right now)
	// because the subfields of .fullname don't even have Fields.
	// But maybe the residue schema should dynamically break out
	// .fullname into its parts the way we do with .config (in
	// which case NonSingularFields would just work).

	if len(configs) <= 1 {
		// There can't be any differences.
		return nil
	}
	var out []Field
	fields := commonSchema(configs).Fields()
	for _, f := range fields {
		base := configs[0].Get(f)
		for _, c := range configs[1:] {
			if c.Get(f) != base {
				out = append(out, f)
				break
			}
		}
	}
	return out
}
