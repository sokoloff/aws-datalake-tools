package compact

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// deleteSources removes the given keys from bucket. Individual failures are
// logged (if Deps.Log is set) but do not abort the batch. Returns the keys
// that were successfully deleted.
func deleteSources(ctx context.Context, deps Deps, bucket string, keys []string) []string {
	deleted := make([]string, 0, len(keys))
	for _, key := range keys {
		_, err := deps.S3.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			if deps.Log != nil {
				deps.Log.Warn("failed to delete source file", "key", key, "error", err)
			}
			continue
		}
		deleted = append(deleted, key)
	}
	return deleted
}
