package dynamo

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecord(t *testing.T) {
	line := `{
		"Item": {
			"s": {"S": "val"},
			"n": {"N": "123.45"},
			"b": {"BOOL": true},
			"null": {"NULL": true},
			"bin": {"B": "YmluYXJ5"},
			"ss": {"SS": ["a", "b"]},
			"ns": {"NS": ["1", "2"]},
			"list": {"L": [{"S": "inner"}]},
			"map": {"M": {"k": {"N": "42"}}}
		}
	}`

	got, err := ParseRecord([]byte(line))
	require.NoError(t, err)

	assert.Equal(t, "val", got["s"])
	assert.Equal(t, json.Number("123.45"), got["n"])
	assert.Equal(t, true, got["b"])
	assert.Nil(t, got["null"])
	assert.Equal(t, []byte("binary"), got["bin"])
	assert.Equal(t, []string{"a", "b"}, got["ss"])
	assert.Equal(t, []json.Number{"1", "2"}, got["ns"])
	assert.Equal(t, []any{"inner"}, got["list"])
	assert.Equal(t, map[string]any{"k": json.Number("42")}, got["map"])
}

func TestConvertAV_DeeplyNested(t *testing.T) {
	line := `{
		"Item": {
			"nested": {
				"M": {
					"l": {
						"L": [
							{"M": {"x": {"S": "y"}}}
						]
					}
				}
			}
		}
	}`
	got, err := ParseRecord([]byte(line))
	require.NoError(t, err)

	expected := map[string]any{
		"nested": map[string]any{
			"l": []any{
				map[string]any{"x": "y"},
			},
		},
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got %+v, want %+v", got, expected)
	}
}
