package load

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/sokoloff/aws-datalake-tools/internal/awsclient"
	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/sokoloff/aws-datalake-tools/pkg/compact"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

type Config struct {
	InputURI              string
	OutputURI             string
	Database              string
	Table                 string
	SchemaFile            string
	InferOnly             bool
	SampleSize            int
	TargetSizeMB          int64
	Partition             string // auto, none, or YYYY-MM-DD
	ReplaceIfExists       bool
	DryRun                bool
	InjectMetadataColumns bool
}

type Deps struct {
	S3   compact.S3API
	Glue schema.GlueTableAPI
	Log  *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	clients, err := awsclient.New(ctx, awsclient.Config{})
	if err != nil {
		return err
	}
	deps := Deps{
		S3:   clients.S3,
		Glue: clients.Glue,
		Log:  logging.FromContext(ctx),
	}
	report, err := RunWithDeps(ctx, cfg, deps)
	if err != nil {
		return err
	}
	return FormatReport(os.Stdout, report)
}

func RunWithDeps(ctx context.Context, cfg Config, deps Deps) (*Report, error) {
	start := time.Now()

	source, err := NewDumpSource(cfg.InputURI, deps.S3)
	if err != nil {
		return nil, err
	}

	summary, err := source.ReadManifestSummary(ctx)
	if err != nil {
		return nil, err
	}

	files, err := source.ReadManifestFiles(ctx)
	if err != nil {
		return nil, err
	}

	partition, err := ResolvePartitionSpec(summary.ExportTime, cfg.Partition)
	if err != nil {
		return nil, err
	}

	spoolPath, recordsRead, inferrer, err := runInferencePass(ctx, source, files, cfg, deps.Log)
	if spoolPath != "" {
		defer os.Remove(spoolPath)
	}
	if err != nil {
		return nil, err
	}

	finalCols, err := resolveFinalSchema(cfg, inferrer)
	if err != nil {
		return nil, err
	}

	if cfg.InferOnly {
		return emitInferOnlyReport(os.Stdout, finalCols, recordsRead)
	}

	outputs, outBytes, err := runWritePass(ctx, spoolPath, finalCols, partition, cfg, deps)
	if err != nil {
		return nil, err
	}

	if cfg.Database != "" && cfg.Table != "" && !cfg.DryRun {
		if err := registerGlueTable(ctx, cfg, finalCols, partition, deps); err != nil {
			return nil, err
		}
	}

	return &Report{
		RecordsRead:   recordsRead,
		OutputFiles:   outputs,
		OutputBytes:   outBytes,
		Schema:        finalCols,
		PartitionKeys: partition.PartitionKeys(),
		Duration:      time.Since(start),
	}, nil
}
