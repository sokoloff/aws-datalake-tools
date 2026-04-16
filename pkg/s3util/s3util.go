package s3util

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Object represents an S3 object.
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// ListObjectsAPI defines the S3 API subset needed for listing objects.
type ListObjectsAPI interface {
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// ParseS3URI splits "s3://bucket/key" into bucket and key components.
func ParseS3URI(uri string) (bucket, key string, err error) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("invalid S3 URI %q: must start with s3://", uri)
	}

	rest := strings.TrimPrefix(uri, "s3://")
	if rest == "" {
		return "", "", fmt.Errorf("invalid S3 URI %q: empty bucket name", uri)
	}

	parts := strings.SplitN(rest, "/", 2)
	bucket = parts[0]
	if bucket == "" {
		return "", "", fmt.Errorf("invalid S3 URI %q: empty bucket name", uri)
	}

	if len(parts) == 2 {
		key = parts[1]
	}

	return bucket, key, nil
}

// FormatS3URI constructs an S3 URI from bucket and key.
func FormatS3URI(bucket, key string) string {
	if key == "" {
		return fmt.Sprintf("s3://%s", bucket)
	}
	return fmt.Sprintf("s3://%s/%s", bucket, key)
}

// ListObjects returns all objects under the given bucket and prefix, handling pagination.
func ListObjects(ctx context.Context, client ListObjectsAPI, bucket, prefix string) ([]Object, error) {
	var objects []Object

	input := &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: &prefix,
	}

	for {
		output, err := client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("listing objects in s3://%s/%s: %w", bucket, prefix, err)
		}

		for _, obj := range output.Contents {
			objects = append(objects, Object{
				Key:          *obj.Key,
				Size:         *obj.Size,
				LastModified: *obj.LastModified,
			})
		}

		if !*output.IsTruncated {
			break
		}
		input.ContinuationToken = output.NextContinuationToken
	}

	return objects, nil
}
