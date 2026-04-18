package dynamo

import (
	"fmt"
	"strings"
)

// NormalizeKeys recursively lowercases all keys in a map.
// If two keys collide after lowercasing, it returns an error.
func NormalizeKeys(record map[string]any) (map[string]any, error) {
	return normalizeMap(record)
}

func normalizeMap(m map[string]any) (map[string]any, error) {
	res := make(map[string]any, len(m))
	for k, v := range m {
		lowerK := strings.ToLower(k)
		if _, ok := res[lowerK]; ok {
			return nil, fmt.Errorf("key collision after normalization: %s", lowerK)
		}

		normalizedV, err := normalizeValue(v)
		if err != nil {
			return nil, err
		}
		res[lowerK] = normalizedV
	}
	return res, nil
}

func normalizeValue(v any) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		return normalizeMap(val)
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			normalizedItem, err := normalizeValue(item)
			if err != nil {
				return nil, err
			}
			res[i] = normalizedItem
		}
		return res, nil
	default:
		return val, nil
	}
}
