package load

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

// resolveFinalSchema returns the column list to use for the write pass. If
// cfg.SchemaFile is set it is read from disk; otherwise the inferrer's result
// is used. Metadata columns are appended when cfg.InjectMetadataColumns is set.
func resolveFinalSchema(cfg Config, inf *Inferrer) ([]schema.Column, error) {
	var cols []schema.Column
	if cfg.SchemaFile != "" {
		loaded, err := loadSchemaFromFile(cfg.SchemaFile)
		if err != nil {
			return nil, err
		}
		cols = loaded
	} else {
		cols = inf.Finalize()
	}

	if cfg.InjectMetadataColumns {
		cols = append(cols,
			schema.Column{Name: "eventname", Type: schema.PrimitiveType{Kind: schema.String}},
			schema.Column{Name: "eventcreationdatetime", Type: schema.PrimitiveType{Kind: schema.Timestamp}},
		)
	}
	return cols, nil
}

func loadSchemaFromFile(path string) ([]schema.Column, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schema file: %w", err)
	}
	var intermediate []struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Comment string `json:"comment"`
	}
	if err := json.Unmarshal(data, &intermediate); err != nil {
		return nil, fmt.Errorf("unmarshaling schema file: %w", err)
	}
	cols := make([]schema.Column, 0, len(intermediate))
	for _, col := range intermediate {
		dt, err := schema.ParseType(col.Type)
		if err != nil {
			return nil, fmt.Errorf("parsing type for column %s: %w", col.Name, err)
		}
		cols = append(cols, schema.Column{Name: col.Name, Type: dt, Comment: col.Comment})
	}
	return cols, nil
}

// emitInferOnlyReport pretty-prints the resolved schema to w and returns an
// infer-only Report containing only the schema and record count.
func emitInferOnlyReport(w io.Writer, cols []schema.Column, recordsRead int64) (*Report, error) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cols); err != nil {
		return nil, err
	}
	return &Report{RecordsRead: recordsRead, Schema: cols}, nil
}
