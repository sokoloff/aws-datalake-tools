package compact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/parquet-go/parquet-go"
)

type fakeS3 struct {
	objects map[string][]byte
	putErr  error
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: make(map[string][]byte)}
}

func (f *fakeS3) put(bucket, key string, data []byte) {
	f.objects[bucket+"/"+key] = data
}

func (f *fakeS3) get(bucket, key string) ([]byte, bool) {
	data, ok := f.objects[bucket+"/"+key]
	return data, ok
}

func (f *fakeS3) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	data, ok := f.get(*params.Bucket, *params.Key)
	if !ok {
		return nil, fmt.Errorf("NoSuchKey")
	}

	start, end := int64(0), int64(len(data)-1)
	if params.Range != nil {
		fmt.Sscanf(*params.Range, "bytes=%d-%d", &start, &end)
	}

	if start > int64(len(data)) {
		start = int64(len(data))
	}
	if end >= int64(len(data)) {
		end = int64(len(data) - 1)
	}
	if start > end {
		return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}

	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(data[start : end+1])),
	}, nil
}

func (f *fakeS3) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	data, ok := f.get(*params.Bucket, *params.Key)
	if !ok {
		return nil, fmt.Errorf("NoSuchKey")
	}
	return &s3.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(data))),
	}, nil
}

func (f *fakeS3) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := ""
	if params.Prefix != nil {
		prefix = *params.Prefix
	}

	var contents []types.Object
	for k, v := range f.objects {
		if strings.HasPrefix(k, *params.Bucket+"/"+prefix) {
			key := strings.TrimPrefix(k, *params.Bucket+"/")
			contents = append(contents, types.Object{
				Key:  aws.String(key),
				Size: aws.Int64(int64(len(v))),
			})
		}
	}
	return &s3.ListObjectsV2Output{Contents: contents}, nil
}

func (f *fakeS3) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	f.put(*params.Bucket, *params.Key, data)
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	delete(f.objects, *params.Bucket+"/"+*params.Key)
	return &s3.DeleteObjectOutput{}, nil
}

func TestCompact_DryRun(t *testing.T) {
	ctx := context.Background()
	s3api := newFakeS3()

	// Write a dummy parquet file
	type row struct {
		ID int64
	}
	buf := new(bytes.Buffer)
	pw := parquet.NewGenericWriter[row](buf)
	pw.Write([]row{{ID: 1}, {ID: 2}})
	pw.Close()

	s3api.put("bucket", "src/file1.parquet", buf.Bytes())

	cfg := Config{
		SourceURI: "s3://bucket/src/",
		TargetURI: "s3://bucket/tgt/",
		DryRun:    true,
	}

	report, err := RunWithDeps(ctx, cfg, Deps{S3: s3api})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.DryRun {
		t.Error("expected DryRun=true")
	}

	// Ensure no targets were written
	for k := range s3api.objects {
		if strings.HasPrefix(k, "bucket/tgt/") {
			t.Errorf("expected no files written, found %s", k)
		}
	}
}

func TestCompact_NoGlue_PassThrough(t *testing.T) {
	ctx := context.Background()
	s3api := newFakeS3()

	type row struct {
		ID   int64  `parquet:"id"`
		Name string `parquet:"name"`
	}

	// Create 3 identical files
	for i := 0; i < 3; i++ {
		buf := new(bytes.Buffer)
		pw := parquet.NewGenericWriter[row](buf)
		for j := 0; j < 5; j++ {
			pw.Write([]row{{ID: int64(i*5 + j), Name: fmt.Sprintf("n-%d", i*5+j)}})
		}
		pw.Close()
		s3api.put("bucket", fmt.Sprintf("src/file%d.parquet", i), buf.Bytes())
	}

	cfg := Config{
		SourceURI:    "s3://bucket/src/",
		TargetURI:    "s3://bucket/tgt/",
		TargetSizeMB: 128,
		DeleteSource: true,
	}

	report, err := RunWithDeps(ctx, cfg, Deps{S3: s3api})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.RowsRead != 15 {
		t.Errorf("expected 15 rows read, got %d", report.RowsRead)
	}
	if report.RowsWritten != 15 {
		t.Errorf("expected 15 rows written, got %d", report.RowsWritten)
	}
	if len(report.OutputFiles) != 1 {
		t.Errorf("expected 1 output file, got %d", len(report.OutputFiles))
	}
	if len(report.DeletedSources) != 3 {
		t.Errorf("expected 3 deleted sources, got %d", len(report.DeletedSources))
	}
}
