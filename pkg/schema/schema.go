package schema

import (
	"context"
	"fmt"
)

// Config holds Glue schema operation parameters.
type Config struct {
	Database string
	Table    string
}

// Describe prints the Glue table schema.
func Describe(ctx context.Context, cfg Config) error {
	return fmt.Errorf("schema: not yet implemented")
}
