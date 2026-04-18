package compact

import (
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

func TestColumnsToParquetGroup(t *testing.T) {
	cols := []schema.Column{
		{Name: "id", Type: schema.PrimitiveType{Kind: schema.BigInt}},
		{Name: "name", Type: schema.PrimitiveType{Kind: schema.String}},
		{Name: "is_active", Type: schema.PrimitiveType{Kind: schema.Boolean}},
		{Name: "created_at", Type: schema.PrimitiveType{Kind: schema.Timestamp}},
		{Name: "tags", Type: schema.ArrayType{ElementType: schema.PrimitiveType{Kind: schema.String}}},
		{Name: "props", Type: schema.MapType{
			KeyType:   schema.PrimitiveType{Kind: schema.String},
			ValueType: schema.PrimitiveType{Kind: schema.Int},
		}},
	}

	node, err := ColumnsToParquetGroup(cols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	schema := parquet.NewSchema("schema", node)
	if schema == nil {
		t.Fatal("expected valid schema")
	}

	// Basic validation of fields presence
	fields := schema.Fields()
	if len(fields) != 6 {
		t.Fatalf("expected 6 fields, got %d", len(fields))
	}
}
