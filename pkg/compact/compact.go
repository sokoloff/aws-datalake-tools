package compact

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
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

type Report struct {
	SourceFiles    []string
	OutputFiles    []string
	DeletedSources []string
	RowsRead       int64
	RowsWritten    int64
	BytesRead      int64
	BytesWritten   int64
	DryRun         bool
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

	report := &Report{
		SourceFiles: make([]string, 0, len(plan.Files)),
	}
	for _, f := range plan.Files {
		report.SourceFiles = append(report.SourceFiles, f.Key)
	}

	uploadFn := NewS3Uploader(deps.S3, plan.TargetBucket)
	rollBytes := cfg.TargetSizeMB * 1024 * 1024
	if rollBytes <= 0 {
		rollBytes = 128 * 1024 * 1024
	}

	writer, err := NewRollingWriter(ctx, plan.TargetSchema, rollBytes, plan.TargetBucket, plan.TargetPrefix, uploadFn)
	if err != nil {
		return nil, fmt.Errorf("initializing writer: %w", err)
	}
	defer writer.Close()

	// Process each file
	for i, f := range plan.Files {
		if deps.Log != nil {
			deps.Log.Info("processing file",
				"index", i+1,
				"total", len(plan.Files),
				"key", f.Key,
			)
		}
		file, err := schema.OpenParquetFileS3(ctx, deps.S3, plan.SourceBucket, f.Key)
		if err != nil {
			return nil, fmt.Errorf("opening source file %s: %w", f.Key, err)
		}

		conv, err := BuildConversion(plan.TargetSchema, file.Schema())
		if err != nil {
			return nil, fmt.Errorf("building conversion for %s: %w", f.Key, err)
		}

		for _, rg := range file.RowGroups() {
			report.RowsRead += rg.NumRows()
			if err := writer.WriteConvertedRowGroup(rg, conv); err != nil {
				return nil, fmt.Errorf("writing row group from %s: %w", f.Key, err)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing writer: %w", err)
	}

	report.OutputFiles = writer.Outputs()
	report.RowsWritten = writer.RowsWritten()

	// Delete sources if requested and everything succeeded
	if cfg.DeleteSource {
		for _, f := range plan.Files {
			_, err := deps.S3.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(plan.SourceBucket),
				Key:    aws.String(f.Key),
			})
			if err != nil {
				if deps.Log != nil {
					deps.Log.Warn("failed to delete source file", "key", f.Key, "error", err)
				}
			} else {
				report.DeletedSources = append(report.DeletedSources, f.Key)
			}
		}
	}

	return report, nil
}

func FormatReport(w io.Writer, r *Report) error {
	if r.DryRun {
		fmt.Fprintln(w, "Dry run complete. No files were modified.")
		return nil
	}

	fmt.Fprintln(w, "Compaction Report")
	fmt.Fprintln(w, "=================")
	fmt.Fprintf(w, "Source files processed: %d\n", len(r.SourceFiles))
	fmt.Fprintf(w, "Output files created:   %d\n", len(r.OutputFiles))
	fmt.Fprintf(w, "Rows read:              %d\n", r.RowsRead)
	fmt.Fprintf(w, "Rows written:           %d\n", r.RowsWritten)

	if len(r.DeletedSources) > 0 {
		fmt.Fprintf(w, "Source files deleted:   %d\n", len(r.DeletedSources))
	}
	return nil
}
