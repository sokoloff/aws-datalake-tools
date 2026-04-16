package schema

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/parquet-go/parquet-go"
)

// S3GetObjectAPI defines the subset of S3 API methods used for reading parquet files.
type S3GetObjectAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

type s3ReaderAt struct {
	ctx    context.Context
	api    S3GetObjectAPI
	bucket string
	key    string
}

func (r *s3ReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	rangeHeader := fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))-1)
	resp, err := r.api.GetObject(r.ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.key),
		Range:  aws.String(rangeHeader),
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return io.ReadFull(resp.Body, p)
}

// ReadParquetSchemaFromS3 reads only the footer of a parquet file from S3 to extract its schema.
func ReadParquetSchemaFromS3(ctx context.Context, api S3GetObjectAPI, bucket, key string) ([]Column, error) {
	head, err := api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("head object: %w", err)
	}

	reader := &s3ReaderAt{ctx: ctx, api: api, bucket: bucket, key: key}
	file, err := parquet.OpenFile(reader, *head.ContentLength)
	if err != nil {
		return nil, fmt.Errorf("opening parquet file: %w", err)
	}

	return ParquetSchemaToColumns(file.Schema())
}

// ParquetSchemaToColumns converts a parquet.Schema to a slice of Columns.
func ParquetSchemaToColumns(schema *parquet.Schema) ([]Column, error) {
	var cols []Column
	for _, field := range schema.Fields() {
		dt, err := parquetNodeToDataType(field)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name(), err)
		}
		cols = append(cols, Column{
			Name:       field.Name(),
			Type:       dt,
			NativeType: describeNativeParquetType(field),
		})
	}
	return cols, nil
}

func describeNativeParquetType(node parquet.Field) string {
	if len(node.Fields()) > 0 {
		return "group"
	}
	t := node.Type()
	res := t.Kind().String()
	if lt := t.LogicalType(); lt != nil {
		res += fmt.Sprintf(" (%v)", lt)
	}
	return res
}

func parquetNodeToDataType(node parquet.Field) (DataType, error) {
	dt, err := parquetNodeToDataTypeInternal(node)
	if err != nil {
		return nil, err
	}

	if node.Repeated() {
		// If it's a repeated field and NOT already identified as an array (via 3-level convention)
		// then wrap it in an ArrayType.
		if _, ok := dt.(ArrayType); !ok {
			return ArrayType{ElementType: dt}, nil
		}
	}
	return dt, nil
}

func parquetNodeToDataTypeInternal(node parquet.Field) (DataType, error) {
	lt := node.Type().LogicalType()
	switch {
	case lt != nil && lt.UTF8 != nil:
		return PrimitiveType{Kind: String}, nil
	case lt != nil && lt.Integer != nil:
		it := lt.Integer
		if it.IsSigned {
			switch it.BitWidth {
			case 8:
				return PrimitiveType{Kind: TinyInt}, nil
			case 16:
				return PrimitiveType{Kind: SmallInt}, nil
			case 32:
				return PrimitiveType{Kind: Int}, nil
			case 64:
				return PrimitiveType{Kind: BigInt}, nil
			}
		}
	case lt != nil && lt.Decimal != nil:
		return DecimalType{Precision: int(lt.Decimal.Precision), Scale: int(lt.Decimal.Scale)}, nil
	case lt != nil && lt.Date != nil:
		return PrimitiveType{Kind: Date}, nil
	case lt != nil && lt.Timestamp != nil:
		return PrimitiveType{Kind: Timestamp}, nil
	}

	// Handle complex types (Struct, List, Map)
	if len(node.Fields()) > 0 {
		// Parquet-go represents groups as nodes with children.
		// Check for List/Map conventions.
		if lt != nil {
			if lt.List != nil {
				// 3-level list convention: list -> element -> item
				// parquet-go might simplify this, but let's be careful.
				if len(node.Fields()) == 1 {
					repeated := node.Fields()[0]
					if len(repeated.Fields()) == 1 {
						element := repeated.Fields()[0]
						et, err := parquetNodeToDataType(element)
						if err != nil {
							return nil, err
						}
						return ArrayType{ElementType: et}, nil
					}
				}
			}
			if lt.Map != nil {
				// 3-level map convention: map -> key_value -> (key, value)
				if len(node.Fields()) == 1 {
					keyValue := node.Fields()[0]
					if len(keyValue.Fields()) == 2 {
						kt, err := parquetNodeToDataType(keyValue.Fields()[0])
						if err != nil {
							return nil, err
						}
						vt, err := parquetNodeToDataType(keyValue.Fields()[1])
						if err != nil {
							return nil, err
						}
						return MapType{KeyType: kt, ValueType: vt}, nil
					}
				}
			}
		}

		// Default to Struct
		var fields []StructField
		for _, f := range node.Fields() {
			ft, err := parquetNodeToDataType(f)
			if err != nil {
				return nil, err
			}
			fields = append(fields, StructField{
				Name:       f.Name(),
				Type:       ft,
				NativeType: describeNativeParquetType(f),
			})
		}
		return StructType{Fields: fields}, nil
	}

	// Fallback to physical types if logical type is missing or unhandled
	switch node.Type().Kind() {
	case parquet.Boolean:
		return PrimitiveType{Kind: Boolean}, nil
	case parquet.Int32:
		return PrimitiveType{Kind: Int}, nil
	case parquet.Int64:
		return PrimitiveType{Kind: BigInt}, nil
	case parquet.Float:
		return PrimitiveType{Kind: Float}, nil
	case parquet.Double:
		return PrimitiveType{Kind: Double}, nil
	case parquet.ByteArray, parquet.FixedLenByteArray:
		return PrimitiveType{Kind: Binary}, nil
	}

	return nil, fmt.Errorf("unsupported parquet type: %v", node.Type())
}
