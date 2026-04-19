package shared

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// WriteTestDDBDump writes a test DynamoDB export to the specified directory.
func WriteTestDDBDump(t *testing.T, dir string, records []map[string]any) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, "AWSDynamoDB", "0123", "data"), 0755); err != nil {
		t.Fatalf("failed to create DDB dump dirs: %v", err)
	}

	summaryPath := filepath.Join(dir, "manifest-summary.json")
	summaryContent := map[string]any{
		"exportTime":   "2024-04-18T10:00:00Z",
		"itemCount":    len(records),
		"outputFormat": "DYNAMODB_JSON",
	}
	summaryBytes, _ := json.Marshal(summaryContent)
	if err := os.WriteFile(summaryPath, summaryBytes, 0644); err != nil {
		t.Fatalf("failed to write manifest-summary.json: %v", err)
	}

	filesPath := filepath.Join(dir, "manifest-files.json")
	filesContent := map[string]any{
		"itemCount":     len(records),
		"md5Checksum":   "abc",
		"etag":          "123",
		"dataFileS3Key": "AWSDynamoDB/0123/data/part-00000.json.gz",
	}
	filesBytes, _ := json.Marshal(filesContent)
	if err := os.WriteFile(filesPath, filesBytes, 0644); err != nil {
		t.Fatalf("failed to write manifest-files.json: %v", err)
	}

	dataPath := filepath.Join(dir, "AWSDynamoDB", "0123", "data", "part-00000.json.gz")
	f, err := os.Create(dataPath)
	if err != nil {
		t.Fatalf("failed to create data file: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	for _, rec := range records {
		wrapper := map[string]any{"Item": rec}
		b, _ := json.Marshal(wrapper)
		b = append(b, '\n')
		gw.Write(b)
	}
}
