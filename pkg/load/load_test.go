package load

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockGlue struct {
	mock.Mock
}

func (m *mockGlue) GetTable(ctx context.Context, input *glue.GetTableInput, optFns ...func(*glue.Options)) (*glue.GetTableOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*glue.GetTableOutput), args.Error(1)
}

func (m *mockGlue) CreateTable(ctx context.Context, input *glue.CreateTableInput, optFns ...func(*glue.Options)) (*glue.CreateTableOutput, error) {
	args := m.Called(ctx, input)
	return nil, args.Error(1)
}

func (m *mockGlue) DeleteTable(ctx context.Context, input *glue.DeleteTableInput, optFns ...func(*glue.Options)) (*glue.DeleteTableOutput, error) {
	args := m.Called(ctx, input)
	return nil, args.Error(1)
}

func (m *mockGlue) UpdateTable(ctx context.Context, input *glue.UpdateTableInput, optFns ...func(*glue.Options)) (*glue.UpdateTableOutput, error) {
	args := m.Called(ctx, input)
	return nil, args.Error(1)
}

func TestLoad_FullRun(t *testing.T) {
	tmpOut, err := os.MkdirTemp("", "load-test-out-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpOut)

	mg := new(mockGlue)
	cfg := Config{
		InputURI:  "testdata",
		OutputURI: tmpOut,
		Database:  "db",
		Table:     "table",
		Partition: "none",
	}

	mg.On("CreateTable", mock.Anything, mock.Anything).Return(nil, nil)

	deps := Deps{
		S3:   new(mockS3), // not used for local dump
		Glue: mg,
	}

	report, err := RunWithDeps(context.Background(), cfg, deps)
	require.NoError(t, err)

	assert.Equal(t, int64(3), report.RecordsRead)
	assert.Len(t, report.OutputFiles, 1)

	outPath := filepath.Join(tmpOut, filepath.Base(report.OutputFiles[0]))
	_, err = os.Stat(outPath)
	assert.NoError(t, err)

	mg.AssertExpectations(t)
	}

	func TestLoad_InferOnly(t *testing.T) {
	cfg := Config{
		InputURI:  "testdata",
		OutputURI: "s3://ignored/",
		InferOnly: true,
		Partition: "auto",
	}
	deps := Deps{
		S3:   new(mockS3),
		Glue: new(mockGlue),
	}

	report, err := RunWithDeps(context.Background(), cfg, deps)
	require.NoError(t, err)
	assert.Equal(t, int64(3), report.RecordsRead)
	assert.Len(t, report.Schema, 13) // id, str, int, float, bool, null, bin, ss, ns, bs, list, map, extra
	}

func TestLoad_WithSchemaFile(t *testing.T) {
	schemaFile := filepath.Join(t.TempDir(), "schema.json")
	intermediate := []map[string]string{
		{"name": "id", "type": "string"},
	}
	data, _ := json.Marshal(intermediate)
	os.WriteFile(schemaFile, data, 0644)

	tmpOut := t.TempDir()
	mg := new(mockGlue)
	mg.On("CreateTable", mock.Anything, mock.Anything).Return(nil, nil)

	cfg := Config{
		InputURI:   "testdata",
		OutputURI:  tmpOut,
		SchemaFile: schemaFile,
		Database:   "db",
		Table:      "table",
		Partition:  "none",
	}
	deps := Deps{
		S3:   new(mockS3),
		Glue: mg,
	}

	_, err := RunWithDeps(context.Background(), cfg, deps)
	require.NoError(t, err)
	mg.AssertExpectations(t)
}

func TestLoad_S3Output(t *testing.T) {
	ms := new(mockS3)
	// mock manifest reads
	ms.On("GetObject", mock.Anything, mock.MatchedBy(func(i *s3.GetObjectInput) bool {
		return *i.Key == "p/manifest-summary.json"
	})).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"exportTime":"2024-04-18T10:00:00Z"}`))),
	}, nil).Once()
	ms.On("GetObject", mock.Anything, mock.MatchedBy(func(i *s3.GetObjectInput) bool {
		return *i.Key == "p/manifest-files.json"
	})).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("{\"dataFileS3Key\":\"f1\"}\n"))),
	}, nil).Once()
	// mock data read
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte(`{"Item":{"id":{"S":"123"}}}`))
	gw.Close()

	ms.On("GetObject", mock.Anything, mock.MatchedBy(func(i *s3.GetObjectInput) bool {
		return *i.Key == "f1"
	})).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(buf.Bytes())),
	}, nil).Once()


	cfg := Config{
		InputURI:  "s3://b/p",
		OutputURI: "s3://out/p",
		Partition: "none",
	}
	deps := Deps{
		S3:   ms,
		Glue: new(mockGlue),
	}

	_, err := RunWithDeps(context.Background(), cfg, deps)
	require.NoError(t, err)
}

func TestFormatReport(t *testing.T) {
	r := &Report{
		RecordsRead: 100,
		OutputFiles: []string{"f1"},
	}
	err := FormatReport(os.Stdout, r)
	assert.NoError(t, err)
}

func TestLoad_Errors(t *testing.T) {
	mg := new(mockGlue)
	ms := new(mockS3)

	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "invalid input uri",
			cfg:  Config{InputURI: "s3://"},
		},
		{
			name: "bad schema file",
			cfg:  Config{InputURI: "testdata", SchemaFile: "non-existent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := Deps{S3: ms, Glue: mg}
			_, err := RunWithDeps(context.Background(), tt.cfg, deps)
			assert.Error(t, err)
		})
	}
}
