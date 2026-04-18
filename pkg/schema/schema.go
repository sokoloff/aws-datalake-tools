package schema

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
)

// DescribeOutput contains the result of describing a Glue table.
type DescribeOutput struct {
	Schema *TableSchema
}

// Describe fetches the schema for a Glue table.
func Describe(ctx context.Context, api GlueTableAPI, database, table string) (*DescribeOutput, error) {
	schema, err := FetchTableSchema(ctx, api, database, table)
	if err != nil {
		return nil, err
	}
	return &DescribeOutput{Schema: schema}, nil
}

// DescribeFile fetches the schema from a Parquet file in S3.
func DescribeFile(ctx context.Context, api S3GetObjectAPI, bucket, key string) (*DescribeOutput, error) {
	cols, err := ReadParquetSchemaFromS3(ctx, api, bucket, key)
	if err != nil {
		return nil, err
	}
	return &DescribeOutput{
		Schema: &TableSchema{
			Table:    key,
			Columns:  cols,
			Location: s3util.FormatS3URI(bucket, key),
		},
	}, nil
}

// DiffOutput contains the result of comparing a Glue table schema with a Parquet file.
type DiffOutput struct {
	GlueSchema *TableSchema
	FileSchema []Column
	Plan       *CoercionPlan
}

// Diff compares a Glue table schema with a Parquet file schema in S3.
func Diff(ctx context.Context, glueAPI GlueTableAPI, s3API S3GetObjectAPI, database, table, bucket, key string) (*DiffOutput, error) {
	glueSchema, err := FetchTableSchema(ctx, glueAPI, database, table)
	if err != nil {
		return nil, fmt.Errorf("fetching glue schema: %w", err)
	}

	fileSchema, err := ReadParquetSchemaFromS3(ctx, s3API, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("reading file schema: %w", err)
	}

	plan := CompareSchemas(glueSchema.Columns, fileSchema)

	return &DiffOutput{
		GlueSchema: glueSchema,
		FileSchema: fileSchema,
		Plan:       plan,
	}, nil
}

// FormatSchemaPretty prints a TableSchema with complex types expanded across multiple lines.
func FormatSchemaPretty(w io.Writer, s *TableSchema, native bool) error {
	fmt.Fprintf(w, "Database: %s\n", s.Database)
	fmt.Fprintf(w, "Table:    %s\n", s.Table)
	fmt.Fprintf(w, "Location: %s\n\n", s.Location)

	fmt.Fprintln(w, "Columns:")
	for _, c := range s.Columns {
		typeStr := c.Type.Pretty(0, native)
		if native && c.NativeType != "" {
			typeStr = fmt.Sprintf("%s [%s]", typeStr, c.NativeType)
		}
		if c.Comment != "" {
			fmt.Fprintf(w, "%s: %s # %s\n", c.Name, typeStr, c.Comment)
		} else {
			fmt.Fprintf(w, "%s: %s\n", c.Name, typeStr)
		}
	}

	if len(s.PartitionKeys) > 0 {
		fmt.Fprintln(w, "\nPartition Keys:")
		for _, c := range s.PartitionKeys {
			typeStr := c.Type.Pretty(0, native)
			if native && c.NativeType != "" {
				typeStr = fmt.Sprintf("%s [%s]", typeStr, c.NativeType)
			}
			if c.Comment != "" {
				fmt.Fprintf(w, "%s: %s # %s\n", c.Name, typeStr, c.Comment)
			} else {
				fmt.Fprintf(w, "%s: %s\n", c.Name, typeStr)
			}
		}
	}
	return nil
}

func FormatSchema(w io.Writer, s *TableSchema, native bool) error {
	fmt.Fprintf(w, "Database: %s\n", s.Database)
	fmt.Fprintf(w, "Table:    %s\n", s.Table)
	fmt.Fprintf(w, "Location: %s\n\n", s.Location)

	fmt.Fprintln(w, "Columns:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := "NAME\tTYPE\tCOMMENT"
	if native {
		header = "NAME\tTYPE\tNATIVE TYPE\tCOMMENT"
	}
	fmt.Fprintln(tw, header)
	for _, c := range s.Columns {
		gt := c.Type.GlueType()
		displayType := gt
		if len(displayType) > 100 {
			displayType = displayType[:97] + "..."
		}
		if native {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Name, displayType, c.NativeType, c.Comment)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, displayType, c.Comment)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if len(s.PartitionKeys) > 0 {
		fmt.Fprintln(w, "\nPartition Keys:")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, header)
		for _, c := range s.PartitionKeys {
			gt := c.Type.GlueType()
			displayType := gt
			if len(displayType) > 100 {
				displayType = displayType[:97] + "..."
			}
			if native {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Name, displayType, c.NativeType, c.Comment)
			} else {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, displayType, c.Comment)
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// Color constants
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
)

// FormatDiff prints a DiffOutput in a human-readable format.
func FormatDiff(w io.Writer, d *DiffOutput) error {
	if d.Plan.Compatible {
		fmt.Fprintf(w, "%sSchemas are compatible.%s\n", ColorGreen, ColorReset)
	} else {
		fmt.Fprintf(w, "%sSchemas are INCOMPATIBLE!%s\n", ColorRed, ColorReset)
	}

	if len(d.Plan.Diffs) == 0 {
		fmt.Fprintln(w, "No differences found.")
		return nil
	}

	fmt.Fprintln(w, "\nDifferences:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tPATH\tGLUE TYPE\tFILE TYPE")
	for _, diff := range d.Plan.Diffs {
		glueType := "-"
		if diff.GlueType != nil {
			glueType = diff.GlueType.GlueType()
			if len(glueType) > 50 {
				glueType = glueType[:47] + "..."
			}
		}
		fileType := "-"
		if diff.FileType != nil {
			fileType = diff.FileType.GlueType()
			if len(fileType) > 50 {
				fileType = fileType[:47] + "..."
			}
		}

		color := ""
		switch diff.Kind {
		case DiffTypeMismatch:
			color = ColorRed
		case DiffMissingInFile, DiffExtraInFile:
			color = ColorYellow
		}

		fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s%s\n", color, diff.Kind, diff.FullPath(), glueType, fileType, ColorReset)
	}
	return tw.Flush()
}

// S3ClientAPI is a helper interface that combines S3 and Glue APIs for convenience.
// But we'll use separate ones in the functions.
type S3ClientAPI interface {
	S3GetObjectAPI
}
type GlueClientAPI interface {
	GlueTableAPI
}
type DescribeConfig struct {
	Database string
	Table    string
}

type DiffConfig struct {
	Database string
	Table    string
	S3Path   string // s3://bucket/key
}
