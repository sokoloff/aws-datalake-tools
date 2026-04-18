package compact

import (
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestColumnsToParquetGroup(t *testing.T) {
	cols := []schema.Column{
		{Name: "id", Type: schema.PrimitiveType{Kind: schema.BigInt}},
		{Name: "name", Type: schema.PrimitiveType{Kind: schema.String}},
		{Name: "is_active", Type: schema.PrimitiveType{Kind: schema.Boolean}},
		{Name: "created_at", Type: schema.PrimitiveType{Kind: schema.Timestamp}},
		{Name: "date", Type: schema.PrimitiveType{Kind: schema.Date}},
		{Name: "binary", Type: schema.PrimitiveType{Kind: schema.Binary}},
		{Name: "float", Type: schema.PrimitiveType{Kind: schema.Float}},
		{Name: "double", Type: schema.PrimitiveType{Kind: schema.Double}},
		{Name: "decimal", Type: schema.DecimalType{Precision: 10, Scale: 2}},
		{Name: "tags", Type: schema.ArrayType{ElementType: schema.PrimitiveType{Kind: schema.String}}},
		{Name: "props", Type: schema.MapType{
			KeyType:   schema.PrimitiveType{Kind: schema.String},
			ValueType: schema.PrimitiveType{Kind: schema.Int},
		}},
		{Name: "nested", Type: schema.StructType{Fields: []schema.StructField{
			{Name: "f1", Type: schema.PrimitiveType{Kind: schema.Int}},
		}}},
	}

	node, err := ColumnsToParquetGroup(cols)
	assert.NoError(t, err)

	schema := parquet.NewSchema("schema", node)
	assert.NotNil(t, schema)
	assert.Equal(t, 12, len(schema.Fields()))
}

func TestColumnsToParquetGroup_Unsupported(t *testing.T) {
	type customType struct{ schema.DataType }
	
	t.Run("top level", func(t *testing.T) {
		cols := []schema.Column{{Name: "err", Type: customType{}}}
		_, err := ColumnsToParquetGroup(cols)
		assert.Error(t, err)
	})

	t.Run("nested struct", func(t *testing.T) {
		cols := []schema.Column{{Name: "s", Type: schema.StructType{Fields: []schema.StructField{{Name: "f", Type: customType{}}}}}}
		_, err := ColumnsToParquetGroup(cols)
		assert.Error(t, err)
	})

	t.Run("array", func(t *testing.T) {
		cols := []schema.Column{{Name: "a", Type: schema.ArrayType{ElementType: customType{}}}}
		_, err := ColumnsToParquetGroup(cols)
		assert.Error(t, err)
	})

	t.Run("map key", func(t *testing.T) {
		// Note: KeyType uses dataTypeToNodeInternal directly in my code, so it might return error
		cols := []schema.Column{{Name: "m", Type: schema.MapType{KeyType: customType{}, ValueType: schema.PrimitiveType{Kind: schema.Int}}}}
		_, err := ColumnsToParquetGroup(cols)
		assert.Error(t, err)
	})

	t.Run("map value", func(t *testing.T) {
		cols := []schema.Column{{Name: "m", Type: schema.MapType{KeyType: schema.PrimitiveType{Kind: schema.String}, ValueType: customType{}}}}
		_, err := ColumnsToParquetGroup(cols)
		assert.Error(t, err)
	})
}


