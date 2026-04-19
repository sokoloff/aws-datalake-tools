//go:build integration

package s3util_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
	"github.com/sokoloff/aws-datalake-tools/test/integration/shared"
)

func TestListObjects_Pagination(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	clients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	bucket := "pagination-test-bucket"

	_, err = clients.S3.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	require.NoError(t, err)

	// Write 2500 objects
	numObjects := 2500
	var wg sync.WaitGroup
	errCh := make(chan error, numObjects)

	// To speed up, we use 50 workers
	sem := make(chan struct{}, 50)

	for i := 0; i < numObjects; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			key := fmt.Sprintf("prefix/obj-%04d", i)
			_, err := clients.S3.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
				Body:   bytes.NewReader([]byte("test")),
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	if err, hasErr := <-errCh; hasErr {
		t.Fatalf("failed to upload object: %v", err)
	}

	// Now list objects
	objects, err := s3util.ListObjects(ctx, clients.S3, bucket, "prefix/")
	require.NoError(t, err)
	require.Len(t, objects, numObjects)
}
