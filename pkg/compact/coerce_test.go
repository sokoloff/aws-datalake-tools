package compact

import (
	"testing"

	"github.com/sokoloff/aws-datalake-tools/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestValidateCoercion_Incompatible(t *testing.T) {
	target := []schema.Column{{Name: "id", Type: schema.PrimitiveType{Kind: schema.BigInt}}}
	file := []schema.Column{{Name: "id", Type: schema.PrimitiveType{Kind: schema.String}}}
	
	summary, err := ValidateCoercion(target, file)
	assert.Error(t, err)
	assert.Contains(t, summary, "schema incompatible")
	assert.Contains(t, summary, "id")
}
