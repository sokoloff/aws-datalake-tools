package compact

import (
	"fmt"
	"io"
	"time"
)

type Report struct {
	SourceFiles    []string
	OutputFiles    []string
	DeletedSources []string
	RowsRead       int64
	RowsWritten    int64
	SourceBytes    int64
	OutputBytes    int64
	Duration       time.Duration
	DryRun         bool
}

func FormatReport(w io.Writer, r *Report) error {
	if r.DryRun {
		fmt.Fprintln(w, "Dry run complete. No files were modified.")
		return nil
	}

	sourceMB := float64(r.SourceBytes) / (1024 * 1024)
	outputMB := float64(r.OutputBytes) / (1024 * 1024)
	savedPct := 0.0
	if r.SourceBytes > 0 {
		savedPct = (float64(r.SourceBytes-r.OutputBytes) / float64(r.SourceBytes)) * 100
	}

	fmt.Fprintln(w, "Compaction Report")
	fmt.Fprintln(w, "=================")
	fmt.Fprintf(w, "Source files processed: %d (%.2f MB)\n", len(r.SourceFiles), sourceMB)
	fmt.Fprintf(w, "Output files created:   %d (%.2f MB)\n", len(r.OutputFiles), outputMB)
	fmt.Fprintf(w, "Rows read/written:      %d\n", r.RowsRead)
	fmt.Fprintf(w, "Space saved:            %.1f%%\n", savedPct)
	fmt.Fprintf(w, "Total time taken:       %v\n", r.Duration)

	if len(r.DeletedSources) > 0 {
		fmt.Fprintf(w, "Source files deleted:   %d\n", len(r.DeletedSources))
	}
	return nil
}
