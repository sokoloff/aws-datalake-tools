package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareSchemas(t *testing.T) {
	glueCols := []Column{
		{Name: "id", Type: PrimitiveType{Kind: BigInt}},
		{Name: "name", Type: PrimitiveType{Kind: String}},
	}

	t.Run("identical", func(t *testing.T) {
		fileCols := []Column{
			{Name: "id", Type: PrimitiveType{Kind: BigInt}},
			{Name: "name", Type: PrimitiveType{Kind: String}},
		}
		plan := CompareSchemas(glueCols, fileCols)
		assert.True(t, plan.Compatible)
		assert.Empty(t, plan.Diffs)
		assert.Equal(t, []int{0, 1}, plan.ColumnMapping)
	})

	t.Run("coercible", func(t *testing.T) {
		fileCols := []Column{
			{Name: "id", Type: PrimitiveType{Kind: Int}}, // Int -> BigInt is OK
			{Name: "name", Type: PrimitiveType{Kind: String}},
		}
		plan := CompareSchemas(glueCols, fileCols)
		assert.True(t, plan.Compatible)
		assert.Empty(t, plan.Diffs) // Coercible types don't show up in Diffs yet, or do they?
		// In my implementation, they DON'T show up in Diffs if Compatible stays true.
		assert.Equal(t, []int{0, 1}, plan.ColumnMapping)
	})

	t.Run("type_mismatch", func(t *testing.T) {
		fileCols := []Column{
			{Name: "id", Type: PrimitiveType{Kind: String}}, // String -> BigInt is NOT OK
			{Name: "name", Type: PrimitiveType{Kind: String}},
		}
		plan := CompareSchemas(glueCols, fileCols)
		assert.False(t, plan.Compatible)
		assert.Len(t, plan.Diffs, 1)
		assert.Equal(t, DiffTypeMismatch, plan.Diffs[0].Kind)
	})

	t.Run("reordered", func(t *testing.T) {
		fileCols := []Column{
			{Name: "name", Type: PrimitiveType{Kind: String}},
			{Name: "id", Type: PrimitiveType{Kind: BigInt}},
		}
		plan := CompareSchemas(glueCols, fileCols)
		assert.True(t, plan.Compatible)
		assert.Len(t, plan.Diffs, 2)
		assert.Equal(t, DiffOrderMismatch, plan.Diffs[0].Kind)
		assert.Equal(t, []int{1, 0}, plan.ColumnMapping)
	})

	t.Run("extra_and_missing", func(t *testing.T) {
		fileCols := []Column{
			{Name: "id", Type: PrimitiveType{Kind: BigInt}},
			{Name: "extra", Type: PrimitiveType{Kind: Int}},
		}
		plan := CompareSchemas(glueCols, fileCols)
		assert.True(t, plan.Compatible)
		assert.Len(t, plan.Diffs, 2)
		assert.Equal(t, DiffMissingInFile, plan.Diffs[0].Kind)
		assert.Equal(t, "name", plan.Diffs[0].Column)
		assert.Equal(t, DiffExtraInFile, plan.Diffs[1].Kind)
		assert.Equal(t, "extra", plan.Diffs[1].Column)
	})

	t.Run("nested_struct_mismatch", func(t *testing.T) {
		glue := []Column{{Name: "user", Type: StructType{Fields: []StructField{
			{Name: "id", Type: PrimitiveType{Kind: Int}},
			{Name: "meta", Type: StructType{Fields: []StructField{
				{Name: "key", Type: PrimitiveType{Kind: String}},
			}}},
		}}}}
		file := []Column{{Name: "user", Type: StructType{Fields: []StructField{
			{Name: "id", Type: PrimitiveType{Kind: String}}, // mismatch
			{Name: "meta", Type: StructType{Fields: []StructField{
				{Name: "extra", Type: PrimitiveType{Kind: Int}}, // missing 'key', extra 'extra'
			}}},
		}}}}
		plan := CompareSchemas(glue, file)
		assert.False(t, plan.Compatible)
		assert.NotEmpty(t, plan.Diffs)

		foundID := false
		foundKey := false
		for _, d := range plan.Diffs {
			if d.FullPath() == "user.id" && d.Kind == DiffTypeMismatch {
				foundID = true
			}
			if d.FullPath() == "user.meta.key" && d.Kind == DiffMissingInFile {
				foundKey = true
			}
		}
		assert.True(t, foundID)
		assert.True(t, foundKey)
	})

	t.Run("nested_array_mismatch", func(t *testing.T) {
		glue := []Column{{Name: "tags", Type: ArrayType{ElementType: PrimitiveType{Kind: Int}}}}
		file := []Column{{Name: "tags", Type: ArrayType{ElementType: PrimitiveType{Kind: String}}}}
		plan := CompareSchemas(glue, file)
		assert.False(t, plan.Compatible)
		assert.Equal(t, "tags.[]", plan.Diffs[0].FullPath())
	})
}

func TestIsTypeCoercible(t *testing.T) {
	tests := []struct {
		from, to DataType
		want     bool
	}{
		{PrimitiveType{Kind: TinyInt}, PrimitiveType{Kind: SmallInt}, true},
		{PrimitiveType{Kind: TinyInt}, PrimitiveType{Kind: Int}, true},
		{PrimitiveType{Kind: TinyInt}, PrimitiveType{Kind: BigInt}, true},
		{PrimitiveType{Kind: SmallInt}, PrimitiveType{Kind: Int}, true},
		{PrimitiveType{Kind: SmallInt}, PrimitiveType{Kind: BigInt}, true},
		{PrimitiveType{Kind: Int}, PrimitiveType{Kind: BigInt}, true},
		{PrimitiveType{Kind: Float}, PrimitiveType{Kind: Double}, true},
		{PrimitiveType{Kind: String}, PrimitiveType{Kind: BigInt}, false},
		{PrimitiveType{Kind: BigInt}, PrimitiveType{Kind: Int}, false},
		{DecimalType{10, 2}, DecimalType{10, 2}, true},
		{DecimalType{10, 2}, PrimitiveType{Kind: BigInt}, false},
	}

	for _, tt := range tests {
		t.Run(tt.from.String()+"->"+tt.to.String(), func(t *testing.T) {
			assert.Equal(t, tt.want, IsTypeCoercible(tt.from, tt.to))
		})
	}
}
