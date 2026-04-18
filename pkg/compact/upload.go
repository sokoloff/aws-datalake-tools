package compact

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// NewS3Uploader creates an UploadFunc for S3.
func NewS3Uploader(api S3API, bucket string) UploadFunc {
	return func(ctx context.Context, key string, body io.Reader, size int64) error {
		_, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(bucket),
			Key:           aws.String(key),
			Body:          body,
			ContentLength: aws.Int64(size),
		})
		return err
	}
}
