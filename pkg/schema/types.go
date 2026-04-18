package schema

import (
	"fmt"
	"strings"
)

// DataType represents a Glue Data Catalog type.
type DataType interface {
	GlueType() string
	Equal(other DataType) bool
	String() string // Compact representation
	Pretty(indent int, native bool) string
}

// PrimitiveKind represents the basic types supported by Glue.
type PrimitiveKind string

const (
	String    PrimitiveKind = "string"
	Boolean   PrimitiveKind = "boolean"
	TinyInt   PrimitiveKind = "tinyint"
	SmallInt  PrimitiveKind = "smallint"
	Int       PrimitiveKind = "int"
	BigInt    PrimitiveKind = "bigint"
	Float     PrimitiveKind = "float"
	Double    PrimitiveKind = "double"
	Date      PrimitiveKind = "date"
	Timestamp PrimitiveKind = "timestamp"
	Binary    PrimitiveKind = "binary"
)

// PrimitiveType represents a simple scalar type.
type PrimitiveType struct {
	Kind PrimitiveKind
}

func (t PrimitiveType) GlueType() string { return string(t.Kind) }
func (t PrimitiveType) String() string   { return string(t.Kind) }
func (t PrimitiveType) Pretty(indent int, native bool) string {
	return string(t.Kind)
}
func (t PrimitiveType) Equal(other DataType) bool {
	o, ok := other.(PrimitiveType)
	return ok && t.Kind == o.Kind
}

// DecimalType represents a fixed-precision decimal type.
type DecimalType struct {
	Precision int
	Scale     int
}

func (t DecimalType) GlueType() string {
	return fmt.Sprintf("decimal(%d,%d)", t.Precision, t.Scale)
}
func (t DecimalType) String() string { return t.GlueType() }
func (t DecimalType) Pretty(indent int, native bool) string {
	return t.GlueType()
}
func (t DecimalType) Equal(other DataType) bool {
	o, ok := other.(DecimalType)
	return ok && t.Precision == o.Precision && t.Scale == o.Scale
}

// ArrayType represents a list of elements of the same type.
type ArrayType struct {
	ElementType DataType
}

func (t ArrayType) GlueType() string {
	return fmt.Sprintf("array<%s>", t.ElementType.GlueType())
}
func (t ArrayType) String() string {
	return fmt.Sprintf("array<%s>", t.ElementType.String())
}
func (t ArrayType) Pretty(indent int, native bool) string {
	if st, ok := t.ElementType.(StructType); ok {
		return fmt.Sprintf("array<%s>", st.Pretty(indent, native))
	}
	return fmt.Sprintf("array<%s>", t.ElementType.Pretty(indent, native))
}
func (t ArrayType) Equal(other DataType) bool {
	o, ok := other.(ArrayType)
	return ok && t.ElementType.Equal(o.ElementType)
}

// MapType represents a collection of key-value pairs.
type MapType struct {
	KeyType   DataType
	ValueType DataType
}

func (t MapType) GlueType() string {
	return fmt.Sprintf("map<%s,%s>", t.KeyType.GlueType(), t.ValueType.GlueType())
}
func (t MapType) String() string {
	return fmt.Sprintf("map<%s, %s>", t.KeyType.String(), t.ValueType.String())
}
func (t MapType) Pretty(indent int, native bool) string {
	return fmt.Sprintf("map<%s, %s>", t.KeyType.Pretty(indent, native), t.ValueType.Pretty(indent, native))
}
func (t MapType) Equal(other DataType) bool {
	o, ok := other.(MapType)
	return ok && t.KeyType.Equal(o.KeyType) && t.ValueType.Equal(o.ValueType)
}

// StructField represents a single field in a struct.
type StructField struct {
	Name       string
	Type       DataType
	NativeType string
	Comment    string
}

// StructType represents a complex type with named fields.
type StructType struct {
	Fields []StructField
}

func (t StructType) GlueType() string {
	var fields []string
	for _, f := range t.Fields {
		fields = append(fields, fmt.Sprintf("%s:%s", f.Name, f.Type.GlueType()))
	}
	return fmt.Sprintf("struct<%s>", strings.Join(fields, ","))
}

func (t StructType) String() string {
	var fields []string
	for _, f := range t.Fields {
		fields = append(fields, fmt.Sprintf("%s:%s", f.Name, f.Type.String()))
	}
	return fmt.Sprintf("struct<%s>", strings.Join(fields, ","))
}

func (t StructType) Pretty(indent int, native bool) string {
	pad := strings.Repeat("  ", indent)
	fieldPad := strings.Repeat("  ", indent+1)
	var lines []string
	lines = append(lines, "struct<")
	for i, f := range t.Fields {
		suffix := ","
		if i == len(t.Fields)-1 {
			suffix = ""
		}

		fieldTypeStr := f.Type.Pretty(indent+1, native)
		if native && f.NativeType != "" {
			fieldTypeStr = fmt.Sprintf("%s [%s]", fieldTypeStr, f.NativeType)
		}

		lines = append(lines, fmt.Sprintf("%s%s: %s%s", fieldPad, f.Name, fieldTypeStr, suffix))
	}
	lines = append(lines, pad+">")
	return strings.Join(lines, "\n")
}

func (t StructType) Equal(other DataType) bool {
	o, ok := other.(StructType)
	if !ok || len(t.Fields) != len(o.Fields) {
		return false
	}
	for i := range t.Fields {
		if t.Fields[i].Name != o.Fields[i].Name || !t.Fields[i].Type.Equal(o.Fields[i].Type) {
			return false
		}
	}
	return true
}

// Column represents a table column.
type Column struct {
	Name       string
	Type       DataType
	NativeType string
	Comment    string
}

// TableSchema represents the schema of a Glue table.
type TableSchema struct {
	Database      string
	Table         string
	Columns       []Column
	PartitionKeys []Column
	Location      string
}
