package shared

import (
	"testing"

	"github.com/parquet-go/parquet-go"
)

// WriteTestParquet writes a slice of structs to a parquet file at the given path.
// The data must be a slice of structs that parquet-go can reflect upon.
func WriteTestParquet[T any](t *testing.T, path string, data []T) {
	t.Helper()

	if err := parquet.WriteFile(path, data); err != nil {
		t.Fatalf("failed to write parquet data to %s: %v", path, err)
	}
}
