package compact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
)

// UploadFunc is an abstraction for uploading a file.
type UploadFunc func(ctx context.Context, key string, body io.Reader, size int64) error

// RollingWriter writes parquet row groups to local temporary files
// and uploads them once they reach rollBytes in size.
type RollingWriter struct {
	ctx          context.Context
	node         parquet.Node
	schema       *parquet.Schema
	rollBytes    int64
	targetBucket string
	targetPrefix string
	uploadFn     UploadFunc
	tmpDir       string

	curFile      *os.File
	curWriter    *parquet.GenericWriter[any]
	curRows      int64
	totalRows    int64
	fileIndex    int
	outputs      []string
}

// NewRollingWriter initializes a RollingWriter.
func NewRollingWriter(ctx context.Context, schema *parquet.Schema, rollBytes int64, targetBucket, targetPrefix string, upload UploadFunc) (*RollingWriter, error) {
	tmpDir, err := os.MkdirTemp("", "datalake-compact-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	w := &RollingWriter{
		ctx:          ctx,
		schema:       schema,
		rollBytes:    rollBytes,
		targetBucket: targetBucket,
		targetPrefix: targetPrefix,
		uploadFn:     upload,
		tmpDir:       tmpDir,
	}

	if err := w.openNewFile(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	return w, nil
}

func (w *RollingWriter) openNewFile() error {
	id := uuid.New().String()
	filename := fmt.Sprintf("part-%05d-%s.parquet", w.fileIndex, id)
	path := filepath.Join(w.tmpDir, filename)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	pw := parquet.NewGenericWriter[any](f, w.schema)

	w.curFile = f
	w.curWriter = pw
	w.curRows = 0
	return nil
}

func (w *RollingWriter) roll() error {
	if w.curWriter == nil {
		return nil
	}

	if err := w.curWriter.Close(); err != nil {
		return fmt.Errorf("closing parquet writer: %w", err)
	}

	info, err := w.curFile.Stat()
	if err != nil {
		return fmt.Errorf("stat temp file: %w", err)
	}
	size := info.Size()

	if _, err := w.curFile.Seek(0, 0); err != nil {
		return fmt.Errorf("seek temp file: %w", err)
	}

	key := filepath.Join(w.targetPrefix, filepath.Base(w.curFile.Name()))
	if err := w.uploadFn(w.ctx, key, w.curFile, size); err != nil {
		return fmt.Errorf("uploading %s: %w", key, err)
	}

	w.outputs = append(w.outputs, key)

	if err := w.curFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Remove(w.curFile.Name()); err != nil {
		return fmt.Errorf("removing temp file: %w", err)
	}

	w.curWriter = nil
	w.curFile = nil
	w.fileIndex++

	return w.openNewFile()
}

// WriteConvertedRowGroup converts the input row group and writes it to the current output file.
func (w *RollingWriter) WriteConvertedRowGroup(rg parquet.RowGroup, conv parquet.Conversion) error {
	convertedRg := parquet.ConvertRowGroup(rg, conv)
	rows := convertedRg.Rows()
	defer rows.Close()

	n, err := parquet.CopyRows(w.curWriter, rows)
	if err != nil {
		return fmt.Errorf("copying rows: %w", err)
	}
	w.curRows += n
	w.totalRows += n

	if err := w.curWriter.Flush(); err != nil {
		return fmt.Errorf("flush writer: %w", err)
	}

	info, err := w.curFile.Stat()
	if err != nil {
		return fmt.Errorf("stat temp file: %w", err)
	}

	if info.Size() >= w.rollBytes {
		if err := w.roll(); err != nil {
			return err
		}
	}

	return nil
}

// Outputs returns the list of uploaded keys.
func (w *RollingWriter) Outputs() []string {
	return w.outputs
}

// RowsWritten returns the total number of rows written across all files.
func (w *RollingWriter) RowsWritten() int64 {
	return w.totalRows
}

// Close finalizes any pending writes, uploads the final part if it has rows, and cleans up.
func (w *RollingWriter) Close() error {
	defer os.RemoveAll(w.tmpDir)

	if w.curWriter != nil && w.curRows > 0 {
		if err := w.roll(); err != nil {
			return err
		}
	} else if w.curWriter != nil {
		// Close empty writer but don't upload
		w.curWriter.Close()
		w.curFile.Close()
	}

	return nil
}
