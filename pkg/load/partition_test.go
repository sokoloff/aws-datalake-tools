package load

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePartitionSpec(t *testing.T) {
	exportTime := time.Date(2024, 4, 18, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		mode     string
		wantNone bool
		wantY    string
		wantM    string
		wantD    string
		wantErr  bool
	}{
		{
			name:     "auto",
			mode:     "auto",
			wantY:    "2024",
			wantM:    "04",
			wantD:    "18",
		},
		{
			name:     "none",
			mode:     "none",
			wantNone: true,
		},
		{
			name:     "manual date",
			mode:     "2023-12-25",
			wantY:    "2023",
			wantM:    "12",
			wantD:    "25",
		},
		{
			name:    "invalid date",
			mode:    "bad-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ResolvePartitionSpec(exportTime, tt.mode)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNone, spec.None)
			if !tt.wantNone {
				assert.Equal(t, tt.wantY, spec.Year)
				assert.Equal(t, tt.wantM, spec.Month)
				assert.Equal(t, tt.wantD, spec.Day)
			}
		})
	}
}

func TestPartitionSpec_SubPrefix(t *testing.T) {
	spec := PartitionSpec{Year: "2024", Month: "04", Day: "18"}
	assert.Equal(t, "year=2024/month=04/day=18/", spec.SubPrefix())

	specNone := PartitionSpec{None: true}
	assert.Equal(t, "", specNone.SubPrefix())
}

func TestPartitionSpec_PartitionKeys(t *testing.T) {
	spec := PartitionSpec{Year: "2024"}
	assert.Equal(t, []string{"year", "month", "day"}, spec.PartitionKeys())

	specNone := PartitionSpec{None: true}
	assert.Nil(t, specNone.PartitionKeys())
}

func TestPartitionSpec_GetPartitionPath(t *testing.T) {
	spec := PartitionSpec{Year: "2024", Month: "04", Day: "18"}
	assert.Equal(t, "s3://bucket/prefix/year=2024/month=04/day=18/", spec.GetPartitionPath("s3://bucket/prefix/"))
	assert.Equal(t, "s3://bucket/prefix/year=2024/month=04/day=18/", spec.GetPartitionPath("s3://bucket/prefix"))

	specNone := PartitionSpec{None: true}
	assert.Equal(t, "s3://bucket/prefix", specNone.GetPartitionPath("s3://bucket/prefix"))
}
