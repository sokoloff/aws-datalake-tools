package shared

import (
	"context"
	"testing"

	"github.com/sokoloff/aws-datalake-tools/internal/awsclient"
)

// NewMotoClients creates AWS clients configured to talk to the local Moto server.
func NewMotoClients(ctx context.Context, t *testing.T, endpoint string) *awsclient.Clients {
	t.Helper()

	t.Setenv("AWS_ENDPOINT_URL", endpoint)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	clients, err := awsclient.New(ctx, awsclient.Config{
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("failed to create AWS clients: %v", err)
	}
	return clients
}
