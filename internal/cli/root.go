package cli

import (
	"context"
	"log/slog"

	"github.com/sokoloff/aws-datalake-tools/internal/awsclient"
	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/spf13/cobra"
)

var globals struct {
	region  string
	profile string
	verbose bool
}

var rootCmd = &cobra.Command{
	Use:   "datalake",
	Short: "AWS Data Lake management tools",
	Long:  "Tools for compacting parquet files, loading DynamoDB data, and managing Glue schemas.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		level := slog.LevelInfo
		if globals.verbose {
			level = slog.LevelDebug
		}
		logger := logging.New(logging.WithLevel(level))
		cmd.SetContext(logging.WithContext(cmd.Context(), logger))
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&globals.region, "region", "", "AWS region")
	rootCmd.PersistentFlags().StringVar(&globals.profile, "profile", "", "AWS profile")
	rootCmd.PersistentFlags().BoolVarP(&globals.verbose, "verbose", "v", false, "Enable debug logging")

	rootCmd.AddCommand(compactCmd)
	rootCmd.AddCommand(loadCmd)
	rootCmd.AddCommand(schemaCmd)
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func newClients(ctx context.Context) (*awsclient.Clients, error) {
	return awsclient.New(ctx, awsclient.Config{
		Region:  globals.region,
		Profile: globals.profile,
	})
}
