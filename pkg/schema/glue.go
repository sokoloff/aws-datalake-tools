package schema

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
)

// GlueTableAPI defines the subset of Glue API methods used by this package.
type GlueTableAPI interface {
	GetTable(ctx context.Context, params *glue.GetTableInput, optFns ...func(*glue.Options)) (*glue.GetTableOutput, error)
	CreateTable(ctx context.Context, params *glue.CreateTableInput, optFns ...func(*glue.Options)) (*glue.CreateTableOutput, error)
	UpdateTable(ctx context.Context, params *glue.UpdateTableInput, optFns ...func(*glue.Options)) (*glue.UpdateTableOutput, error)
	DeleteTable(ctx context.Context, params *glue.DeleteTableInput, optFns ...func(*glue.Options)) (*glue.DeleteTableOutput, error)
	CreatePartition(ctx context.Context, params *glue.CreatePartitionInput, optFns ...func(*glue.Options)) (*glue.CreatePartitionOutput, error)
}

// FetchTableSchema retrieves the schema for a Glue table.
func FetchTableSchema(ctx context.Context, api GlueTableAPI, database, table string) (*TableSchema, error) {
	resp, err := api.GetTable(ctx, &glue.GetTableInput{
		DatabaseName: aws.String(database),
		Name:         aws.String(table),
	})
	if err != nil {
		return nil, fmt.Errorf("fetching table %s.%s: %w", database, table, err)
	}

	t := resp.Table
	if t == nil {
		return nil, fmt.Errorf("table %s.%s not found", database, table)
	}
	if t.StorageDescriptor == nil {
		return nil, fmt.Errorf("table %s.%s has no storage descriptor", database, table)
	}

	columns, err := columnsFromGlue(t.StorageDescriptor.Columns)
	if err != nil {
		return nil, fmt.Errorf("parsing columns for %s.%s: %w", database, table, err)
	}

	partitionKeys, err := columnsFromGlue(t.PartitionKeys)
	if err != nil {
		return nil, fmt.Errorf("parsing partition keys for %s.%s: %w", database, table, err)
	}

	return &TableSchema{
		Database:      database,
		Table:         table,
		Columns:       columns,
		PartitionKeys: partitionKeys,
		Location:      aws.ToString(t.StorageDescriptor.Location),
	}, nil
}

// CreateTableInput contains parameters for creating a new Glue table.
type CreateTableInput struct {
	Database      string
	Table         string
	Columns       []Column
	PartitionKeys []Column
	Location      string
	Parameters    map[string]string
	Replace       bool
}

// CreateTable creates a new external Parquet table in Glue.
func CreateTable(ctx context.Context, api GlueTableAPI, in CreateTableInput) error {
	if in.Replace {
		_, _ = api.DeleteTable(ctx, &glue.DeleteTableInput{
			DatabaseName: aws.String(in.Database),
			Name:         aws.String(in.Table),
		})
	}

	params := map[string]string{
		"EXTERNAL":       "TRUE",
		"classification": "parquet",
	}
	for k, v := range in.Parameters {
		params[k] = v
	}

	input := &glue.CreateTableInput{
		DatabaseName: aws.String(in.Database),
		TableInput: &gluetypes.TableInput{
			Name:      aws.String(in.Table),
			TableType: aws.String("EXTERNAL_TABLE"),
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Columns:      columnsToGlue(in.Columns),
				Location:     aws.String(in.Location),
				InputFormat:  aws.String("org.apache.hadoop.hive.ql.io.parquet.MapredParquetInputFormat"),
				OutputFormat: aws.String("org.apache.hadoop.hive.ql.io.parquet.MapredParquetOutputFormat"),
				SerdeInfo: &gluetypes.SerDeInfo{
					SerializationLibrary: aws.String("org.apache.hadoop.hive.ql.io.parquet.serde.ParquetHiveSerDe"),
					Parameters: map[string]string{
						"serialization.format": "1",
					},
				},
			},
			PartitionKeys: columnsToGlue(in.PartitionKeys),
			Parameters:    params,
		},
	}


	_, err := api.CreateTable(ctx, input)
	if err != nil {
		return fmt.Errorf("creating table %s.%s: %w", in.Database, in.Table, err)
	}
	return nil
}

// CreatePartitionInput contains parameters for creating a new partition in a Glue table.
type CreatePartitionInput struct {
	Database       string
	Table          string
	PartitionValues []string
	Location       string
}

// CreatePartition creates a new partition in an existing Glue table.
func CreatePartition(ctx context.Context, api GlueTableAPI, in CreatePartitionInput) error {
	input := &glue.CreatePartitionInput{
		DatabaseName: aws.String(in.Database),
		TableName:    aws.String(in.Table),
		PartitionInput: &gluetypes.PartitionInput{
			Values: in.PartitionValues,
			StorageDescriptor: &gluetypes.StorageDescriptor{
				Location: aws.String(in.Location),
				InputFormat:  aws.String("org.apache.hadoop.hive.ql.io.parquet.MapredParquetInputFormat"),
				OutputFormat: aws.String("org.apache.hadoop.hive.ql.io.parquet.MapredParquetOutputFormat"),
				SerdeInfo: &gluetypes.SerDeInfo{
					SerializationLibrary: aws.String("org.apache.hadoop.hive.ql.io.parquet.serde.ParquetHiveSerDe"),
					Parameters: map[string]string{
						"serialization.format": "1",
					},
				},
			},
		},
	}

	_, err := api.CreatePartition(ctx, input)
	if err != nil {
		var alreadyExists *gluetypes.AlreadyExistsException
		if errors.As(err, &alreadyExists) {
			return nil
		}
		return fmt.Errorf("creating partition %v for table %s.%s: %w", in.PartitionValues, in.Database, in.Table, err)
	}
	return nil
}

// UpdateTableSchema updates the columns of an existing Glue table.
func UpdateTableSchema(ctx context.Context, api GlueTableAPI, database, table string, columns []Column) error {
	resp, err := api.GetTable(ctx, &glue.GetTableInput{
		DatabaseName: aws.String(database),
		Name:         aws.String(table),
	})
	if err != nil {
		return fmt.Errorf("fetching table for update %s.%s: %w", database, table, err)
	}

	t := resp.Table
	if t == nil {
		return fmt.Errorf("table %s.%s not found for update", database, table)
	}
	if t.StorageDescriptor == nil {
		return fmt.Errorf("table %s.%s has no storage descriptor", database, table)
	}

	t.StorageDescriptor.Columns = columnsToGlue(columns)

	_, err = api.UpdateTable(ctx, &glue.UpdateTableInput{

		DatabaseName: aws.String(database),
		Name:         aws.String(table),
		TableInput: &gluetypes.TableInput{
			Name:              t.Name,
			Description:       t.Description,
			Owner:             t.Owner,
			LastAccessTime:    t.LastAccessTime,
			LastAnalyzedTime:  t.LastAnalyzedTime,
			Retention:         t.Retention,
			StorageDescriptor: t.StorageDescriptor,
			PartitionKeys:     t.PartitionKeys,
			ViewOriginalText:  t.ViewOriginalText,
			ViewExpandedText:  t.ViewExpandedText,
			TableType:         t.TableType,
			Parameters:        t.Parameters,
			TargetTable:       t.TargetTable,
		},
	})
	if err != nil {
		return fmt.Errorf("updating table %s.%s: %w", database, table, err)
	}
	return nil
}

func columnsFromGlue(glueCols []gluetypes.Column) ([]Column, error) {
	var cols []Column
	for _, gc := range glueCols {
		rawType := aws.ToString(gc.Type)
		dt, err := ParseType(rawType)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", aws.ToString(gc.Name), err)
		}
		cols = append(cols, Column{
			Name:       aws.ToString(gc.Name),
			Type:       dt,
			NativeType: rawType,
			Comment:    aws.ToString(gc.Comment),
		})
	}
	return cols, nil
}

func columnsToGlue(cols []Column) []gluetypes.Column {
	var glueCols []gluetypes.Column
	for _, c := range cols {
		glueCols = append(glueCols, gluetypes.Column{
			Name:    aws.String(c.Name),
			Type:    aws.String(c.Type.GlueType()),
			Comment: aws.String(c.Comment),
		})
	}
	return glueCols
}
