package awsclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_DefaultConfig(t *testing.T) {
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	clients, err := New(context.Background(), Config{})
	require.NoError(t, err)
	assert.NotNil(t, clients.S3)
	assert.NotNil(t, clients.Glue)
	assert.NotNil(t, clients.DynamoDB)
}

func TestNew_WithRegion(t *testing.T) {
	clients, err := New(context.Background(), Config{Region: "eu-west-1"})
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", clients.AWSConfig.Region)
}
