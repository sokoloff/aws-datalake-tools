package load

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
)

// Inferrer accumulates observations about fields to infer a schema.
type Inferrer struct {
	root fieldObs
}

type fieldObs struct {
	name            string
	count           int64
	observedKinds   map[schema.PrimitiveKind]int64
	observedDecimals map[string]int64 // "precision,scale"
	children        map[string]*fieldObs
	listElement     *fieldObs
	mapKey          *fieldObs
	mapValue        *fieldObs
	
	// For map vs struct heuristic
	distinctKeysets map[string]int64 // marshaled sorted keys -> count
}

func newFieldObs(name string) *fieldObs {
	return &fieldObs{
		name:            name,
		observedKinds:   make(map[schema.PrimitiveKind]int64),
		observedDecimals: make(map[string]int64),
		children:        make(map[string]*fieldObs),
		distinctKeysets: make(map[string]int64),
	}
}

// NewInferrer initializes a schema inferrer.
func NewInferrer() *Inferrer {
	return &Inferrer{root: *newFieldObs("")}
}

// Observe records observations about a single record.
func (i *Inferrer) Observe(record map[string]any) {
	i.root.observeMap(record)
}

func (o *fieldObs) observeValue(v any) {
	o.count++
	if v == nil {
		return
	}

	switch val := v.(type) {
	case string:
		o.observedKinds[schema.String]++
	case bool:
		o.observedKinds[schema.Boolean]++
	case json.Number:
		if _, err := val.Int64(); err == nil {
			o.observedKinds[schema.BigInt]++
		} else {
			o.observedKinds[schema.Double]++
		}
	case []byte:
		o.observedKinds[schema.Binary]++
	case []any:
		if o.listElement == nil {
			o.listElement = newFieldObs("element")
		}
		for _, item := range val {
			o.listElement.observeValue(item)
		}
	case map[string]any:
		o.observeMap(val)
	case []string:
		if o.listElement == nil {
			o.listElement = newFieldObs("element")
		}
		for _, item := range val {
			o.listElement.observeValue(item)
		}
	case []json.Number:
		if o.listElement == nil {
			o.listElement = newFieldObs("element")
		}
		for _, item := range val {
			o.listElement.observeValue(item)
		}
	case [][]byte:
		if o.listElement == nil {
			o.listElement = newFieldObs("element")
		}
		for _, item := range val {
			o.listElement.observeValue(item)
		}
	}
}

func (o *fieldObs) observeMap(m map[string]any) {
	if len(m) == 0 {
		return
	}

	keys := make([]string, 0, len(m))
	for k, v := range m {
		keys = append(keys, k)
		child, ok := o.children[k]
		if !ok {
			child = newFieldObs(k)
			o.children[k] = child
		}
		child.observeValue(v)
	}

	sort.Strings(keys)
	keyset := strings.Join(keys, ",")
	if len(o.distinctKeysets) < 1000 {
		o.distinctKeysets[keyset]++
	}
}

// Finalize produces the final inferred schema.
func (i *Inferrer) Finalize() []schema.Column {
	cols := make([]schema.Column, 0, len(i.root.children))
	
	names := make([]string, 0, len(i.root.children))
	for n := range i.root.children {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		child := i.root.children[n]
		cols = append(cols, schema.Column{
			Name: n,
			Type: child.finalizeType(),
		})
	}
	return cols
}

func (o *fieldObs) finalizeType() schema.DataType {
	if o.listElement != nil {
		return schema.ArrayType{ElementType: o.listElement.finalizeType()}
	}

	if len(o.children) > 0 {
		fields := make([]schema.StructField, 0, len(o.children))
		names := make([]string, 0, len(o.children))
		for n := range o.children {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fields = append(fields, schema.StructField{
				Name: n,
				Type: o.children[n].finalizeType(),
			})
		}
		return schema.StructType{Fields: fields}
	}

	if len(o.observedKinds) == 0 {
		return schema.PrimitiveType{Kind: schema.String}
	}

	if o.observedKinds[schema.String] > 0 {
		return schema.PrimitiveType{Kind: schema.String}
	}
	if o.observedKinds[schema.Double] > 0 || o.observedKinds[schema.Float] > 0 {
		return schema.PrimitiveType{Kind: schema.Double}
	}
	if o.observedKinds[schema.BigInt] > 0 {
		return schema.PrimitiveType{Kind: schema.BigInt}
	}
	if o.observedKinds[schema.Boolean] > 0 {
		return schema.PrimitiveType{Kind: schema.Boolean}
	}
	if o.observedKinds[schema.Binary] > 0 {
		return schema.PrimitiveType{Kind: schema.Binary}
	}

	return schema.PrimitiveType{Kind: schema.String}
}
