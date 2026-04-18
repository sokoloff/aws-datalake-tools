package s3util

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseS3URI(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{
			name:       "full path",
			input:      "s3://my-bucket/path/to/file.parquet",
			wantBucket: "my-bucket",
			wantKey:    "path/to/file.parquet",
		},
		{
			name:       "trailing slash",
			input:      "s3://my-bucket/prefix/",
			wantBucket: "my-bucket",
			wantKey:    "prefix/",
		},
		{
			name:       "bucket only with slash",
			input:      "s3://my-bucket/",
			wantBucket: "my-bucket",
			wantKey:    "",
		},
		{
			name:       "bucket only no slash",
			input:      "s3://my-bucket",
			wantBucket: "my-bucket",
			wantKey:    "",
		},
		{
			name:       "nested path",
			input:      "s3://my-bucket/a/b/c/d.parquet",
			wantBucket: "my-bucket",
			wantKey:    "a/b/c/d.parquet",
		},
		{
			name:    "https scheme",
			input:   "https://my-bucket/path",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "just scheme",
			input:   "s3://",
			wantErr: true,
		},
		{
			name:    "empty bucket",
			input:   "s3:///key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key, err := ParseS3URI(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBucket, bucket)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}

func TestFormatS3URI(t *testing.T) {
	assert.Equal(t, "s3://bucket/key/path", FormatS3URI("bucket", "key/path"))
	assert.Equal(t, "s3://bucket", FormatS3URI("bucket", ""))
}

func TestFormatS3URI_RoundTrip(t *testing.T) {
	uris := []string{
		"s3://my-bucket/path/to/file.parquet",
		"s3://my-bucket",
	}
	for _, uri := range uris {
		bucket, key, err := ParseS3URI(uri)
		require.NoError(t, err)
		assert.Equal(t, uri, FormatS3URI(bucket, key))
	}
}

// mockListObjects implements ListObjectsAPI for testing.
type mockListObjects struct {
	pages []s3.ListObjectsV2Output
	calls int
	err   error
}

func (m *mockListObjects) ListObjectsV2(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.err != nil {
		return nil, m.err
	}
	page := m.pages[m.calls]
	m.calls++
	return &page, nil
}

func TestListObjects(t *testing.T) {
	now := time.Now()

	mock := &mockListObjects{
		pages: []s3.ListObjectsV2Output{
			{
				Contents: []s3types.Object{
					{Key: aws.String("prefix/a.parquet"), Size: aws.Int64(100), LastModified: &now},
					{Key: aws.String("prefix/b.parquet"), Size: aws.Int64(200), LastModified: &now},
				},
				IsTruncated:       aws.Bool(true),
				NextContinuationToken: aws.String("token1"),
			},
			{
				Contents: []s3types.Object{
					{Key: aws.String("prefix/c.parquet"), Size: aws.Int64(300), LastModified: &now},
				},
				IsTruncated: aws.Bool(false),
			},
		},
	}

	objects, err := ListObjects(context.Background(), mock, "bucket", "prefix/")
	require.NoError(t, err)
	assert.Len(t, objects, 3)
	assert.Equal(t, "prefix/a.parquet", objects[0].Key)
	assert.Equal(t, "prefix/c.parquet", objects[2].Key)
	assert.Equal(t, int64(200), objects[1].Size)
	assert.Equal(t, 2, mock.calls)
}

func TestListObjects_Error(t *testing.T) {
	mock := &mockListObjects{
		err: fmt.Errorf("s3 error"),
	}

	objects, err := ListObjects(context.Background(), mock, "bucket", "prefix/")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "s3 error")
	assert.Nil(t, objects)
}

func TestListObjects_Empty(t *testing.T) {
	mock := &mockListObjects{
		pages: []s3.ListObjectsV2Output{
			{
				Contents:    []s3types.Object{},
				IsTruncated: aws.Bool(false),
			},
		},
	}

	objects, err := ListObjects(context.Background(), mock, "bucket", "prefix/")
	require.NoError(t, err)
	assert.Empty(t, objects)
}
