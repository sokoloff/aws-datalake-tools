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
}
