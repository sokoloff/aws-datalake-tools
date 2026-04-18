package compact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// NewLocalUploader creates an UploadFunc that writes to the local filesystem.
func NewLocalUploader() UploadFunc {
	return func(ctx context.Context, path string, body io.Reader, size int64) error {
		// Ensure the parent directory exists
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}

		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("creating file %s: %w", path, err)
		}
		defer f.Close()

		if _, err := io.Copy(f, body); err != nil {
			return fmt.Errorf("writing to %s: %w", path, err)
		}

		return nil
	}
}
