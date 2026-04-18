package load

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sokoloff/aws-datalake-tools/pkg/compact"
	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
)

// DumpSource defines the interface for reading a DynamoDB dump.
type DumpSource interface {
	ReadManifestSummary(ctx context.Context) (*ManifestSummary, error)
	ReadManifestFiles(ctx context.Context) ([]ManifestEntry, error)
	OpenDataFile(ctx context.Context, key string) (io.ReadCloser, error)
	Describe() string
}

// NewDumpSource returns an S3 or Local dump source based on the URI.
func NewDumpSource(uri string, s3api compact.S3API) (DumpSource, error) {
	if strings.HasPrefix(uri, "s3://") {
		bucket, prefix, err := s3util.ParseS3URI(uri)
		if err != nil {
			return nil, err
		}
		return &S3DumpSource{S3: s3api, Bucket: bucket, Prefix: prefix}, nil
	}
	return &LocalDumpSource{Root: uri}, nil
}

// LocalDumpSource reads a dump from the local filesystem.
type LocalDumpSource struct {
	Root string
}

func (s *LocalDumpSource) ReadManifestSummary(ctx context.Context) (*ManifestSummary, error) {
	data, err := os.ReadFile(filepath.Join(s.Root, "manifest-summary.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest summary: %w", err)
	}
	var sum ManifestSummary
	if err := json.Unmarshal(data, &sum); err != nil {
		return nil, fmt.Errorf("unmarshaling manifest summary: %w", err)
	}
	return &sum, nil
}

func (s *LocalDumpSource) ReadManifestFiles(ctx context.Context) ([]ManifestEntry, error) {
	f, err := os.Open(filepath.Join(s.Root, "manifest-files.json"))
	if err != nil {
		return nil, fmt.Errorf("opening manifest files: %w", err)
	}
	defer f.Close()

	var entries []ManifestEntry
	scanner := bufio.NewScanner(f)
	const maxTokenSize = 1024 * 1024
	scanner.Buffer(make([]byte, maxTokenSize), maxTokenSize)

	for scanner.Scan() {
		var entry ManifestEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("unmarshaling manifest entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

func (s *LocalDumpSource) OpenDataFile(ctx context.Context, key string) (io.ReadCloser, error) {
	// key is the dataFileS3Key from the manifest, e.g. "AWSDynamoDB/0123/data/abc.json.gz"
	// We assume local layout matches S3 layout relative to root.
	// Actually DynamoDB export layout is usually <root>/data/<name>.json.gz
	// and manifest-files.json is at <root>/manifest-files.json.
	// But dataFileS3Key usually includes the full path from bucket root.
	// We'll just take the base filename and look in <root>/data/
	filename := filepath.Base(key)
	return os.Open(filepath.Join(s.Root, "data", filename))
}

func (s *LocalDumpSource) Describe() string {
	return "local://" + s.Root
}

// S3DumpSource reads a dump from S3.
type S3DumpSource struct {
	S3     compact.S3API
	Bucket string
	Prefix string
}

func (s *S3DumpSource) ReadManifestSummary(ctx context.Context) (*ManifestSummary, error) {
	key := filepath.Join(s.Prefix, "manifest-summary.json")
	resp, err := s.S3.GetObject(ctx, s.GetObjectInput(key))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var sum ManifestSummary
	if err := json.Unmarshal(data, &sum); err != nil {
		return nil, err
	}
	return &sum, nil
}

func (s *S3DumpSource) ReadManifestFiles(ctx context.Context) ([]ManifestEntry, error) {
	key := filepath.Join(s.Prefix, "manifest-files.json")
	resp, err := s.S3.GetObject(ctx, s.GetObjectInput(key))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var entries []ManifestEntry
	scanner := bufio.NewScanner(resp.Body)
	const maxTokenSize = 1024 * 1024
	scanner.Buffer(make([]byte, maxTokenSize), maxTokenSize)

	for scanner.Scan() {
		var entry ManifestEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

func (s *S3DumpSource) OpenDataFile(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.S3.GetObject(ctx, s.GetObjectInput(key))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (s *S3DumpSource) GetObjectInput(key string) *s3.GetObjectInput {
	return &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}
}


func (s *S3DumpSource) Describe() string {
	return fmt.Sprintf("s3://%s/%s", s.Bucket, s.Prefix)
}
