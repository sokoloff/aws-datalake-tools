package cli

import (
	"github.com/sokoloff/aws-datalake-tools/pkg/load"
	"github.com/spf13/cobra"
)

var loadFlags struct {
	input           string
	output          string
	database        string
	table           string
	schemaFile      string
	inferOnly       bool
	sampleSize      int
	targetSizeMB    int64
	partition       string
	replaceIfExists bool
	dryRun          bool
}

var loadCmd = &cobra.Command{
	Use:   "load",
	Short: "Load a DynamoDB S3 export into partitioned Parquet",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := load.Config{
			InputURI:        loadFlags.input,
			OutputURI:       loadFlags.output,
			Database:        loadFlags.database,
			Table:           loadFlags.table,
			SchemaFile:      loadFlags.schemaFile,
			InferOnly:       loadFlags.inferOnly,
			SampleSize:      loadFlags.sampleSize,
			TargetSizeMB:    loadFlags.targetSizeMB,
			Partition:       loadFlags.partition,
			ReplaceIfExists: loadFlags.replaceIfExists,
			DryRun:          loadFlags.dryRun,
		}
		return load.Run(cmd.Context(), cfg)
	},
}

func init() {
	loadCmd.Flags().StringVar(&loadFlags.input, "input", "", "Source S3 or local path to DynamoDB export")
	loadCmd.Flags().StringVar(&loadFlags.output, "output", "", "Target S3 URI or local path for Parquet files")
	loadCmd.Flags().StringVar(&loadFlags.database, "database", "", "Glue database name")
	loadCmd.Flags().StringVar(&loadFlags.table, "table", "", "Glue table name")
	loadCmd.Flags().StringVar(&loadFlags.schemaFile, "schema-file", "", "Load schema from JSON file instead of inferring")
	loadCmd.Flags().BoolVar(&loadFlags.inferOnly, "infer-only", false, "Infer schema and print to stdout, then exit")
	loadCmd.Flags().IntVar(&loadFlags.sampleSize, "sample-size", 0, "Number of records to sample for inference (0 = all)")
	loadCmd.Flags().Int64Var(&loadFlags.targetSizeMB, "target-size-mb", 128, "Target Parquet file size in MB")
	loadCmd.Flags().StringVar(&loadFlags.partition, "partition", "auto", "Partition mode: auto, none, or YYYY-MM-DD")
	loadCmd.Flags().BoolVar(&loadFlags.replaceIfExists, "replace-if-exists", false, "Replace existing Glue table")
	loadCmd.Flags().BoolVar(&loadFlags.dryRun, "dry-run", false, "Dry run (no S3/Glue writes)")

	_ = loadCmd.MarkFlagRequired("input")
	_ = loadCmd.MarkFlagRequired("output")
}
