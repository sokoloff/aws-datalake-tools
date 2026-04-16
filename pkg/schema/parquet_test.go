package schema

import (
	"bytes"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
)

func TestParquetSchemaToColumns(t *testing.T) {
	type Row struct {
		ID   int32  `parquet:"id"`
		Name string `parquet:"name"`
		Tags []string `parquet:"tags"`
	}

	var buf bytes.Buffer
	writer := parquet.NewWriter(&buf, parquet.SchemaOf(Row{}))
	err := writer.Write(Row{ID: 1, Name: "test", Tags: []string{"a", "b"}})
	assert.NoError(t, err)
	err = writer.Close()
	assert.NoError(t, err)

	reader := bytes.NewReader(buf.Bytes())
	file, err := parquet.OpenFile(reader, int64(buf.Len()))
	assert.NoError(t, err)

	cols, err := ParquetSchemaToColumns(file.Schema())
	assert.NoError(t, err)

	assert.Len(t, cols, 3)
	assert.Equal(t, "id", cols[0].Name)
	assert.Equal(t, PrimitiveType{Kind: Int}, cols[0].Type)
	assert.Equal(t, "name", cols[1].Name)
	assert.Equal(t, PrimitiveType{Kind: String}, cols[1].Type)
	assert.Equal(t, "tags", cols[2].Name)
	assert.Equal(t, ArrayType{ElementType: PrimitiveType{Kind: String}}, cols[2].Type)
}

func TestParquetNodeToDataType_Decimal(t *testing.T) {
	schema := parquet.NewSchema("decimal", parquet.Group{
		"val": parquet.Decimal(2, 10, parquet.Int64Type),
	})
	cols, err := ParquetSchemaToColumns(schema)
	assert.NoError(t, err)
	assert.Len(t, cols, 1)
	assert.Equal(t, DecimalType{Precision: 10, Scale: 2}, cols[0].Type)
}
