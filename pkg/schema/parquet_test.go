package schema

import (
	"bytes"
	"context"
	"io"
	"testing"

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
