package schema

import (
	"testing"
)

func TestParseType(t *testing.T) {
	tests := []struct {
		input    string
		expected DataType
		wantErr  bool
	}{
		{"string", PrimitiveType{Kind: String}, false},
		{"int", PrimitiveType{Kind: Int}, false},
		{"decimal(10,2)", DecimalType{Precision: 10, Scale: 2}, false},
		{"array<string>", ArrayType{ElementType: PrimitiveType{Kind: String}}, false},
		{"map<string,int>", MapType{KeyType: PrimitiveType{Kind: String}, ValueType: PrimitiveType{Kind: Int}}, false},
		{
			"struct<name:string,age:int>",
			StructType{Fields: []StructField{
				{Name: "name", Type: PrimitiveType{Kind: String}},
				{Name: "age", Type: PrimitiveType{Kind: Int}},
			}},
			false,
		},
		{
			"struct<name:string,tags:array<string>,meta:map<string,string>>",
			StructType{Fields: []StructField{
				{Name: "name", Type: PrimitiveType{Kind: String}},
				{Name: "tags", Type: ArrayType{ElementType: PrimitiveType{Kind: String}}},
				{Name: "meta", Type: MapType{KeyType: PrimitiveType{Kind: String}, ValueType: PrimitiveType{Kind: String}}},
			}},
			false,
		},
		{
			"array<struct<id:int,val:string>>",
			ArrayType{ElementType: StructType{Fields: []StructField{
				{Name: "id", Type: PrimitiveType{Kind: Int}},
				{Name: "val", Type: PrimitiveType{Kind: String}},
			}}},
			false,
		},
		{"  string  ", PrimitiveType{Kind: String}, false},
		{"struct < name : string , age : int >", StructType{Fields: []StructField{
			{Name: "name", Type: PrimitiveType{Kind: String}},
			{Name: "age", Type: PrimitiveType{Kind: Int}},
		}}, false},
		{"invalid", nil, true},
		{"struct<name:string", nil, true},
		{"array<string", nil, true},
		{"decimal(10)", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.GlueType() != tt.expected.GlueType() {
					t.Errorf("ParseType() = %v, want %v", got.GlueType(), tt.expected.GlueType())
				}
				if !got.Equal(tt.expected) {
					t.Errorf("ParseType() not equal to expected")
				}
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	types := []DataType{
		PrimitiveType{Kind: String},
		DecimalType{Precision: 18, Scale: 9},
		ArrayType{ElementType: PrimitiveType{Kind: BigInt}},
		MapType{KeyType: PrimitiveType{Kind: String}, ValueType: PrimitiveType{Kind: Double}},
		StructType{Fields: []StructField{
			{Name: "f1", Type: PrimitiveType{Kind: Int}},
			{Name: "f2", Type: ArrayType{ElementType: PrimitiveType{Kind: String}}},
		}},
	}

	for _, dt := range types {
		s := dt.GlueType()
		got, err := ParseType(s)
		if err != nil {
			t.Errorf("failed to parse generated type string %q: %v", s, err)
			continue
		}
		if !got.Equal(dt) {
			t.Errorf("round-trip failed for %q: got %q, want %q", s, got.GlueType(), dt.GlueType())
		}
	}
}
