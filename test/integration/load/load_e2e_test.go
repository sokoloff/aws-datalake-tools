//go:build integration

package load_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/sokoloff/aws-datalake-tools/pkg/load"
	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
	"github.com/sokoloff/aws-datalake-tools/test/integration/shared"
)

func setupLoadTest(ctx context.Context, t *testing.T, clients *load.Deps, bucket, dbName string) {
	t.Helper()

	_, err := clients.S3.(*s3.Client).CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	require.NoError(t, err)

	_, err = clients.Glue.(*glue.Client).CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{
			Name: aws.String(dbName),
		},
	})
	require.NoError(t, err)
}

func TestLoad_E2E_LocalDump_S3Output(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	awsClients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	deps := load.Deps{
		S3:   awsClients.S3,
		Glue: awsClients.Glue,
	}

	bucket := "load-test-bucket-1"
	dbName := "testdb"
	tableName := "testtable"
	setupLoadTest(ctx, t, &deps, bucket, dbName)

	testdataPath := "../../../pkg/load/testdata/"

	cfg := load.Config{
		InputURI:        testdataPath,
		OutputURI:       fmt.Sprintf("s3://%s/out/", bucket),
		Database:        dbName,
		Table:           tableName,
		Partition:       "auto",
		ReplaceIfExists: true,
	}

	report, err := load.RunWithDeps(ctx, cfg, deps)
	require.NoError(t, err)

	// In testdata, manifest-summary.json says itemCount is 3
	require.Equal(t, int64(3), report.RecordsRead)

	// Assert partitioned parquet output
	objects, err := s3util.ListObjects(ctx, awsClients.S3, bucket, "out/")
	require.NoError(t, err)
	require.NotEmpty(t, objects)

	// Check if partitioned
	var foundParquet bool
	for _, obj := range objects {
		if strings.Contains(obj.Key, "year=") && strings.HasSuffix(obj.Key, ".parquet") {
			foundParquet = true
			break
		}
	}
	require.True(t, foundParquet, "should have partitioned parquet files")

	// Verify Glue table created
	tbl, err := schema.FetchTableSchema(ctx, awsClients.Glue, dbName, tableName)
	require.NoError(t, err)
	require.NotEmpty(t, tbl.Columns)
	require.Equal(t, fmt.Sprintf("s3://%s/out/", bucket), tbl.Location)
}

func TestLoad_E2E_S3Dump_S3Output(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	awsClients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	deps := load.Deps{
		S3:   awsClients.S3,
		Glue: awsClients.Glue,
	}

	bucket := "load-test-bucket-2"
	dbName := "testdb"
	tableName := "testtable2"
	setupLoadTest(ctx, t, &deps, bucket, dbName)

	// Upload testdata to S3
	testdataPath := "../../../pkg/load/testdata/"
	err = filepath.Walk(testdataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(testdataPath, path)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		s3Key := "dump/" + rel
		if strings.HasPrefix(rel, "data/") {
			s3Key = "AWSDynamoDB/0123/" + rel
		}
		_, err = awsClients.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(s3Key),
			Body:   f,
		})
		return err
	})
	require.NoError(t, err)

	cfg := load.Config{
		InputURI:        fmt.Sprintf("s3://%s/dump/", bucket),
		OutputURI:       fmt.Sprintf("s3://%s/out/", bucket),
		Database:        dbName,
		Table:           tableName,
		Partition:       "auto",
		ReplaceIfExists: true,
	}

	report, err := load.RunWithDeps(ctx, cfg, deps)
	require.NoError(t, err)

	require.Equal(t, int64(3), report.RecordsRead)

	objects, err := s3util.ListObjects(ctx, awsClients.S3, bucket, "out/")
	require.NoError(t, err)
	require.NotEmpty(t, objects)

	tbl, err := schema.FetchTableSchema(ctx, awsClients.Glue, dbName, tableName)
	require.NoError(t, err)
	require.NotEmpty(t, tbl.Columns)
}

func TestLoad_E2E_InferOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	awsClients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	deps := load.Deps{
		S3:   awsClients.S3,
		Glue: awsClients.Glue,
	}

	testdataPath := "../../../pkg/load/testdata/"

	cfg := load.Config{
		InputURI:  testdataPath,
		InferOnly: true,
		Partition: "none",
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	report, err := load.RunWithDeps(ctx, cfg, deps)

	w.Close()
	os.Stdout = oldStdout

	require.NoError(t, err)
	require.Equal(t, int64(3), report.RecordsRead)

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	require.Contains(t, output, `"Name"`)
	require.Contains(t, output, `"Type"`)

	// No Glue table should be created since we didn't specify DB/Table and it's infer only
}
