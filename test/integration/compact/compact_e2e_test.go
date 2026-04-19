//go:build integration

package compact_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/sokoloff/aws-datalake-tools/pkg/compact"
	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
	"github.com/sokoloff/aws-datalake-tools/test/integration/shared"
)

type TestRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

func setupCompactTest(ctx context.Context, t *testing.T, clients *compact.Deps, bucket, dbName, tableName string) {
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

	createInput := schema.CreateTableInput{
		Database: dbName,
		Table:    tableName,
		Columns: []schema.Column{
			{Name: "id", Type: &schema.PrimitiveType{Kind: schema.BigInt}},
			{Name: "name", Type: &schema.PrimitiveType{Kind: schema.String}},
		},
		Location: fmt.Sprintf("s3://%s/target/", bucket),
	}
	err = schema.CreateTable(ctx, clients.Glue, createInput)
	require.NoError(t, err)
}

func uploadTestFile(ctx context.Context, t *testing.T, s3Client *s3.Client, bucket, key string, rows []TestRow) {
	t.Helper()

	tmpFile := filepath.Join(t.TempDir(), "test.parquet")
	shared.WriteTestParquet(t, tmpFile, rows)

	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer f.Close()

	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   f,
	})
	require.NoError(t, err)
}

func TestCompact_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	awsClients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	deps := compact.Deps{
		S3:   awsClients.S3,
		Glue: awsClients.Glue,
	}

	bucket := "compact-test-bucket"
	dbName := "testdb"
	tableName := "testtable"

	setupCompactTest(ctx, t, &deps, bucket, dbName, tableName)

	// Create and upload 3 files, 1000 rows each
	for i := 0; i < 3; i++ {
		var rows []TestRow
		for j := 0; j < 1000; j++ {
			rows = append(rows, TestRow{ID: int64(i*1000 + j), Name: fmt.Sprintf("name-%d", i*1000+j)})
		}
		uploadTestFile(ctx, t, awsClients.S3, bucket, fmt.Sprintf("source/part-%d.parquet", i), rows)
	}

	cfg := compact.Config{
		SourceURI:    fmt.Sprintf("s3://%s/source/", bucket),
		TargetURI:    fmt.Sprintf("s3://%s/target/", bucket),
		Database:     dbName,
		Table:        tableName,
		TargetSizeMB: 1, // small to ensure it writes
		DeleteSource: false,
	}

	report, err := compact.RunWithDeps(ctx, cfg, deps)
	require.NoError(t, err)

	require.Equal(t, int64(3000), report.RowsRead)
	require.Equal(t, int64(3000), report.RowsWritten)
	require.NotEmpty(t, report.OutputFiles)

	// Verify sources are still there
	srcObjects, err := s3util.ListObjects(ctx, awsClients.S3, bucket, "source/")
	require.NoError(t, err)
	require.Len(t, srcObjects, 3)

	// Verify targets
	tgtObjects, err := s3util.ListObjects(ctx, awsClients.S3, bucket, "target/")
	require.NoError(t, err)
	require.NotEmpty(t, tgtObjects)
}

func TestCompact_E2E_DeleteSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	awsClients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	deps := compact.Deps{
		S3:   awsClients.S3,
		Glue: awsClients.Glue,
	}

	bucket := "compact-test-bucket-del"
	dbName := "testdb"
	tableName := "testtable"

	setupCompactTest(ctx, t, &deps, bucket, dbName, tableName)

	var rows []TestRow
	for j := 0; j < 10; j++ {
		rows = append(rows, TestRow{ID: int64(j), Name: "name"})
	}
	uploadTestFile(ctx, t, awsClients.S3, bucket, "source/part-0.parquet", rows)

	cfg := compact.Config{
		SourceURI:    fmt.Sprintf("s3://%s/source/", bucket),
		TargetURI:    fmt.Sprintf("s3://%s/target/", bucket),
		Database:     dbName,
		Table:        tableName,
		TargetSizeMB: 1,
		DeleteSource: true,
	}

	report, err := compact.RunWithDeps(ctx, cfg, deps)
	require.NoError(t, err)
	require.Len(t, report.DeletedSources, 1)

	// Verify sources are gone
	srcObjects, err := s3util.ListObjects(ctx, awsClients.S3, bucket, "source/")
	require.NoError(t, err)
	require.Empty(t, srcObjects)
}

func TestCompact_E2E_IncompatibleSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	awsClients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	deps := compact.Deps{
		S3:   awsClients.S3,
		Glue: awsClients.Glue,
	}

	bucket := "compact-test-bucket-inc"
	dbName := "testdb"
	tableName := "testtable"

	setupCompactTest(ctx, t, &deps, bucket, dbName, tableName)

	// Upload incompatible schema
	type BadRow struct {
		ID   string `parquet:"id"` // string instead of int64
		Name string `parquet:"name"`
	}
	var rows []BadRow
	rows = append(rows, BadRow{ID: "bad", Name: "name"})

	tmpFile := filepath.Join(t.TempDir(), "bad.parquet")
	shared.WriteTestParquet(t, tmpFile, rows)
	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer f.Close()
	_, err = awsClients.S3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("source/part-0.parquet"),
		Body:   f,
	})
	require.NoError(t, err)

	cfg := compact.Config{
		SourceURI:    fmt.Sprintf("s3://%s/source/", bucket),
		TargetURI:    fmt.Sprintf("s3://%s/target/", bucket),
		Database:     dbName,
		Table:        tableName,
	}

	_, err = compact.RunWithDeps(ctx, cfg, deps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "building plan")

	// Target should be empty
	tgtObjects, err := s3util.ListObjects(ctx, awsClients.S3, bucket, "target/")
	require.NoError(t, err)
	require.Empty(t, tgtObjects)
}
