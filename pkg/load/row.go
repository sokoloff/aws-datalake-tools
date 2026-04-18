package load

import (
	"encoding/json"
	"sort"

	"github.com/parquet-go/parquet-go"
)

// RowPlanner converts generic map records to parquet.Row based on a schema.
type RowPlanner struct {
	schema *parquet.Schema
	leaves []leafPlan
}

type leafPlan struct {
	path   []string
	index  int
	maxDef int
	maxRep int
	node   parquet.Node
}

// NewRowPlanner initializes a row planner for the given parquet schema and its root node.
func NewRowPlanner(schema *parquet.Schema, root parquet.Node) *RowPlanner {
	p := &RowPlanner{schema: schema}
	p.leaves = p.buildLeafPlans(root, nil, 0, 0)

	// Map schema column paths to their index
	schemaCols := schema.Columns() // Returns [][]string
	for i, colPath := range schemaCols {
		for j := range p.leaves {
			if sliceEqual(p.leaves[j].path, colPath) {
				p.leaves[j].index = i
			}
		}
	}

	// Sort leaves by column index to ensure we emit parquet.Values in order
	sort.Slice(p.leaves, func(i, j int) bool {
		return p.leaves[i].index < p.leaves[j].index
	})

	return p
}

func (p *RowPlanner) buildLeafPlans(node parquet.Node, path []string, def, rep int) []leafPlan {
	if node.Optional() {
		def++
	}
	if node.Repeated() {
		def++
		rep++
	}

	if node.Leaf() {
		return []leafPlan{{
			path:   path,
			maxDef: def,
			maxRep: rep,
			node:   node,
		}}
	}

	var leaves []leafPlan
	fields := node.Fields()
	for _, f := range fields {
		childPath := append([]string{}, path...)
		childPath = append(childPath, f.Name())
		leaves = append(leaves, p.buildLeafPlans(f, childPath, def, rep)...)
	}
	return leaves
}

// Build converts a record to a parquet.Row.
func (p *RowPlanner) Build(record map[string]any, buf []parquet.Value) (parquet.Row, error) {
	row := make(parquet.Row, 0, len(p.leaves))
	for _, leaf := range p.leaves {
		val := getValue(record, leaf.path)

		var pVal parquet.Value
		if val == nil {
			pVal = parquet.ValueOf(nil).Level(0, leaf.maxDef-1, leaf.index)
		} else {
			actual := val
			if num, ok := val.(json.Number); ok {
				if i, err := num.Int64(); err == nil {
					actual = i
				} else if f, err := num.Float64(); err == nil {
					actual = f
				}
			}
			pVal = parquet.ValueOf(actual).Level(0, leaf.maxDef, leaf.index)
		}
		row = append(row, pVal)
	}
	return row, nil
}

func getValue(m map[string]any, path []string) any {
	var current any = m
	for _, part := range path {
		if curMap, ok := current.(map[string]any); ok {
			current = curMap[part]
		} else {
			return nil
		}
	}
	return current
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
