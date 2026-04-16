package cli

import (
	"fmt"

	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/spf13/cobra"
)

var compactFlags struct {
	source   string
	target   string
	database string
	table    string
}

var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact small parquet files into larger ones",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logging.FromContext(cmd.Context())
		log.Info("starting compaction", "source", compactFlags.source, "target", compactFlags.target)
		return fmt.Errorf("compact command not yet implemented")
	},
}

func init() {
	compactCmd.Flags().StringVar(&compactFlags.source, "source", "", "Source S3 URI (required)")
	compactCmd.Flags().StringVar(&compactFlags.target, "target", "", "Target S3 URI")
	compactCmd.Flags().StringVar(&compactFlags.database, "database", "", "Glue database name")
	compactCmd.Flags().StringVar(&compactFlags.table, "table", "", "Glue table name")
	_ = compactCmd.MarkFlagRequired("source")
}
