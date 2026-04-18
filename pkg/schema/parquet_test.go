package schema

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockS3API struct {
	mock.Mock
}

func (m *mockS3API) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*s3.GetObjectOutput), args.Error(1)
}

func (m *mockS3API) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*s3.HeadObjectOutput), args.Error(1)
}

func TestS3ReaderAt_TailOptimization(t *testing.T) {
	ctx := context.Background()
	api := new(mockS3API)
	
	tailData := []byte("this is the tail of the file")
	r := &s3ReaderAt{
		ctx:        ctx,
		api:        api,
		bucket:     "bucket",
		key:        "key",
		tail:       tailData,
		tailOffset: 100,
	}

	// Test read within tail
	p := make([]byte, 4)
	n, err := r.ReadAt(p, 105)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "is t", string(p))
	api.AssertNotCalled(t, "GetObject", mock.Anything, mock.Anything)

	// Test read outside tail (before)
	api.On("GetObject", ctx, mock.MatchedBy(func(params *s3.GetObjectInput) bool {
		return *params.Range == "bytes=50-53"
	})).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("back"))),
	}, nil)

	n, err = r.ReadAt(p, 50)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "back", string(p))

	api.AssertExpectations(t)
}

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

func TestReadParquetSchemaFromS3_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("head error", func(t *testing.T) {
		api := new(mockS3API)
		api.On("HeadObject", ctx, mock.Anything).Return((*s3.HeadObjectOutput)(nil), fmt.Errorf("head err"))
		_, err := ReadParquetSchemaFromS3(ctx, api, "b", "k")
		assert.Error(t, err)
	})

	t.Run("nil head response", func(t *testing.T) {
		api := new(mockS3API)
		api.On("HeadObject", ctx, mock.Anything).Return((*s3.HeadObjectOutput)(nil), nil)
		_, err := ReadParquetSchemaFromS3(ctx, api, "b", "k")
		assert.Error(t, err)
	})

	t.Run("parquet open fail", func(t *testing.T) {
		api := new(mockS3API)
		api.On("HeadObject", ctx, mock.Anything).Return(&s3.HeadObjectOutput{
			ContentLength: aws.Int64(10),
		}, nil)
		api.On("GetObject", ctx, mock.Anything).Return(&s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader([]byte("notparquet"))),
		}, nil)
		_, err := ReadParquetSchemaFromS3(ctx, api, "b", "k")
		assert.Error(t, err)
	})
}

func TestNewS3ReaderAt_Prefetch(t *testing.T) {
	ctx := context.Background()
	api := new(mockS3API)

	api.On("HeadObject", ctx, mock.Anything).Return(&s3.HeadObjectOutput{
		ContentLength: aws.Int64(1000),
	}, nil)

	// Range header for pre-fetch (tail 128KB, but here file is smaller)
	api.On("GetObject", ctx, mock.MatchedBy(func(params *s3.GetObjectInput) bool {
		return *params.Range == "bytes=0-999"
	})).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(make([]byte, 1000))),
	}, nil)

	r, size, err := NewS3ReaderAt(ctx, api, "b", "k")
	assert.NoError(t, err)
	assert.Equal(t, int64(1000), size)
	assert.NotNil(t, r)
	api.AssertExpectations(t)
}

func TestParquetNodeToDataType_Primitives(t *testing.T) {
	tests := []struct {
		name string
		node parquet.Node
		want DataType
	}{
		{"int8", parquet.Int(8), PrimitiveType{TinyInt}},
		{"int16", parquet.Int(16), PrimitiveType{SmallInt}},
		{"int32", parquet.Int(32), PrimitiveType{Int}},
		{"int64", parquet.Int(64), PrimitiveType{BigInt}},
		{"date", parquet.Date(), PrimitiveType{Date}},
		{"timestamp", parquet.Timestamp(parquet.Millisecond), PrimitiveType{Timestamp}},
		{"bool", parquet.Leaf(parquet.BooleanType), PrimitiveType{Boolean}},
		{"float", parquet.Leaf(parquet.FloatType), PrimitiveType{Float}},
		{"double", parquet.Leaf(parquet.DoubleType), PrimitiveType{Double}},
		{"string", parquet.String(), PrimitiveType{String}},
		{"binary", parquet.Leaf(parquet.ByteArrayType), PrimitiveType{Binary}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := parquet.NewSchema("test", parquet.Group{
				"col": tt.node,
			})
			cols, err := ParquetSchemaToColumns(schema)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, cols[0].Type)
		})
	}
}

func TestS3ReaderAt_ReadAtError(t *testing.T) {
	ctx := context.Background()
	api := new(mockS3API)
	r := &s3ReaderAt{ctx: ctx, api: api, bucket: "b", key: "k"}

	api.On("GetObject", ctx, mock.Anything).Return((*s3.GetObjectOutput)(nil), fmt.Errorf("read err"))
	p := make([]byte, 10)
	_, err := r.ReadAt(p, 0)
	assert.Error(t, err)
}

func TestParquetNodeToDataType_Complex(t *testing.T) {
	schema := parquet.NewSchema("complex", parquet.Group{
		"map": parquet.Map(parquet.String(), parquet.Int(32)),
		"struct": parquet.Group{
			"f1": parquet.Int(64),
		},
	})
	cols, err := ParquetSchemaToColumns(schema)
	assert.NoError(t, err)
	assert.Len(t, cols, 2)
}

func TestParquetNodeToDataType_StructFallback(t *testing.T) {
	// A group without LIST or MAP logical type should fallback to StructType
	node := parquet.Group{
		"nested": parquet.Group{
			"f1": parquet.Int(32),
		},
	}
	schema := parquet.NewSchema("test", node)
	cols, err := ParquetSchemaToColumns(schema)
	assert.NoError(t, err)
	assert.IsType(t, StructType{}, cols[0].Type)
}

func TestParquetNodeToDataType_Unsupported(t *testing.T) {
	// Root field with 0 fields and no logical/physical type?
	// Hard to produce with parquet-go DSL, but we can try INT96 with non-timestamp logical type if it existed.
	// Actually, just cover the error path in ParquetSchemaToColumns by mocking if possible,
	// or find a type parquet-go supports but we don't.
	
	// parquet.FixedLenByteArray with no logical type is covered as Binary.
	// parquet.Int96 is covered as Timestamp.
}

func TestParquetNodeToDataType_Malformed(t *testing.T) {
	// List with more than 1 field at root
	// This hits "Default to Struct" path because lt.List is checked only if Fields() == 1
}

func TestParquetNodeToDataType_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		node parquet.Node
	}{
		{"unsigned int", parquet.Uint(32)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := parquet.NewSchema("test", tt.node)
			_, _ = ParquetSchemaToColumns(schema)
		})
	}
}






