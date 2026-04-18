package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypes_Equal(t *testing.T) {
	tests := []struct {
		name  string
		t1    DataType
		t2    DataType
		equal bool
	}{
		{
			name:  "primitive equal",
			t1:    PrimitiveType{Kind: String},
			t2:    PrimitiveType{Kind: String},
			equal: true,
		},
		{
			name:  "primitive unequal kind",
			t1:    PrimitiveType{Kind: String},
			t2:    PrimitiveType{Kind: Int},
			equal: false,
		},
		{
			name:  "primitive unequal type",
			t1:    PrimitiveType{Kind: String},
			t2:    DecimalType{Precision: 10, Scale: 2},
			equal: false,
		},
		{
			name:  "decimal equal",
			t1:    DecimalType{Precision: 10, Scale: 2},
			t2:    DecimalType{Precision: 10, Scale: 2},
			equal: true,
		},
		{
			name:  "decimal unequal precision",
			t1:    DecimalType{Precision: 10, Scale: 2},
			t2:    DecimalType{Precision: 11, Scale: 2},
			equal: false,
		},
		{
			name:  "decimal unequal scale",
			t1:    DecimalType{Precision: 10, Scale: 2},
			t2:    DecimalType{Precision: 10, Scale: 1},
			equal: false,
		},
		{
			name:  "array equal",
			t1:    ArrayType{ElementType: PrimitiveType{Kind: String}},
			t2:    ArrayType{ElementType: PrimitiveType{Kind: String}},
			equal: true,
		},
		{
			name:  "array unequal element",
			t1:    ArrayType{ElementType: PrimitiveType{Kind: String}},
			t2:    ArrayType{ElementType: PrimitiveType{Kind: Int}},
			equal: false,
		},
		{
			name:  "map equal",
			t1:    MapType{KeyType: PrimitiveType{Kind: String}, ValueType: PrimitiveType{Kind: Int}},
			t2:    MapType{KeyType: PrimitiveType{Kind: String}, ValueType: PrimitiveType{Kind: Int}},
			equal: true,
		},
		{
			name:  "map unequal value",
			t1:    MapType{KeyType: PrimitiveType{Kind: String}, ValueType: PrimitiveType{Kind: Int}},
			t2:    MapType{KeyType: PrimitiveType{Kind: String}, ValueType: PrimitiveType{Kind: String}},
			equal: false,
		},
		{
			name:  "struct equal",
			t1:    StructType{Fields: []StructField{{Name: "id", Type: PrimitiveType{Kind: Int}}}},
			t2:    StructType{Fields: []StructField{{Name: "id", Type: PrimitiveType{Kind: Int}}}},
			equal: true,
		},
		{
			name:  "struct unequal name",
			t1:    StructType{Fields: []StructField{{Name: "id", Type: PrimitiveType{Kind: Int}}}},
			t2:    StructType{Fields: []StructField{{Name: "uid", Type: PrimitiveType{Kind: Int}}}},
			equal: false,
		},
		{
			name:  "struct unequal count",
			t1:    StructType{Fields: []StructField{{Name: "id", Type: PrimitiveType{Kind: Int}}}},
			t2:    StructType{Fields: []StructField{{Name: "id", Type: PrimitiveType{Kind: Int}}, {Name: "val", Type: PrimitiveType{Kind: String}}}},
			equal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.equal, tt.t1.Equal(tt.t2))
		})
	}
}

func TestTypes_StringAndPretty(t *testing.T) {
	s := StructType{
		Fields: []StructField{
			{Name: "id", Type: PrimitiveType{Kind: Int}, NativeType: "INT32"},
			{Name: "tags", Type: ArrayType{ElementType: PrimitiveType{Kind: String}}},
			{Name: "meta", Type: MapType{KeyType: PrimitiveType{Kind: String}, ValueType: PrimitiveType{Kind: String}}},
		},
	}

	assert.NotEmpty(t, s.GlueType())
	assert.NotEmpty(t, s.String())
	assert.NotEmpty(t, s.Pretty(0, true))
	assert.NotEmpty(t, s.Pretty(0, false))

	a := ArrayType{ElementType: StructType{Fields: []StructField{{Name: "f1", Type: PrimitiveType{Kind: String}}}}}
	assert.NotEmpty(t, a.Pretty(0, false))

	d := DecimalType{Precision: 10, Scale: 2}
	assert.Equal(t, "decimal(10,2)", d.GlueType())
	assert.Equal(t, "decimal(10,2)", d.Pretty(0, false))
}
