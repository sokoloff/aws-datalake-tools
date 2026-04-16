package cli

import (
	"fmt"

	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/spf13/cobra"
)

var loadFlags struct {
	input           string
	output          string
	database        string
	table           string
	replaceIfExists bool
}

var loadCmd = &cobra.Command{
	Use:   "load",
	Short: "Load DynamoDB dump data into parquet format",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logging.FromContext(cmd.Context())
		log.Info("starting load", "input", loadFlags.input, "output", loadFlags.output)
		return fmt.Errorf("load command not yet implemented")
	},
}

func init() {
	loadCmd.Flags().StringVar(&loadFlags.input, "input", "", "Input S3 URI of DynamoDB dump (required)")
	loadCmd.Flags().StringVar(&loadFlags.output, "output", "", "Output S3 URI for parquet files (required)")
	loadCmd.Flags().StringVar(&loadFlags.database, "database", "", "Glue database name")
	loadCmd.Flags().StringVar(&loadFlags.table, "table", "", "Glue table name")
	loadCmd.Flags().BoolVar(&loadFlags.replaceIfExists, "replace-if-exists", false, "Replace Glue table if it already exists")
	_ = loadCmd.MarkFlagRequired("input")
	_ = loadCmd.MarkFlagRequired("output")
}
