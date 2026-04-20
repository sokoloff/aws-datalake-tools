package compact

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sokoloff/aws-datalake-tools/internal/awsclient"
	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

type Config struct {
	SourceURI    string
	TargetURI    string
	Database     string
	Table        string
	TargetSizeMB int64
	DeleteSource bool
	DryRun       bool
	MaxFiles     int
}

type Deps struct {
	S3   S3API
	Glue schema.GlueTableAPI
	Log  *slog.Logger
}

type S3API interface {
	schema.S3GetObjectAPI
	s3util.ListObjectsAPI
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
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
	plan, err := BuildPlan(ctx, deps, cfg)
	if err != nil {
		if plan != nil && len(plan.Incompatible) > 0 {
			plan.Describe(os.Stdout)
		}
		return nil, fmt.Errorf("building plan: %w", err)
	}

	if cfg.DryRun {
		plan.Describe(os.Stdout)
		return &Report{DryRun: true}, nil
	}

	report := newReportFromPlan(plan)
	defer func() { report.Duration = time.Since(start) }()

	writer, err := newWriterForPlan(ctx, deps, plan, cfg)
	if err != nil {
		return nil, fmt.Errorf("initializing writer: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			writer.Close()
		}
	}()

	if err := writeAllDownloads(ctx, deps, plan, writer, report); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing writer: %w", err)
	}
	closed = true

	report.OutputFiles = writer.Outputs()
	report.RowsWritten = writer.RowsWritten()
	report.OutputBytes = writer.TotalBytesWritten()

	if cfg.DeleteSource {
		keys := make([]string, len(plan.Files))
		for i, f := range plan.Files {
			keys[i] = f.Key
		}
		report.DeletedSources = deleteSources(ctx, deps, plan.SourceBucket, keys)
	}

	return report, nil
}

func newReportFromPlan(plan *CompactionPlan) *Report {
	r := &Report{SourceFiles: make([]string, 0, len(plan.Files))}
	for _, f := range plan.Files {
		r.SourceFiles = append(r.SourceFiles, f.Key)
		r.SourceBytes += f.Size
	}
	return r
}

func newWriterForPlan(ctx context.Context, deps Deps, plan *CompactionPlan, cfg Config) (*RollingWriter, error) {
	rollBytes := cfg.TargetSizeMB * 1024 * 1024
	if rollBytes <= 0 {
		rollBytes = 128 * 1024 * 1024
	}
	uploadFn := NewS3Uploader(deps.S3, plan.TargetBucket)
	return NewRollingWriter(ctx, plan.TargetSchema, rollBytes, plan.TargetBucket, plan.TargetPrefix, uploadFn)
}

// writeAllDownloads streams downloaded files in source order and writes each
// through the schema conversion into writer. Temp files are cleaned up per
// iteration (not via defer) so resources don't accumulate across large plans.
func writeAllDownloads(ctx context.Context, deps Deps, plan *CompactionPlan, writer *RollingWriter, report *Report) error {
	results := streamDownloads(ctx, deps.S3, plan.SourceBucket, plan.Files, deps.Log)
	total := len(plan.Files)

	for res := range results {
		if err := writeOne(deps.Log, plan, writer, report, &res, total); err != nil {
			res.Cleanup()
			// Drain remaining results so downloader goroutines can exit cleanly.
			for r := range results {
				r.Cleanup()
			}
			return err
		}
		res.Cleanup()
	}
	return nil
}

func writeOne(log *slog.Logger, plan *CompactionPlan, writer *RollingWriter, report *Report, res *downloadResult, total int) error {
	if res.Err != nil {
		return fmt.Errorf("downloading %s: %w", res.Key, res.Err)
	}

	if log != nil {
		log.Info("processing file",
			"index", res.Index+1,
			"total", total,
			"key", res.Key,
		)
	}

	conv, err := BuildConversion(plan.TargetSchema, res.File.Schema())
	if err != nil {
		return fmt.Errorf("building conversion for %s: %w", res.Key, err)
	}

	for _, rg := range res.File.RowGroups() {
		report.RowsRead += rg.NumRows()
		if err := writer.WriteConvertedRowGroup(rg, conv); err != nil {
			return fmt.Errorf("writing row group from %s: %w", res.Key, err)
		}
	}
	return nil
}
