package schema

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatSchema(t *testing.T) {
	s := &TableSchema{
		Database: "test_db",
		Table:    "test_table",
		Location: "s3://bucket/prefix",
		Columns: []Column{
			{Name: "id", Type: PrimitiveType{Kind: BigInt}, Comment: "identifier"},
			{Name: "name", Type: PrimitiveType{Kind: String}},
		},
		PartitionKeys: []Column{
			{Name: "dt", Type: PrimitiveType{Kind: String}, Comment: "date partition"},
		},
	}

	var buf bytes.Buffer
	err := FormatSchema(&buf, s, false)
	assert.NoError(t, err)

	output := buf.String()
	t.Logf("Output:\n%s", output)
}
