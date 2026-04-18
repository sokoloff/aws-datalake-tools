package compact

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/parquet-go/parquet-go"
	"github.com/sokoloff/aws-datalake-tools/pkg/s3util"
	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

type PlanMode int

const (
	ModePassThrough PlanMode = iota
	ModeGlueCoerced
)

type CompactionPlan struct {
	SourceBucket  string
	SourcePrefix  string
	TargetBucket  string
	TargetPrefix  string
	Files         []s3util.Object
	TargetColumns []schema.Column
	TargetSchema  *parquet.Schema
	Mode          PlanMode
	GlueSchema    *schema.TableSchema
	Incompatible  []string
}

func BuildPlan(ctx context.Context, deps Deps, cfg Config) (*CompactionPlan, error) {
	srcBucket, srcPrefix, err := s3util.ParseS3URI(cfg.SourceURI)
	if err != nil {
		return nil, fmt.Errorf("parsing source URI: %w", err)
	}

	tgtBucket, tgtPrefix, err := s3util.ParseS3URI(cfg.TargetURI)
	if err != nil {
		return nil, fmt.Errorf("parsing target URI: %w", err)
	}

	objects, err := s3util.ListObjects(ctx, deps.S3, srcBucket, srcPrefix)
	if err != nil {
		return nil, fmt.Errorf("listing source objects: %w", err)
	}

	var files []s3util.Object
	for _, obj := range objects {
		if strings.HasSuffix(obj.Key, ".parquet") {
			files = append(files, obj)
			if cfg.MaxFiles > 0 && len(files) >= cfg.MaxFiles {
				break
			}
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no parquet files found in %s", cfg.SourceURI)
	}

	plan := &CompactionPlan{
		SourceBucket: srcBucket,
		SourcePrefix: srcPrefix,
		TargetBucket: tgtBucket,
		TargetPrefix: tgtPrefix,
		Files:        files,
	}

	if cfg.Database != "" && cfg.Table != "" {
		plan.Mode = ModeGlueCoerced
		glueSchema, err := schema.FetchTableSchema(ctx, deps.Glue, cfg.Database, cfg.Table)
		if err != nil {
			return nil, fmt.Errorf("fetching glue schema: %w", err)
		}
		plan.GlueSchema = glueSchema
		plan.TargetColumns = glueSchema.Columns
	} else {
		plan.Mode = ModePassThrough
		firstFileSchema, err := schema.ReadParquetSchemaFromS3(ctx, deps.S3, srcBucket, files[0].Key)
		if err != nil {
			return nil, fmt.Errorf("reading schema from first file: %w", err)
		}
		plan.TargetColumns = firstFileSchema
	}

	targetNode, err := ColumnsToParquetGroup(plan.TargetColumns)
	if err != nil {
		return nil, fmt.Errorf("building target parquet node: %w", err)
	}
	plan.TargetSchema = parquet.NewSchema("schema", targetNode)

	// Validate coercion
	for _, f := range plan.Files {
		fileCols, err := schema.ReadParquetSchemaFromS3(ctx, deps.S3, plan.SourceBucket, f.Key)
		if err != nil {
			return nil, fmt.Errorf("reading schema from %s: %w", f.Key, err)
		}

		summary, err := ValidateCoercion(plan.TargetColumns, fileCols)
		if err != nil {
			plan.Incompatible = append(plan.Incompatible, fmt.Sprintf("%s: %s", f.Key, summary))
		}
	}

	if len(plan.Incompatible) > 0 {
		return plan, fmt.Errorf("incompatible schemas found in %d files", len(plan.Incompatible))
	}

	return plan, nil
}

func (p *CompactionPlan) Describe(w io.Writer) {
	fmt.Fprintf(w, "Compaction Plan (Dry Run)\n")
	fmt.Fprintf(w, "=========================\n")
	fmt.Fprintf(w, "Mode:          %v\n", map[PlanMode]string{ModePassThrough: "Pass-Through", ModeGlueCoerced: "Glue Coerced"}[p.Mode])
	fmt.Fprintf(w, "Source:        s3://%s/%s\n", p.SourceBucket, p.SourcePrefix)
	fmt.Fprintf(w, "Target:        s3://%s/%s\n", p.TargetBucket, p.TargetPrefix)
	fmt.Fprintf(w, "Files to scan: %d\n", len(p.Files))

	fmt.Fprintf(w, "\nTarget Schema Columns:\n")
	for _, col := range p.TargetColumns {
		fmt.Fprintf(w, "  - %s: %s\n", col.Name, col.Type)
	}

	if len(p.Incompatible) > 0 {
		fmt.Fprintf(w, "\nIncompatible Files:\n")
		for _, msg := range p.Incompatible {
			fmt.Fprintf(w, "%s\n", msg)
		}
	}
}
