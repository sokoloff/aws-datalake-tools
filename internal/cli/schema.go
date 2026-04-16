package cli

import (
	"fmt"

	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/spf13/cobra"
)

var schemaFlags struct {
	database string
	table    string
}

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Manage Glue Data Catalog schemas",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logging.FromContext(cmd.Context())
		log.Info("describing schema", "database", schemaFlags.database, "table", schemaFlags.table)
		return fmt.Errorf("schema command not yet implemented")
	},
}

func init() {
	schemaCmd.Flags().StringVar(&schemaFlags.database, "database", "", "Glue database name (required)")
	schemaCmd.Flags().StringVar(&schemaFlags.table, "table", "", "Glue table name (required)")
	_ = schemaCmd.MarkFlagRequired("database")
	_ = schemaCmd.MarkFlagRequired("table")
}
