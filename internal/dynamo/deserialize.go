package dynamo

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ConvertAttributeMap converts a map of DynamoDB AttributeValues to a standard Go map[string]any.
func ConvertAttributeMap(avMap map[string]types.AttributeValue) map[string]any {
	result := make(map[string]any, len(avMap))
	for k, av := range avMap {
		result[k] = convertAV(av)
	}
	return result
}

// ParseRecord parses a DynamoDB JSON line into a map of standard Go types.
func ParseRecord(line []byte) (map[string]any, error) {
	// Step 1: Parse the top-level "Item" wrapper
	var wrapper struct {
		Item json.RawMessage `json:"Item"`
	}
	if err := json.Unmarshal(line, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshaling top-level: %w", err)
	}

	// Step 2: Parse "Item" as DynamoDB JSON into map[string]types.AttributeValue
	avMap, err := attributevalue.UnmarshalMapJSON(wrapper.Item)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling item as ddb json: %w", err)
	}

	// Step 3: Convert types.AttributeValue to map[string]any recursively
	return ConvertAttributeMap(avMap), nil
}

func convertAV(av types.AttributeValue) any {
	if av == nil {
		return nil
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return json.Number(v.Value)
	case *types.AttributeValueMemberBOOL:
		return v.Value
	case *types.AttributeValueMemberNULL:
		return nil
	case *types.AttributeValueMemberB:
		return v.Value
	case *types.AttributeValueMemberSS:
		return v.Value
	case *types.AttributeValueMemberNS:
		nums := make([]json.Number, len(v.Value))
		for i, n := range v.Value {
			nums[i] = json.Number(n)
		}
		return nums
	case *types.AttributeValueMemberBS:
		return v.Value
	case *types.AttributeValueMemberL:
		list := make([]any, len(v.Value))
		for i, item := range v.Value {
			list[i] = convertAV(item)
		}
		return list
	case *types.AttributeValueMemberM:
		m := make(map[string]any, len(v.Value))
		for k, val := range v.Value {
			m[k] = convertAV(val)
		}
		return m
	default:
		return nil
	}
}
