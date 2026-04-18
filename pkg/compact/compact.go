package compact

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/parquet-go/parquet-go"
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
	SourceBytes    int64
	OutputBytes    int64
	Duration       time.Duration
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

	report := &Report{
		SourceFiles: make([]string, 0, len(plan.Files)),
	}
	defer func() {
		report.Duration = time.Since(start)
	}()
	for _, f := range plan.Files {
		report.SourceFiles = append(report.SourceFiles, f.Key)
		report.SourceBytes += f.Size
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

	// Pipeline:
	// 1. Worker pool downloads files in parallel to local temp storage
	// 2. Results are sent to a channel
	// 3. Main thread processes files sequentially in the correct order

	type workItem struct {
		key   string
		index int
	}
	type workResult struct {
		file    *parquet.File
		rawFile *os.File
		tmpPath string
		key     string
		index   int
		err     error
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8
	}
	if numWorkers > len(plan.Files) {
		numWorkers = len(plan.Files)
	}

	workCh := make(chan workItem, len(plan.Files))
	resultCh := make(chan workResult, len(plan.Files))

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workCh {
				res := workResult{key: item.key, index: item.index}
				f, raw, tmpPath, err := downloadAndOpenFile(ctx, deps.S3, plan.SourceBucket, item.key)
				if err != nil {
					res.err = err
				} else {
					res.file = f
					res.rawFile = raw
					res.tmpPath = tmpPath
				}
				resultCh <- res
			}
		}()
	}

	for i, f := range plan.Files {
		workCh <- workItem{key: f.Key, index: i}
	}
	close(workCh)

	// To keep logs and processing order stable, we use a map to buffer results
	// and process them in order as they become available.
	pending := make(map[int]workResult)
	nextToProcess := 0
	receivedCount := 0

	for receivedCount < len(plan.Files) {
		res := <-resultCh
		pending[res.index] = res
		receivedCount++

		for {
			item, ok := pending[nextToProcess]
			if !ok {
				break
			}
			delete(pending, nextToProcess)
			nextToProcess++

			if item.err != nil {
				return nil, fmt.Errorf("downloading %s: %w", item.key, item.err)
			}

			// Clean up the temp file after processing
			defer os.Remove(item.tmpPath)
			defer item.rawFile.Close()

			if deps.Log != nil {
				deps.Log.Info("processing file",
					"index", item.index+1,
					"total", len(plan.Files),
					"key", item.key,
				)
			}

			conv, err := BuildConversion(plan.TargetSchema, item.file.Schema())
			if err != nil {
				return nil, fmt.Errorf("building conversion for %s: %w", item.key, err)
			}

			for _, rg := range item.file.RowGroups() {
				report.RowsRead += rg.NumRows()
				if err := writer.WriteConvertedRowGroup(rg, conv); err != nil {
					return nil, fmt.Errorf("writing row group from %s: %w", item.key, err)
				}
			}

			// Manually close and remove to avoid defer buildup in the loop
			item.rawFile.Close()
			os.Remove(item.tmpPath)
		}
	}

	wg.Wait()

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing writer: %w", err)
	}

	report.OutputFiles = writer.Outputs()
	report.RowsWritten = writer.RowsWritten()
	report.OutputBytes = writer.TotalBytesWritten()

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

	sourceMB := float64(r.SourceBytes) / (1024 * 1024)
	outputMB := float64(r.OutputBytes) / (1024 * 1024)
	savedPct := 0.0
	if r.SourceBytes > 0 {
		savedPct = (float64(r.SourceBytes-r.OutputBytes) / float64(r.SourceBytes)) * 100
	}

	fmt.Fprintln(w, "Compaction Report")
	fmt.Fprintln(w, "=================")
	fmt.Fprintf(w, "Source files processed: %d (%.2f MB)\n", len(r.SourceFiles), sourceMB)
	fmt.Fprintf(w, "Output files created:   %d (%.2f MB)\n", len(r.OutputFiles), outputMB)
	fmt.Fprintf(w, "Rows read/written:      %d\n", r.RowsRead)
	fmt.Fprintf(w, "Space saved:            %.1f%%\n", savedPct)
	fmt.Fprintf(w, "Total time taken:       %v\n", r.Duration)

	if len(r.DeletedSources) > 0 {
		fmt.Fprintf(w, "Source files deleted:   %d\n", len(r.DeletedSources))
	}
	return nil
}

func downloadAndOpenFile(ctx context.Context, api S3API, bucket, key string) (*parquet.File, *os.File, string, error) {
	resp, err := api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, nil, "", err
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp("", "datalake-src-*.parquet")
	if err != nil {
		return nil, nil, "", err
	}

	size, err := io.Copy(tmp, resp.Body)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, nil, "", err
	}

	if _, err := tmp.Seek(0, 0); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, nil, "", err
	}

	pf, err := parquet.OpenFile(tmp, size)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, nil, "", err
	}

	return pf, tmp, tmp.Name(), nil
}
