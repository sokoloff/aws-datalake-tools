package load

import (
	"encoding/json"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/pkg/compact"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRowPlanner_Build(t *testing.T) {
	cols := []schema.Column{
		{Name: "id", Type: schema.PrimitiveType{Kind: schema.String}},
		{Name: "val", Type: schema.PrimitiveType{Kind: schema.Double}},
		{Name: "opt", Type: schema.PrimitiveType{Kind: schema.BigInt}},
		{Name: "meta", Type: schema.StructType{Fields: []schema.StructField{
			{Name: "tag", Type: schema.PrimitiveType{Kind: schema.String}},
		}}},
	}

	targetNode, err := compact.ColumnsToParquetGroup(cols)
	require.NoError(t, err)
	parquetSchema := parquet.NewSchema("test", targetNode)
	planner := NewRowPlanner(parquetSchema, targetNode)

	// Identify indices
	indices := make(map[string]int)
	for i, colPath := range parquetSchema.Columns() {
		indices[colPath[len(colPath)-1]] = i
	}

	rec := map[string]any{
		"id":  "123",
		"val": json.Number("42.5"),
		"opt": nil,
		"meta": map[string]any{
			"tag": "a",
		},
	}

	row, err := planner.Build(rec, nil)
	require.NoError(t, err)

	// id
	idxID := indices["id"]
	assert.Equal(t, "123", row[idxID].String())
	assert.Equal(t, 1, row[idxID].DefinitionLevel())

	// val
	idxVal := indices["val"]
	assert.Equal(t, 42.5, row[idxVal].Double())
	assert.Equal(t, 1, row[idxVal].DefinitionLevel())

	// opt
	idxOpt := indices["opt"]
	assert.True(t, row[idxOpt].IsNull())
	assert.Equal(t, 0, row[idxOpt].DefinitionLevel())

	// tag
	idxTag := indices["tag"]
	assert.Equal(t, "a", row[idxTag].String())
	assert.Equal(t, 2, row[idxTag].DefinitionLevel())
}
