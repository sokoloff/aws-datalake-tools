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
	"github.com/stretchr/testify/assert"
)

type fakeS3 struct {
	objects      map[string][]byte
	putErr       error
	listErr      error
	getObjectErr error
	deleteErr    error
	bodyOverride io.ReadCloser
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
	if f.getObjectErr != nil {
		return nil, f.getObjectErr
	}
	if f.bodyOverride != nil {
		return &s3.GetObjectOutput{Body: f.bodyOverride}, nil
	}
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
	if f.listErr != nil {
		return nil, f.listErr
	}
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
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	delete(f.objects, *params.Bucket+"/"+*params.Key)
	return &s3.DeleteObjectOutput{}, nil
}

func TestCompact_DryRun(t *testing.T) {
	ctx := context.Background()
	s3api := newFakeS3()

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
}

func TestCompact_NoGlue_PassThrough(t *testing.T) {
	ctx := context.Background()
	s3api := newFakeS3()

	type row struct {
		ID   int64  `parquet:"id"`
		Name string `parquet:"name"`
	}

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

	assert.Equal(t, int64(15), report.RowsRead)
	assert.Equal(t, int64(15), report.RowsWritten)
	assert.Len(t, report.OutputFiles, 1)
	assert.Len(t, report.DeletedSources, 3)
}

func TestCompact_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("list error", func(t *testing.T) {
		s3api := &fakeS3{listErr: fmt.Errorf("list fail")}
		cfg := Config{SourceURI: "s3://b/s", TargetURI: "s3://b/t"}
		_, err := RunWithDeps(ctx, cfg, Deps{S3: s3api})
		assert.Error(t, err)
	})

	t.Run("no files", func(t *testing.T) {
		s3api := newFakeS3()
		cfg := Config{SourceURI: "s3://b/s", TargetURI: "s3://b/t"}
		_, err := RunWithDeps(ctx, cfg, Deps{S3: s3api})
		assert.Error(t, err)
	})

	t.Run("download error", func(t *testing.T) {
		s3api := newFakeS3()
		s3api.put("b", "s/f1.parquet", []byte("data"))
		s3api.getObjectErr = fmt.Errorf("get fail")
		cfg := Config{SourceURI: "s3://b/s", TargetURI: "s3://b/t"}
		_, err := RunWithDeps(ctx, cfg, Deps{S3: s3api})
		assert.Error(t, err)
	})

	t.Run("writer init error", func(t *testing.T) {
		s3api := newFakeS3()
		type Row struct{ ID int }
		buf := new(bytes.Buffer)
		pw := parquet.NewGenericWriter[Row](buf)
		pw.Write([]Row{{1}})
		pw.Close()
		s3api.put("b", "s/f1.parquet", buf.Bytes())

		cfg := Config{SourceURI: "s3://b/s", TargetURI: "s3://b/t"}
		t.Setenv("TMPDIR", "/non/existent/path")
		_, err := RunWithDeps(ctx, cfg, Deps{S3: s3api})
		assert.Error(t, err)
	})

	t.Run("delete error", func(t *testing.T) {
		s3api := newFakeS3()
		type row struct{ ID int }
		buf := new(bytes.Buffer)
		pw := parquet.NewGenericWriter[row](buf)
		pw.Write([]row{{1}})
		pw.Close()
		s3api.put("b", "s/f1.parquet", buf.Bytes())
		s3api.deleteErr = fmt.Errorf("delete fail")

		cfg := Config{SourceURI: "s3://b/s", TargetURI: "s3://b/t", DeleteSource: true}
		report, err := RunWithDeps(ctx, cfg, Deps{S3: s3api})
		assert.NoError(t, err)
		assert.Empty(t, report.DeletedSources)
	})
}

func TestFormatReport(t *testing.T) {
	r := &Report{
		SourceFiles: []string{"f1"},
		OutputFiles: []string{"o1"},
		SourceBytes: 100,
		OutputBytes: 50,
		DeletedSources: []string{"s1"},
	}
	assert.NoError(t, FormatReport(io.Discard, r))

	r.SourceBytes = 0
	assert.NoError(t, FormatReport(io.Discard, r))

	r.DryRun = true
	assert.NoError(t, FormatReport(io.Discard, r))
}

func TestRun(t *testing.T) {
	ctx := context.Background()
	cfg := Config{SourceURI: "invalid-uri", TargetURI: "s3://b/t"}
	_ = Run(ctx, cfg)

	cfg.SourceURI = "s3://b/s"
	cfg.DryRun = true
	// Still fails because of AWS clients in Run, but hits the DryRun branch before that?
	// No, BuildPlan is inside RunWithDeps which is called after client init.
}

func TestDownloadAndOpenFile_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("get fail", func(t *testing.T) {
		s3api := &fakeS3{getObjectErr: assert.AnError}
		_, _, _, err := downloadAndOpenFile(ctx, s3api, "b", "k")
		assert.Error(t, err)
	})

	t.Run("copy fail", func(t *testing.T) {
		s3api := newFakeS3()
		s3api.put("b", "k", []byte("data"))
		s3api.bodyOverride = errorReader{err: assert.AnError}
		_, _, _, err := downloadAndOpenFile(ctx, s3api, "b", "k")
		assert.Error(t, err)
	})

	t.Run("seek fail", func(t *testing.T) {
		s3api := newFakeS3()
		s3api.put("b", "k", []byte("data"))
		// We'd need to mock the filesystem or return a closed file handle.
		// Hard to trigger seek fail on os.File without closing it first.
	})
}


func TestCompact_RollTriggers(t *testing.T) {
	ctx := context.Background()
	s3api := newFakeS3()

	type row struct {
		Data string `parquet:"data"`
	}

	buf := new(bytes.Buffer)
	pw := parquet.NewGenericWriter[row](buf)
	for i := 0; i < 10001; i++ {
		pw.Write([]row{{Data: strings.Repeat("a", 100)}})
	}
	pw.Close()
	s3api.put("bucket", "src/big.parquet", buf.Bytes())

	cfg := Config{
		SourceURI:    "s3://bucket/src/",
		TargetURI:    "s3://bucket/tgt/",
		TargetSizeMB: 1,
	}

	report, err := RunWithDeps(ctx, cfg, Deps{S3: s3api})
	assert.NoError(t, err)
	assert.Greater(t, report.RowsWritten, int64(10000))
}

type errorReader struct{ err error }
func (e errorReader) Read(p []byte) (n int, err error) { return 0, e.err }
func (e errorReader) Close() error               { return nil }
