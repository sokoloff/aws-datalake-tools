package load

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/sokoloff/aws-datalake-tools/internal/dynamo"
)

// DynamoDB items can be up to 400KB; a 1MB scanner buffer gives headroom.
const scannerMaxTokenSize = 1024 * 1024

// runInferencePass walks every manifest entry, observes each record with the
// inferrer, and spools the normalized JSON to a gzip'd temp file for pass 2.
// The caller owns the returned spool path and must os.Remove it.
func runInferencePass(ctx context.Context, src DumpSource, files []ManifestEntry, cfg Config, log *slog.Logger) (spoolPath string, recordsProcessed int64, inferrer *Inferrer, err error) {
	inferrer = NewInferrer()
	spoolPath = filepath.Join(os.TempDir(), fmt.Sprintf("datalake-spool-%d.json.gz", time.Now().UnixNano()))
	spoolFile, err := os.Create(spoolPath)
	if err != nil {
		return "", 0, nil, fmt.Errorf("creating spool file: %w", err)
	}
	defer spoolFile.Close()

	gzipWriter := gzip.NewWriter(spoolFile)
	defer gzipWriter.Close()

	for _, entry := range files {
		if err := processFilePass1(ctx, src, entry, inferrer, gzipWriter, &recordsProcessed, cfg, log); err != nil {
			return spoolPath, recordsProcessed, inferrer, err
		}
	}
	return spoolPath, recordsProcessed, inferrer, nil
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
	scanner.Buffer(make([]byte, scannerMaxTokenSize), scannerMaxTokenSize)

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

		data, err := json.Marshal(norm)
		if err != nil {
			return fmt.Errorf("marshaling record: %w", err)
		}
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("writing spool: %w", err)
		}
		if _, err := out.Write([]byte("\n")); err != nil {
			return fmt.Errorf("writing spool: %w", err)
		}

		*count++
		if cfg.SampleSize > 0 && *count >= int64(cfg.SampleSize) {
			break
		}
	}
	return scanner.Err()
}
