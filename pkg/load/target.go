package load

import (
	"path/filepath"
	"strings"

	"github.com/sokoloff/aws-datalake-tools/pkg/compact"
	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
)

// resolveTarget decides whether output goes to S3 or to a local directory and
// returns the bucket (empty for local), the final prefix (base prefix joined
// with the partition sub-prefix), and the uploader function.
func resolveTarget(cfg Config, partition PartitionSpec, deps Deps) (bucket, prefix string, up compact.UploadFunc, err error) {
	if strings.HasPrefix(cfg.OutputURI, "s3://") {
		bucket, prefix, err = s3util.ParseS3URI(cfg.OutputURI)
		if err != nil {
			return "", "", nil, err
		}
		up = compact.NewS3Uploader(deps.S3, bucket)
	} else {
		prefix = cfg.OutputURI
		up = compact.NewLocalUploader()
	}

	prefix = filepath.Join(prefix, partition.SubPrefix())
	return bucket, prefix, up, nil
}
