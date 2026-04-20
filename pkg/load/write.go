package load

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/pkg/compact"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

// runWritePass reads the spool file, converts each record into a parquet row
// via the RowPlanner, and streams them through a rolling writer that lands the
// output at the resolved target location. Returns the output file list and the
// total bytes written.
func runWritePass(ctx context.Context, spoolPath string, cols []schema.Column, partition PartitionSpec, cfg Config, deps Deps) ([]string, int64, error) {
	targetNode, err := compact.ColumnsToParquetGroup(cols)
	if err != nil {
		return nil, 0, fmt.Errorf("building parquet node: %w", err)
	}
	parquetSchema := parquet.NewSchema("dynamodb_record", targetNode)
	planner := NewRowPlanner(parquetSchema, targetNode)

	rollBytes := cfg.TargetSizeMB * 1024 * 1024
	if rollBytes <= 0 {
		rollBytes = 128 * 1024 * 1024
	}

	bucket, prefix, uploadFn, err := resolveTarget(cfg, partition, deps)
	if err != nil {
		return nil, 0, err
	}

	writer, err := compact.NewRollingWriter(ctx, parquetSchema, rollBytes, bucket, prefix, uploadFn)
	if err != nil {
		return nil, 0, err
	}
	closed := false
	defer func() {
		if !closed {
			writer.Close()
		}
	}()

	if err := processSpoolPass2(spoolPath, planner, writer); err != nil {
		return nil, 0, err
	}

	if err := writer.Close(); err != nil {
		return nil, 0, err
	}
	closed = true

	return writer.Outputs(), writer.TotalBytesWritten(), nil
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
	scanner.Buffer(make([]byte, scannerMaxTokenSize), scannerMaxTokenSize)

	var batch []parquet.Row
	for scanner.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return fmt.Errorf("unmarshaling spool record: %w", err)
		}

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
