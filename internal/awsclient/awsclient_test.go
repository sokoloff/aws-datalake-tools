package awsclient

import (
	"context"
	"os"
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

func TestNew_WithProfile(t *testing.T) {
	// Create a dummy config file with the profile
	tmpDir := t.TempDir()
	configFile := tmpDir + "/config"
	os.WriteFile(configFile, []byte("[profile dummy]\nregion = us-east-1"), 0644)
	t.Setenv("AWS_CONFIG_FILE", configFile)
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")

	clients, err := New(context.Background(), Config{Profile: "dummy"})
	require.NoError(t, err)
	assert.NotNil(t, clients)
}

func TestNew_Error(t *testing.T) {
	// Force error by providing an invalid config file path and requiring it
	t.Setenv("AWS_CONFIG_FILE", "/non/existent/path/to/config")
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
	// Note: config.LoadDefaultConfig might not fail just because a file is missing,
	// it usually just ignores missing files unless we use custom loaders.
	// However, if we set an invalid AWS_REGION it might fail in some SDK versions?
	// Actually, let's just test that the error branch CAN be reached.
	// One way is to set an invalid duration for some timeout env vars.
	t.Setenv("AWS_MAX_ATTEMPTS", "not-a-number")
	
	_, err := New(context.Background(), Config{})
	assert.Error(t, err)
}
