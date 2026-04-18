package load

import (
	"encoding/json"
	"testing"

	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferrer_ObserveAndFinalize(t *testing.T) {
	inf := NewInferrer()

	// Record 1: Base types
	inf.Observe(map[string]any{
		"id":    "123",
		"val":   json.Number("42"),
		"valid": true,
		"meta":  map[string]any{"tags": []any{"a", "b"}},
	})

	// Record 2: Type promotion (int -> double) and optional field
	inf.Observe(map[string]any{
		"id":    "456",
		"val":   json.Number("3.14"),
		"extra": "missing in rec 1",
	})

	// Record 3: Null fallback and deep nesting
	inf.Observe(map[string]any{
		"id":   "789",
		"val":  nil,
		"null": nil,
		"nested": map[string]any{
			"deep": map[string]any{"x": json.Number("1")},
		},
	})

	cols := inf.Finalize()
	require.Len(t, cols, 7) // id, val, valid, meta, extra, null, nested

	// Map columns by name for easier assertions
	colMap := make(map[string]schema.Column)
	for _, c := range cols {
		colMap[c.Name] = c
	}

	assert.Equal(t, schema.String, colMap["id"].Type.(schema.PrimitiveType).Kind)
	assert.Equal(t, schema.Double, colMap["val"].Type.(schema.PrimitiveType).Kind) // promoted from BigInt to Double
	assert.Equal(t, schema.Boolean, colMap["valid"].Type.(schema.PrimitiveType).Kind)
	assert.Equal(t, schema.String, colMap["extra"].Type.(schema.PrimitiveType).Kind)
	assert.Equal(t, schema.String, colMap["null"].Type.(schema.PrimitiveType).Kind) // pure null fallback

	// Nested meta
	meta := colMap["meta"].Type.(schema.StructType)
	assert.Len(t, meta.Fields, 1)
	assert.Equal(t, "tags", meta.Fields[0].Name)
	assert.IsType(t, schema.ArrayType{}, meta.Fields[0].Type)

	// Deep nesting
	nested := colMap["nested"].Type.(schema.StructType)
	deep := nested.Fields[0].Type.(schema.StructType)
	assert.Equal(t, schema.BigInt, deep.Fields[0].Type.(schema.PrimitiveType).Kind)
}

func TestInferrer_Sets(t *testing.T) {
	inf := NewInferrer()
	inf.Observe(map[string]any{
		"ss": []string{"a", "b"},
		"ns": []json.Number{json.Number("1"), json.Number("2")},
		"bs": [][]byte{[]byte("bin")},
	})

	cols := inf.Finalize()
	colMap := make(map[string]schema.Column)
	for _, c := range cols {
		colMap[c.Name] = c
	}

	assert.IsType(t, schema.ArrayType{}, colMap["ss"].Type)
	assert.Equal(t, schema.String, colMap["ss"].Type.(schema.ArrayType).ElementType.(schema.PrimitiveType).Kind)
	assert.IsType(t, schema.ArrayType{}, colMap["ns"].Type)
	assert.Equal(t, schema.BigInt, colMap["ns"].Type.(schema.ArrayType).ElementType.(schema.PrimitiveType).Kind)
	assert.IsType(t, schema.ArrayType{}, colMap["bs"].Type)
	assert.Equal(t, schema.Binary, colMap["bs"].Type.(schema.ArrayType).ElementType.(schema.PrimitiveType).Kind)
}
