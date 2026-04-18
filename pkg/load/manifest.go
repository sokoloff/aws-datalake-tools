package load

import (
	"time"
)

// ManifestSummary represents the manifest-summary.json in a DynamoDB export.
type ManifestSummary struct {
	ExportTime   time.Time `json:"exportTime"`
	ItemCount    int64     `json:"itemCount"`
	OutputFormat string    `json:"outputFormat"` // Should be "DYNAMODB_JSON"
}

// ManifestEntry represents one line in manifest-files.json.
type ManifestEntry struct {
	ItemCount      int64  `json:"itemCount"`
	MD5Checksum    string `json:"md5Checksum"`
	ETag           string `json:"etag"`
	DataFileS3Key  string `json:"dataFileS3Key"`
}
