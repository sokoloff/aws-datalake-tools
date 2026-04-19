package shared

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartMoto starts a Moto container for testing AWS services locally.
func StartMoto(ctx context.Context, t *testing.T) (testcontainers.Container, string, error) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	req := testcontainers.ContainerRequest{
		Image:        "motoserver/moto:5.1.22",
		ExposedPorts: []string{"5000/tcp"},
		WaitingFor: wait.ForHTTP("/moto-api/").
			WithPort("5000/tcp").
			WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
			WithStartupTimeout(2 * time.Minute),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}

	ip, err := container.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	mappedPort, err := container.MappedPort(ctx, "5000")
	if err != nil {
		return nil, "", err
	}

	endpoint := fmt.Sprintf("http://%s:%s", ip, mappedPort.Port())
	return container, endpoint, nil
}

// ResetMoto resets the Moto server state via its API.
func ResetMoto(t *testing.T, endpoint string) {
	t.Helper()
	res, err := http.Post(fmt.Sprintf("%s/moto-api/reset", endpoint), "application/json", nil)
	if err != nil {
		t.Logf("Warning: failed to reset Moto: %v", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Logf("Warning: Moto reset returned status %d", res.StatusCode)
	}
}

// StartRIE starts an AWS Lambda Runtime Interface Emulator (RIE) container.
func StartRIE(ctx context.Context, bootstrapPath string) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "public.ecr.aws/lambda/provided:al2023",
		ExposedPorts: []string{"8080/tcp"},
		Cmd:          []string{"bootstrap"},
		Env: map[string]string{
			"LOG_LEVEL": "DEBUG",
		},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      bootstrapPath,
				ContainerFilePath: "/var/runtime/bootstrap",
				FileMode:          0755,
			},
		},
		WaitingFor: wait.ForListeningPort("8080/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}

	ip, err := container.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	mappedPort, err := container.MappedPort(ctx, "8080")
	if err != nil {
		return nil, "", err
	}

	invokeURL := fmt.Sprintf("http://%s:%s/2015-03-31/functions/function/invocations", ip, mappedPort.Port())
	return container, invokeURL, nil
}
