package load

import (
	"context"
	"fmt"

	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

// registerGlueTable creates (or replaces) the Glue table for the written data
// and, when the partition spec is not 'none', registers the single partition
// produced by this run.
func registerGlueTable(ctx context.Context, cfg Config, cols []schema.Column, partition PartitionSpec, deps Deps) error {
	var pkCols []schema.Column
	for _, k := range partition.PartitionKeys() {
		pkCols = append(pkCols, schema.Column{Name: k, Type: schema.PrimitiveType{Kind: schema.String}})
	}

	tableInput := schema.CreateTableInput{
		Database:      cfg.Database,
		Table:         cfg.Table,
		Location:      cfg.OutputURI,
		Columns:       cols,
		PartitionKeys: pkCols,
		Parameters:    map[string]string{},
		Replace:       cfg.ReplaceIfExists,
	}

	if err := schema.CreateTable(ctx, deps.Glue, tableInput); err != nil {
		return fmt.Errorf("registering glue table: %w", err)
	}

	if partition.None {
		return nil
	}

	partLocation := partition.GetPartitionPath(cfg.OutputURI)
	err := schema.CreatePartition(ctx, deps.Glue, schema.CreatePartitionInput{
		Database:        cfg.Database,
		Table:           cfg.Table,
		PartitionValues: []string{partition.Year, partition.Month, partition.Day},
		Location:        partLocation,
	})
	if err != nil {
		return fmt.Errorf("adding partition to glue table: %w", err)
	}
	return nil
}
