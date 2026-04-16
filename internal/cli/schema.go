package cli

import (
	"fmt"
	"os"

	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Manage and inspect Glue schemas",
}

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Describe a Glue table schema or a Parquet file",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, _ := cmd.Flags().GetString("database")
		table, _ := cmd.Flags().GetString("table")
		file, _ := cmd.Flags().GetString("file")
		pretty, _ := cmd.Flags().GetBool("pretty")
		native, _ := cmd.Flags().GetBool("native")

		if file != "" {
			bucket, key, err := schema.ParseS3Path(file)
			if err != nil {
				return err
			}

			clients, err := newClients(cmd.Context())
			if err != nil {
				return err
			}

			out, err := schema.DescribeFile(cmd.Context(), clients.S3, bucket, key)
			if err != nil {
				return err
			}

			if pretty {
				return schema.FormatSchemaPretty(os.Stdout, out.Schema, native)
			}
			return schema.FormatSchema(os.Stdout, out.Schema, native)
		}

		if db == "" || table == "" {
			return fmt.Errorf("database and table are required (or use --file for direct file description)")
		}

		clients, err := newClients(cmd.Context())
		if err != nil {
			return err
		}

		out, err := schema.Describe(cmd.Context(), clients.Glue, db, table)
		if err != nil {
			return err
		}

		if pretty {
			return schema.FormatSchemaPretty(os.Stdout, out.Schema, native)
		}
		return schema.FormatSchema(os.Stdout, out.Schema, native)
	},
}

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare Glue schema with a Parquet file",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, _ := cmd.Flags().GetString("database")
		table, _ := cmd.Flags().GetString("table")
		filePath, _ := cmd.Flags().GetString("file")

		if db == "" || table == "" || filePath == "" {
			return fmt.Errorf("database, table, and file are required")
		}

		bucket, key, err := schema.ParseS3Path(filePath)
		if err != nil {
			return err
		}

		clients, err := newClients(cmd.Context())
		if err != nil {
			return err
		}

		out, err := schema.Diff(cmd.Context(), clients.Glue, clients.S3, db, table, bucket, key)
		if err != nil {
			return err
		}

		return schema.FormatDiff(os.Stdout, out)
	},
}

func init() {
	schemaCmd.AddCommand(describeCmd)
	schemaCmd.AddCommand(diffCmd)

	schemaCmd.PersistentFlags().StringP("database", "d", "", "Glue database name")
	schemaCmd.PersistentFlags().StringP("table", "t", "", "Glue table name")

	describeCmd.Flags().Bool("pretty", false, "Pretty-print complex types")
	describeCmd.Flags().Bool("native", false, "Show native types (Glue or Parquet)")
	describeCmd.Flags().StringP("file", "f", "", "S3 path to parquet file to describe")

	diffCmd.Flags().StringP("file", "f", "", "S3 path to parquet file (s3://bucket/key)")
}
