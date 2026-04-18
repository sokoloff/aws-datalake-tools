package load

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/internal/awsclient"
	"github.com/sokoloff/aws-datalake-tools/internal/dynamo"
	"github.com/sokoloff/aws-datalake-tools/internal/logging"
	"github.com/sokoloff/aws-datalake-tools/pkg/compact"
	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

type Config struct {
	InputURI        string
	OutputURI       string
	Database        string
	Table           string
	SchemaFile      string
	InferOnly       bool
	SampleSize      int
	TargetSizeMB    int64
	Partition       string // auto, none, or YYYY-MM-DD
	ReplaceIfExists bool
	DryRun          bool
}

type Deps struct {
	S3   compact.S3API
	Glue schema.GlueTableAPI
	Log  *slog.Logger
}

type Report struct {
	RecordsRead    int64
	OutputFiles    []string
	OutputBytes    int64
	Schema         []schema.Column
	PartitionKeys  []string
	Duration       time.Duration
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

	// Pass 1: Schema Inference + Spooling
	inferrer := NewInferrer()
	spoolPath := filepath.Join(os.TempDir(), fmt.Sprintf("datalake-spool-%d.json.gz", time.Now().UnixNano()))
	spoolFile, err := os.Create(spoolPath)
	if err != nil {
		return nil, fmt.Errorf("creating spool file: %w", err)
	}
	defer os.Remove(spoolPath)

	gzipWriter := gzip.NewWriter(spoolFile)
	var recordsProcessed int64

	for _, entry := range files {
		if err := processFilePass1(ctx, source, entry, inferrer, gzipWriter, &recordsProcessed, cfg, deps.Log); err != nil {
			spoolFile.Close()
			return nil, err
		}
	}
	gzipWriter.Close()
	spoolFile.Close()

	var finalCols []schema.Column
	if cfg.SchemaFile != "" {
		data, err := os.ReadFile(cfg.SchemaFile)
		if err != nil {
			return nil, fmt.Errorf("reading schema file: %w", err)
		}
		if err := json.Unmarshal(data, &finalCols); err != nil {
			return nil, fmt.Errorf("unmarshaling schema file: %w", err)
		}
	} else {
		finalCols = inferrer.Finalize()
	}

	if cfg.InferOnly {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(finalCols)
		return &Report{RecordsRead: recordsProcessed, Schema: finalCols}, nil
	}

	// Pass 2: Write Parquet
	targetNode, err := compact.ColumnsToParquetGroup(finalCols)
	if err != nil {
		return nil, fmt.Errorf("building parquet node: %w", err)
	}
	parquetSchema := parquet.NewSchema("dynamodb_record", targetNode)
	planner := NewRowPlanner(parquetSchema, targetNode)

	rollBytes := cfg.TargetSizeMB * 1024 * 1024
	if rollBytes <= 0 {
		rollBytes = 128 * 1024 * 1024
	}

	var bucket, targetPrefix string
	var uploadFn compact.UploadFunc

	if strings.HasPrefix(cfg.OutputURI, "s3://") {
		var err error
		bucket, targetPrefix, err = s3util.ParseS3URI(cfg.OutputURI)
		if err != nil {
			return nil, err
		}
		uploadFn = compact.NewS3Uploader(deps.S3, bucket)
	} else {
		targetPrefix = cfg.OutputURI
		uploadFn = compact.NewLocalUploader()
	}

	targetPrefix = filepath.Join(targetPrefix, partition.SubPrefix())
	writer, err := compact.NewRollingWriter(ctx, parquetSchema, rollBytes, bucket, targetPrefix, uploadFn)
	if err != nil {
		return nil, err
	}

	defer writer.Close()

	if err := processSpoolPass2(spoolPath, planner, writer); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	// Glue Registration
	if cfg.Database != "" && cfg.Table != "" && !cfg.DryRun {
		var pkCols []schema.Column
		for _, k := range partition.PartitionKeys() {
			pkCols = append(pkCols, schema.Column{Name: k, Type: schema.PrimitiveType{Kind: schema.String}})
		}

		tableInput := schema.CreateTableInput{
			Database:      cfg.Database,
			Table:         cfg.Table,
			Location:      cfg.OutputURI,
			Columns:       finalCols,
			PartitionKeys: pkCols,
			Replace:       cfg.ReplaceIfExists,
		}

		// Note: PartitionKeys logic in schema.CreateTable might need adjustment
		// but we'll use existing pkg/schema logic.
		if err := schema.CreateTable(ctx, deps.Glue, tableInput); err != nil {
			return nil, fmt.Errorf("registering glue table: %w", err)
		}
	}

	return &Report{
		RecordsRead:   recordsProcessed,
		OutputFiles:   writer.Outputs(),
		OutputBytes:   writer.TotalBytesWritten(),
		Schema:        finalCols,
		PartitionKeys: partition.PartitionKeys(),
		Duration:      time.Since(start),
	}, nil
}

func processFilePass1(ctx context.Context, src DumpSource, entry ManifestEntry, inf *Inferrer, out *gzip.Writer, count *int64, cfg Config, log *slog.Logger) error {
	rc, err := src.OpenDataFile(ctx, entry.DataFileS3Key)
	if err != nil {
		return err
	}
	defer rc.Close()

	gr, err := gzip.NewReader(rc)
	if err != nil {
		return err
	}
	defer gr.Close()

	scanner := bufio.NewScanner(gr)
	// DynamoDB items can be up to 400KB. We'll use a 1MB buffer.
	const maxTokenSize = 1024 * 1024
	scanner.Buffer(make([]byte, maxTokenSize), maxTokenSize)

	for scanner.Scan() {
		rec, err := dynamo.ParseRecord(scanner.Bytes())
		if err != nil {
			return err
		}
		norm, err := dynamo.NormalizeKeys(rec)
		if err != nil {
			return err
		}

		inf.Observe(norm)

		data, _ := json.Marshal(norm)
		out.Write(data)
		out.Write([]byte("\n"))

		*count++
		if cfg.SampleSize > 0 && *count >= int64(cfg.SampleSize) {
			break
		}
	}
	return scanner.Err()
}

func processSpoolPass2(path string, planner *RowPlanner, writer *compact.RollingWriter) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	scanner := bufio.NewScanner(gr)
	const maxTokenSize = 1024 * 1024
	scanner.Buffer(make([]byte, maxTokenSize), maxTokenSize)

	var batch []parquet.Row
	for scanner.Scan() {
		var rec map[string]any
		json.Unmarshal(scanner.Bytes(), &rec)

		row, err := planner.Build(rec, nil)
		if err != nil {
			return err
		}
		batch = append(batch, row)

		if len(batch) >= 1000 {
			if _, err := writer.WriteRows(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if _, err := writer.WriteRows(batch); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func FormatReport(w io.Writer, r *Report) error {
	fmt.Fprintln(w, "DynamoDB Load Report")
	fmt.Fprintln(w, "====================")
	fmt.Fprintf(w, "Records processed: %d\n", r.RecordsRead)
	fmt.Fprintf(w, "Output files:      %d\n", len(r.OutputFiles))
	fmt.Fprintf(w, "Total time:        %v\n", r.Duration)
	return nil
}
