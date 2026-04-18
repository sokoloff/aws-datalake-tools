package load

import (
	"fmt"
	"strings"
	"time"
)

// PartitionSpec defines how the output data should be partitioned.
type PartitionSpec struct {
	Year  string
	Month string
	Day   string
	None  bool
}

// ResolvePartitionSpec resolves the partitioning based on the export time and mode.
func ResolvePartitionSpec(exportTime time.Time, mode string) (PartitionSpec, error) {
	if mode == "none" {
		return PartitionSpec{None: true}, nil
	}

	targetTime := exportTime
	if mode != "auto" {
		t, err := time.Parse("2006-01-02", mode)
		if err != nil {
			return PartitionSpec{}, fmt.Errorf("invalid partition date format (expected YYYY-MM-DD or 'auto'/'none'): %w", err)
		}
		targetTime = t
	}

	return PartitionSpec{
		Year:  targetTime.Format("2006"),
		Month: targetTime.Format("01"),
		Day:   targetTime.Format("02"),
	}, nil
}

// SubPrefix returns the S3 prefix fragment for the partitions, e.g. "year=2024/month=01/day=01/".
func (p PartitionSpec) SubPrefix() string {
	if p.None {
		return ""
	}
	return fmt.Sprintf("year=%s/month=%s/day=%s/", p.Year, p.Month, p.Day)
}

// PartitionKeys returns the partition columns for Glue.
func (p PartitionSpec) PartitionKeys() []string {
	if p.None {
		return nil
	}
	return []string{"year", "month", "day"}
}

// GetPartitionPath joins a base prefix with the sub-prefix.
func (p PartitionSpec) GetPartitionPath(base string) string {
	if p.None {
		return base
	}
	res := base
	if !strings.HasSuffix(res, "/") {
		res += "/"
	}
	return res + p.SubPrefix()
}
