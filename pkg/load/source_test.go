package load

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockS3 struct {
	mock.Mock
}

func (m *mockS3) GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.GetObjectOutput), args.Error(1)
}

func (m *mockS3) PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return nil, nil
}

func (m *mockS3) DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return nil, nil
}

func (m *mockS3) HeadObject(ctx context.Context, input *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return nil, nil
}

func (m *mockS3) ListObjectsV2(ctx context.Context, input *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return nil, nil
}

func TestLocalDumpSource(t *testing.T) {
	src := &LocalDumpSource{Root: "testdata"}

	summary, err := src.ReadManifestSummary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), summary.ItemCount)

	entries, err := src.ReadManifestFiles(context.Background())
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "AWSDynamoDB/0123/data/part-00000.json.gz", entries[0].DataFileS3Key)

	rc, err := src.OpenDataFile(context.Background(), entries[0].DataFileS3Key)
	require.NoError(t, err)
	defer rc.Close()
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestS3DumpSource(t *testing.T) {
	m := new(mockS3)
	src := &S3DumpSource{S3: m, Bucket: "b", Prefix: "p"}

	// Test ReadManifestSummary
	m.On("GetObject", mock.Anything, mock.MatchedBy(func(i *s3.GetObjectInput) bool {
		return *i.Key == "p/manifest-summary.json"
	})).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"itemCount":5}`))),
	}, nil)

	summary, err := src.ReadManifestSummary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(5), summary.ItemCount)

	// Test ReadManifestFiles
	m.On("GetObject", mock.Anything, mock.MatchedBy(func(i *s3.GetObjectInput) bool {
		return *i.Key == "p/manifest-files.json"
	})).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("{\"itemCount\":10,\"dataFileS3Key\":\"f1\"}\n"))),
	}, nil)


	entries, err := src.ReadManifestFiles(context.Background())
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "f1", entries[0].DataFileS3Key)

	// Test OpenDataFile
	m.On("GetObject", mock.Anything, mock.MatchedBy(func(i *s3.GetObjectInput) bool {
		return *i.Key == "f1"
	})).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("data"))),
	}, nil)

	rc, err := src.OpenDataFile(context.Background(), "f1")
	require.NoError(t, err)
	defer rc.Close()
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))

	// Test Describe
	assert.Equal(t, "s3://b/p", src.Describe())
	m.AssertExpectations(t)
}

func TestNewDumpSource(t *testing.T) {
	m := new(mockS3)

	src, err := NewDumpSource("s3://bucket/prefix", m)
	require.NoError(t, err)
	assert.IsType(t, &S3DumpSource{}, src)

	src, err = NewDumpSource("./local", m)
	require.NoError(t, err)
	assert.IsType(t, &LocalDumpSource{}, src)
	assert.Equal(t, "local://./local", src.Describe())

	_, err = NewDumpSource("s3://", m)
	assert.Error(t, err)
}

func TestLocalDumpSource_Errors(t *testing.T) {
	src := &LocalDumpSource{Root: "non-existent"}
	_, err := src.ReadManifestSummary(context.Background())
	assert.Error(t, err)
	_, err = src.ReadManifestFiles(context.Background())
	assert.Error(t, err)
	_, err = src.OpenDataFile(context.Background(), "f")
	assert.Error(t, err)
}

func TestS3DumpSource_Errors(t *testing.T) {
	m := new(mockS3)
	src := &S3DumpSource{S3: m, Bucket: "b", Prefix: "p"}

	m.On("GetObject", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("s3 error"))

	_, err := src.ReadManifestSummary(context.Background())
	assert.Error(t, err)
	_, err = src.ReadManifestFiles(context.Background())
	assert.Error(t, err)
	_, err = src.OpenDataFile(context.Background(), "f")
	assert.Error(t, err)
}
