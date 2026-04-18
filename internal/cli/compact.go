package cli

import (
	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/sokoloff/aws-datalake-tools/pkg/compact"
	"github.com/spf13/cobra"
)

var compactFlags struct {
	source       string
	target       string
	database     string
	table        string
	targetSizeMB int64
	deleteSource bool
	dryRun       bool
	maxFiles     int
}

var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact small parquet files into larger ones",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logging.FromContext(cmd.Context())
		log.Info("starting compaction", "source", compactFlags.source, "target", compactFlags.target)

		cfg := compact.Config{
			SourceURI:    compactFlags.source,
			TargetURI:    compactFlags.target,
			Database:     compactFlags.database,
			Table:        compactFlags.table,
			TargetSizeMB: compactFlags.targetSizeMB,
			DeleteSource: compactFlags.deleteSource,
			DryRun:       compactFlags.dryRun,
			MaxFiles:     compactFlags.maxFiles,
		}

		return compact.Run(cmd.Context(), cfg)
	},
}

func init() {
	compactCmd.Flags().StringVar(&compactFlags.source, "source", "", "Source S3 URI (required)")
	compactCmd.Flags().StringVar(&compactFlags.target, "target", "", "Target S3 URI (required)")
	compactCmd.Flags().StringVar(&compactFlags.database, "database", "", "Glue database name")
	compactCmd.Flags().StringVar(&compactFlags.table, "table", "", "Glue table name")
	compactCmd.Flags().Int64Var(&compactFlags.targetSizeMB, "target-size-mb", 128, "Target file size in MB")
	compactCmd.Flags().BoolVar(&compactFlags.deleteSource, "delete-source", false, "Delete source files after compaction")
	compactCmd.Flags().BoolVar(&compactFlags.dryRun, "dry-run", false, "Plan compaction without executing")
	compactCmd.Flags().IntVar(&compactFlags.maxFiles, "max-files", 0, "Maximum number of files to compact (0 = no limit)")

	_ = compactCmd.MarkFlagRequired("source")
	_ = compactCmd.MarkFlagRequired("target")
}
