package dynamo

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeKeys(t *testing.T) {
	record := map[string]any{
		"ID":   "123",
		"Name": "Test",
		"Meta": map[string]any{
			"CreatedAt": "2024-01-01",
			"Tags":      []any{map[string]any{"Key": "k1"}},
		},
	}

	got, err := NormalizeKeys(record)
	require.NoError(t, err)

	expected := map[string]any{
		"id":   "123",
		"name": "Test",
		"meta": map[string]any{
			"createdat": "2024-01-01",
			"tags":      []any{map[string]any{"key": "k1"}},
		},
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got %+v, want %+v", got, expected)
	}
}

func TestNormalizeKeys_Collision(t *testing.T) {
	record := map[string]any{
		"ID": "1",
		"id": "2",
	}
	_, err := NormalizeKeys(record)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key collision")
}
