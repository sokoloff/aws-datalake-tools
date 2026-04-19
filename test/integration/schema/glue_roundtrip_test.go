//go:build integration

package schema_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/stretchr/testify/require"

	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
	"github.com/sokoloff/aws-datalake-tools/test/integration/shared"
)

func TestGlue_CreateDescribeUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	clients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	dbName := "testdb"
	tableName := "testtable"

	// Create DB first
	_, err = clients.Glue.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{
			Name: aws.String(dbName),
		},
	})
	require.NoError(t, err)

	// CreateTable
	initialCols := []schema.Column{
		{Name: "id", Type: &schema.PrimitiveType{Kind: schema.Int}},
		{Name: "name", Type: &schema.PrimitiveType{Kind: schema.String}},
	}
	createInput := schema.CreateTableInput{
		Database: dbName,
		Table:    tableName,
		Columns:  initialCols,
		Location: "s3://test-bucket/data/",
	}
	err = schema.CreateTable(ctx, clients.Glue, createInput)
	require.NoError(t, err)

	// FetchTableSchema
	fetchedSchema, err := schema.FetchTableSchema(ctx, clients.Glue, dbName, tableName)
	require.NoError(t, err)
	require.Equal(t, dbName, fetchedSchema.Database)
	require.Equal(t, tableName, fetchedSchema.Table)
	require.Equal(t, "s3://test-bucket/data/", fetchedSchema.Location)
	require.Len(t, fetchedSchema.Columns, 2)
	require.Equal(t, "id", fetchedSchema.Columns[0].Name)
	require.Equal(t, "int", fetchedSchema.Columns[0].Type.GlueType())

	// UpdateTableSchema
	updatedCols := append(initialCols, schema.Column{
		Name: "age", Type: &schema.PrimitiveType{Kind: schema.Int},
	})
	err = schema.UpdateTableSchema(ctx, clients.Glue, dbName, tableName, updatedCols)
	require.NoError(t, err)

	// Re-fetch
	fetchedSchema, err = schema.FetchTableSchema(ctx, clients.Glue, dbName, tableName)
	require.NoError(t, err)
	require.Len(t, fetchedSchema.Columns, 3)
	require.Equal(t, "age", fetchedSchema.Columns[2].Name)
}

func TestGlue_ReplaceIfExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	moto, endpoint, err := shared.StartMoto(ctx, t)
	require.NoError(t, err)
	defer moto.Terminate(ctx)

	clients := shared.NewMotoClients(ctx, t, endpoint)
	defer shared.ResetMoto(t, endpoint)

	dbName := "testdb"
	tableName := "replacetable"

	_, err = clients.Glue.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{
			Name: aws.String(dbName),
		},
	})
	require.NoError(t, err)

	// CreateTable first time
	createInput := schema.CreateTableInput{
		Database: dbName,
		Table:    tableName,
		Columns: []schema.Column{
			{Name: "id", Type: &schema.PrimitiveType{Kind: schema.Int}},
		},
		Location: "s3://test-bucket/old/",
	}
	err = schema.CreateTable(ctx, clients.Glue, createInput)
	require.NoError(t, err)

	// Replace
	createInput.Columns = []schema.Column{
		{Name: "uuid", Type: &schema.PrimitiveType{Kind: schema.String}},
	}
	createInput.Location = "s3://test-bucket/new/"
	createInput.Replace = true
	err = schema.CreateTable(ctx, clients.Glue, createInput)
	require.NoError(t, err)

	// Fetch to verify replace
	fetchedSchema, err := schema.FetchTableSchema(ctx, clients.Glue, dbName, tableName)
	require.NoError(t, err)
	require.Equal(t, "s3://test-bucket/new/", fetchedSchema.Location)
	require.Len(t, fetchedSchema.Columns, 1)
	require.Equal(t, "uuid", fetchedSchema.Columns[0].Name)
}
