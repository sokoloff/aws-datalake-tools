package schema

// DiffKind represents the type of difference between two columns or fields.
type DiffKind string

const (
	DiffMissingInFile DiffKind = "missing_in_file"
	DiffExtraInFile   DiffKind = "extra_in_file"
	DiffTypeMismatch  DiffKind = "type_mismatch"
	DiffOrderMismatch DiffKind = "order_mismatch"
)

// ColumnDiff represents a difference between Glue and Parquet schemas.
type ColumnDiff struct {
	Kind      DiffKind
	Column    string   // Top-level column name
	Path      []string // Path to nested field
	GlueType  DataType
	FileType  DataType
	GlueIndex int
	FileIndex int
}

func (d ColumnDiff) FullPath() string {
	if len(d.Path) == 0 {
		return d.Column
	}
	res := d.Column
	for _, p := range d.Path {
		res += "." + p
	}
	return res
}

// CoercionPlan represents the result of comparing two schemas.
type CoercionPlan struct {
	Compatible    bool
	Diffs         []ColumnDiff
	ColumnMapping []int // Maps Glue column index to File column index (-1 if missing)
}

// CompareSchemas compares Glue columns with Parquet columns recursively.
func CompareSchemas(glueCols, fileCols []Column) *CoercionPlan {
	plan := &CoercionPlan{
		Compatible:    true,
		ColumnMapping: make([]int, len(glueCols)),
	}

	fileColMap := make(map[string]int)
	for i, c := range fileCols {
		fileColMap[c.Name] = i
	}

	glueColMap := make(map[string]int)
	for i, c := range glueCols {
		glueColMap[c.Name] = i
	}

	// Check for missing or type-mismatched columns in file
	for i, gc := range glueCols {
		plan.ColumnMapping[i] = -1
		if fi, ok := fileColMap[gc.Name]; ok {
			plan.ColumnMapping[i] = fi
			fc := fileCols[fi]
			
			if !gc.Type.Equal(fc.Type) {
				if !IsTypeCoercible(fc.Type, gc.Type) {
					// Pinpoint the mismatch
					mismatches := findTypeMismatches(gc.Name, nil, gc.Type, fc.Type)
					if len(mismatches) > 0 {
						plan.Compatible = false
						for _, m := range mismatches {
							m.GlueIndex = i
							m.FileIndex = fi
							plan.Diffs = append(plan.Diffs, m)
						}
					}
				}
			}
			if i != fi {
				plan.Diffs = append(plan.Diffs, ColumnDiff{
					Kind:      DiffOrderMismatch,
					Column:    gc.Name,
					GlueIndex: i,
					FileIndex: fi,
				})
			}
		} else {
			plan.Diffs = append(plan.Diffs, ColumnDiff{
				Kind:      DiffMissingInFile,
				Column:    gc.Name,
				GlueType:  gc.Type,
				GlueIndex: i,
			})
		}
	}

	// Check for extra columns in file
	for i, fc := range fileCols {
		if _, ok := glueColMap[fc.Name]; !ok {
			plan.Diffs = append(plan.Diffs, ColumnDiff{
				Kind:      DiffExtraInFile,
				Column:    fc.Name,
				FileType:  fc.Type,
				FileIndex: i,
			})
		}
	}

	return plan
}

func findTypeMismatches(column string, path []string, glue, file DataType) []ColumnDiff {
	if glue.Equal(file) || IsTypeCoercible(file, glue) {
		return nil
	}

	// Try to recurse into structs
	gs, ok1 := glue.(StructType)
	fs, ok2 := file.(StructType)
	if ok1 && ok2 {
		var diffs []ColumnDiff
		fMap := make(map[string]int)
		for i, f := range fs.Fields {
			fMap[f.Name] = i
		}
		for _, gf := range gs.Fields {
			if fi, ok := fMap[gf.Name]; ok {
				newPath := append([]string(nil), path...)
				newPath = append(newPath, gf.Name)
				diffs = append(diffs, findTypeMismatches(column, newPath, gf.Type, fs.Fields[fi].Type)...)
			} else {
				diffs = append(diffs, ColumnDiff{
					Kind:     DiffMissingInFile,
					Column:   column,
					Path:     append(append([]string(nil), path...), gf.Name),
					GlueType: gf.Type,
				})
			}
		}
		// Extra in file
		gMap := make(map[string]bool)
		for _, f := range gs.Fields {
			gMap[f.Name] = true
		}
		for _, ff := range fs.Fields {
			if !gMap[ff.Name] {
				diffs = append(diffs, ColumnDiff{
					Kind:     DiffExtraInFile,
					Column:   column,
					Path:     append(append([]string(nil), path...), ff.Name),
					FileType: ff.Type,
				})
			}
		}
		if len(diffs) > 0 {
			return diffs
		}
	}

	// Try to recurse into arrays
	ga, ok1 := glue.(ArrayType)
	fa, ok2 := file.(ArrayType)
	if ok1 && ok2 {
		newPath := append([]string(nil), path...)
		newPath = append(newPath, "[]")
		return findTypeMismatches(column, newPath, ga.ElementType, fa.ElementType)
	}

	// Terminal mismatch
	return []ColumnDiff{{
		Kind:     DiffTypeMismatch,
		Column:   column,
		Path:     path,
		GlueType: glue,
		FileType: file,
	}}
}

// IsTypeCoercible returns true if 'from' can be safely widened to 'to'.
func IsTypeCoercible(from, to DataType) bool {
	if from.Equal(to) {
		return true
	}
	f, okf := from.(PrimitiveType)
	t, okt := to.(PrimitiveType)
	if !okf || !okt {
		return false
	}

	widening := map[PrimitiveKind][]PrimitiveKind{
		TinyInt:  {SmallInt, Int, BigInt},
		SmallInt: {Int, BigInt},
		Int:      {BigInt},
		Float:    {Double},
	}

	for _, w := range widening[f.Kind] {
		if w == t.Kind {
			return true
		}
	}

	return false
}
