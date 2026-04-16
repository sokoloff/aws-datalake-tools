package load

import (
	"context"
	"fmt"
)

// Config holds DynamoDB initial load parameters.
type Config struct {
	InputURI        string
	OutputURI       string
	Database        string
	Table           string
	ReplaceIfExists bool
}

// Run executes the DynamoDB dump to parquet conversion.
func Run(ctx context.Context, cfg Config) error {
	return fmt.Errorf("load: not yet implemented")
}
