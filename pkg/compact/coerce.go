package compact

import (
	"fmt"
	"strings"

	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

// BuildConversion creates a parquet.Conversion between the target node and source schema.
func BuildConversion(target parquet.Node, source *parquet.Schema) (parquet.Conversion, error) {
	return parquet.Convert(target, source)
}

// ValidateCoercion checks if fileCols can be coerced into targetCols.
// Returns a summary string and an error if incompatible.
func ValidateCoercion(targetCols, fileCols []schema.Column) (string, error) {
	plan := schema.CompareSchemas(targetCols, fileCols)
	if plan.Compatible {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("schema incompatible:\n")
	for _, diff := range plan.Diffs {
		if diff.Kind == schema.DiffTypeMismatch {
			sb.WriteString(fmt.Sprintf("  - column '%s' type mismatch: cannot coerce file type %s to target type %s\n", 
				diff.FullPath(), diff.FileType, diff.GlueType))
		}
	}
	return sb.String(), fmt.Errorf("incompatible schemas")
}
