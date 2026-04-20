package load

import (
	"fmt"
	"io"
	"time"

	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

type Report struct {
	RecordsRead   int64
	OutputFiles   []string
	OutputBytes   int64
	Schema        []schema.Column
	PartitionKeys []string
	Duration      time.Duration
}

func FormatReport(w io.Writer, r *Report) error {
	fmt.Fprintln(w, "DynamoDB Load Report")
	fmt.Fprintln(w, "====================")
	fmt.Fprintf(w, "Records processed: %d\n", r.RecordsRead)
	fmt.Fprintf(w, "Output files:      %d\n", len(r.OutputFiles))
	fmt.Fprintf(w, "Output size:       %.2f MB\n", float64(r.OutputBytes)/(1024*1024))
	fmt.Fprintf(w, "Total time:        %.2fs\n", r.Duration.Seconds())
	return nil
}
