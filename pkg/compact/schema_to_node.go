package compact

import (
	"fmt"

	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

// ColumnsToParquetGroup converts a slice of schema.Column to a parquet.Group.
func ColumnsToParquetGroup(cols []schema.Column) (parquet.Node, error) {
	fields := make(map[string]parquet.Node)
	for _, col := range cols {
		node, err := dataTypeToNode(col.Type)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", col.Name, err)
		}
		fields[col.Name] = node
	}
	return parquet.Group(fields), nil
}

func dataTypeToNode(t schema.DataType) (parquet.Node, error) {
	node, err := dataTypeToNodeInternal(t)
	if err != nil {
		return nil, err
	}
	return parquet.Optional(node), nil
}

func dataTypeToNodeInternal(t schema.DataType) (parquet.Node, error) {
	switch dt := t.(type) {
	case schema.PrimitiveType:
		switch dt.Kind {
		case schema.Boolean:
			return parquet.Leaf(parquet.BooleanType), nil
		case schema.TinyInt, schema.SmallInt, schema.Int:
			return parquet.Leaf(parquet.Int32Type), nil
		case schema.BigInt:
			return parquet.Leaf(parquet.Int64Type), nil
		case schema.Float:
			return parquet.Leaf(parquet.FloatType), nil
		case schema.Double:
			return parquet.Leaf(parquet.DoubleType), nil
		case schema.String:
			return parquet.String().(parquet.Node), nil
		case schema.Binary:
			return parquet.Leaf(parquet.ByteArrayType), nil
		case schema.Date:
			return parquet.Date(), nil
		case schema.Timestamp:
			return parquet.Timestamp(parquet.Millisecond), nil
		default:
			return nil, fmt.Errorf("unsupported primitive kind: %v", dt.Kind)
		}
	case schema.DecimalType:
		return parquet.Decimal(dt.Scale, dt.Precision, parquet.Int64Type).(parquet.Node), nil
	case schema.ArrayType:
		elemNode, err := dataTypeToNode(dt.ElementType)
		if err != nil {
			return nil, err
		}
		return parquet.List(elemNode).(parquet.Node), nil
	case schema.MapType:
		keyNode, err := dataTypeToNodeInternal(dt.KeyType)
		if err != nil {
			return nil, err
		}
		valNode, err := dataTypeToNode(dt.ValueType)
		if err != nil {
			return nil, err
		}
		return parquet.Map(parquet.Required(keyNode), valNode).(parquet.Node), nil
	case schema.StructType:
		fields := make(map[string]parquet.Node)
		for _, f := range dt.Fields {
			n, err := dataTypeToNode(f.Type)
			if err != nil {
				return nil, err
			}
			fields[f.Name] = n
		}
		return parquet.Group(fields), nil
	default:
		return nil, fmt.Errorf("unsupported type: %T", t)
	}
}
