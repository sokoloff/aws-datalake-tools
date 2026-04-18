package schema

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	assert.Contains(t, output, "id")
	assert.Contains(t, output, "bigint")

	buf.Reset()
	err = FormatSchema(&buf, s, true)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "NAME")
	assert.Contains(t, buf.String(), "TYPE")
}

func TestDescribeFile(t *testing.T) {
	ctx := context.Background()
	api := new(mockS3API)
	api.On("HeadObject", ctx, mock.Anything).Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(10)}, nil)
	api.On("GetObject", ctx, mock.Anything).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(nil))}, nil)

	// Open will fail but we want to see it reached
	_, err := DescribeFile(ctx, api, "b", "k")
	assert.Error(t, err)
}

func TestCreateTable_Error(t *testing.T) {
	api := new(mockGlueAPI)
	api.On("CreateTable", mock.Anything, mock.Anything).Return((*glue.CreateTableOutput)(nil), fmt.Errorf("err"))
	err := CreateTable(context.Background(), api, CreateTableInput{})
	assert.Error(t, err)
}

func TestDescribe(t *testing.T) {
	api := new(mockGlueAPI)
	ctx := context.Background()

	api.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{
		Table: &gluetypes.Table{
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Columns:  []gluetypes.Column{},
				Location: aws.String("loc"),
			},
		},
	}, nil)

	out, err := Describe(ctx, api, "db", "tbl")
	assert.NoError(t, err)
	assert.NotNil(t, out.Schema)
}

func TestDescribe_Error(t *testing.T) {
	api := new(mockGlueAPI)
	api.On("GetTable", mock.Anything, mock.Anything).Return((*glue.GetTableOutput)(nil), fmt.Errorf("err"))
	_, err := Describe(context.Background(), api, "db", "tbl")
	assert.Error(t, err)
}

func TestDescribeFile_Success(t *testing.T) {
	ctx := context.Background()
	api := new(mockS3API)

	type Row struct {
		ID int32 `parquet:"id"`
	}
	var buf bytes.Buffer
	writer := parquet.NewWriter(&buf, parquet.SchemaOf(Row{}))
	writer.Write(Row{ID: 1})
	writer.Close()
	data := buf.Bytes()

	api.On("HeadObject", ctx, mock.Anything).Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(int64(len(data)))}, nil)
	api.On("GetObject", ctx, mock.Anything).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil)

	out, err := DescribeFile(ctx, api, "b", "k")
	assert.NoError(t, err)
	assert.NotNil(t, out.Schema)
	assert.Equal(t, "id", out.Schema.Columns[0].Name)
}

func TestDiff_Success(t *testing.T) {
	glueAPI := new(mockGlueAPI)
	s3API := new(mockS3API)
	ctx := context.Background()

	glueAPI.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{
		Table: &gluetypes.Table{
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Columns:  []gluetypes.Column{{Name: aws.String("id"), Type: aws.String("int")}},
				Location: aws.String("loc"),
			},
		},
	}, nil)

	type Row struct {
		ID int32 `parquet:"id"`
	}
	var buf bytes.Buffer
	writer := parquet.NewWriter(&buf, parquet.SchemaOf(Row{}))
	writer.Write(Row{ID: 1})
	writer.Close()
	data := buf.Bytes()

	s3API.On("HeadObject", ctx, mock.Anything).Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(int64(len(data)))}, nil)
	s3API.On("GetObject", ctx, mock.Anything).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil)

	out, err := Diff(ctx, glueAPI, s3API, "db", "tbl", "b", "k")
	assert.NoError(t, err)
	assert.True(t, out.Plan.Compatible)
}


func TestDiff_ErrorFile(t *testing.T) {
	glueAPI := new(mockGlueAPI)
	s3API := new(mockS3API)
	ctx := context.Background()

	glueAPI.On("GetTable", ctx, mock.Anything).Return(&glue.GetTableOutput{
		Table: &gluetypes.Table{
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Columns: []gluetypes.Column{{Name: aws.String("c1"), Type: aws.String("int")}},
			},
		},
	}, nil)

	s3API.On("HeadObject", ctx, mock.Anything).Return((*s3.HeadObjectOutput)(nil), fmt.Errorf("err"))

	_, err := Diff(ctx, glueAPI, s3API, "db", "tbl", "b", "k")
	assert.Error(t, err)
}


func TestDiff_ErrorFetch(t *testing.T) {
	glueAPI := new(mockGlueAPI)
	glueAPI.On("GetTable", mock.Anything, mock.Anything).Return((*glue.GetTableOutput)(nil), fmt.Errorf("err"))
	_, err := Diff(context.Background(), glueAPI, nil, "db", "tbl", "b", "k")
	assert.Error(t, err)
}

func TestFormatSchemaPretty(t *testing.T) {
	s := &TableSchema{
		Columns: []Column{
			{Name: "c1", Type: PrimitiveType{Int}, Comment: "comm"},
			{Name: "c2", Type: PrimitiveType{Int}},
		},
		PartitionKeys: []Column{{Name: "p1", Type: PrimitiveType{String}}},
	}
	assert.NoError(t, FormatSchemaPretty(io.Discard, s, true))
}

func TestFormatDiff(t *testing.T) {
	d := &DiffOutput{
		Plan: &CoercionPlan{
			Compatible: false,
			Diffs: []ColumnDiff{
				{Kind: DiffTypeMismatch, Column: "c1", GlueType: PrimitiveType{Int}, FileType: PrimitiveType{String}},
				{Kind: DiffMissingInFile, Column: "c2", GlueType: PrimitiveType{Int}},
				{Kind: DiffExtraInFile, Column: "c3", FileType: PrimitiveType{Int}},
			},
		},
	}
	assert.NoError(t, FormatDiff(io.Discard, d))

	d.Plan.Compatible = true
	d.Plan.Diffs = nil
	assert.NoError(t, FormatDiff(io.Discard, d))
}
