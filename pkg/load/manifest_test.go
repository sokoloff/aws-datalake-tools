package load

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestSummary_Unmarshal(t *testing.T) {
	data := `{
		"exportTime": "2024-04-18T10:00:00Z",
		"itemCount": 100,
		"outputFormat": "DYNAMODB_JSON"
	}`
	var sum ManifestSummary
	err := json.Unmarshal([]byte(data), &sum)
	require.NoError(t, err)

	assert.Equal(t, int64(100), sum.ItemCount)
	assert.Equal(t, "DYNAMODB_JSON", sum.OutputFormat)
	assert.Equal(t, 2024, sum.ExportTime.Year())
	assert.Equal(t, time.Month(4), sum.ExportTime.Month())
}

func TestManifestEntry_Unmarshal(t *testing.T) {
	data := `{"itemCount":10,"md5Checksum":"abc","etag":"123","dataFileS3Key":"data/1.gz"}`
	var entry ManifestEntry
	err := json.Unmarshal([]byte(data), &entry)
	require.NoError(t, err)

	assert.Equal(t, int64(10), entry.ItemCount)
	assert.Equal(t, "abc", entry.MD5Checksum)
	assert.Equal(t, "data/1.gz", entry.DataFileS3Key)
}
