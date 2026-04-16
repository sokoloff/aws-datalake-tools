package compact

import (
	"context"
	"fmt"
)

// Config holds compaction parameters.
type Config struct {
	SourceURI string
	TargetURI string
	Database  string
	Table     string
}

// Run executes the compaction process.
func Run(ctx context.Context, cfg Config) error {
	return fmt.Errorf("compact: not yet implemented")
}
