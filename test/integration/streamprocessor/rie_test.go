//go:build integration

package streamprocessor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"

	"github.com/sokoloff/aws-datalake-tools/test/integration/shared"
)

var (
	rieInvokeURL string
	rieContainer interface {
		Logs(ctx context.Context) (io.ReadCloser, error)
	}
)

func TestMain(m *testing.M) {
	// Build bootstrap
	wd, _ := os.Getwd()
	bootstrapPath := wd + "/bootstrap"
	cmd := exec.Command("go", "build", "-o", bootstrapPath, "../../../cmd/stream-processor")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("failed to build bootstrap: " + string(out))
	}

	ctx := context.Background()
	container, invokeURL, err := shared.StartRIE(ctx, bootstrapPath)
	if err != nil {
		panic("failed to start RIE: " + err.Error())
	}

	type logger interface {
		Logs(ctx context.Context) (io.ReadCloser, error)
	}
	if l, ok := container.(logger); ok {
		rieContainer = l
	}

	rieInvokeURL = invokeURL

	code := m.Run()

	container.Terminate(ctx)
	os.Remove(bootstrapPath)
	os.Exit(code)
}

func invokeRIE(t *testing.T, req events.KinesisFirehoseEvent) events.KinesisFirehoseResponse {
	t.Helper()

	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(rieInvokeURL, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		var logOut string
		if rieContainer != nil {
			logs, _ := rieContainer.Logs(context.Background())
			if logs != nil {
				buf := new(bytes.Buffer)
				io.Copy(buf, logs)
				logOut = buf.String()
				logs.Close()
			}
		}
		t.Fatalf("expected 200, got %d, body: %s\nLogs: %s", resp.StatusCode, string(body), logOut)
	}

	var firehoseResp events.KinesisFirehoseResponse
	err = json.NewDecoder(resp.Body).Decode(&firehoseResp)
	require.NoError(t, err)

	return firehoseResp
}

func TestStreamProcessor_RIE_HappyPath(t *testing.T) {
	// Build a Firehose event with one DDB-Kinesis-stream record
	ddbData := `{
		"eventName": "INSERT",
		"dynamodb": {
			"ApproximateCreationDateTime": 1618732800000,
			"NewImage": {
				"Id": {"S": "123"},
				"Count": {"N": "42"},
				"Active": {"BOOL": true},
				"Items": {"L": [{"S": "a"}]},
				"Meta": {"M": {"Key": {"S": "val"}}}
			}
		}
	}`

	req := events.KinesisFirehoseEvent{
		Records: []events.KinesisFirehoseEventRecord{
			{
				RecordID: "rec-1",
				Data:     []byte(ddbData),
			},
		},
	}

	resp := invokeRIE(t, req)

	require.Len(t, resp.Records, 1)
	require.Equal(t, "rec-1", resp.Records[0].RecordID)
	require.Equal(t, events.KinesisFirehoseTransformedStateOk, resp.Records[0].Result)

	// decode base64 data
	var decoded map[string]any
	err := json.Unmarshal(resp.Records[0].Data, &decoded)
	require.NoError(t, err)

	require.Equal(t, "123", decoded["id"])
	require.Equal(t, float64(42), decoded["count"])
	require.Equal(t, true, decoded["active"])
	require.Equal(t, "insert", decoded["eventname"])
	require.Contains(t, decoded["eventcreationdatetime"], "2021-04-18T")
}

func TestStreamProcessor_RIE_MixedBatch(t *testing.T) {
	goodData := `{
		"eventName": "MODIFY",
		"dynamodb": {
			"ApproximateCreationDateTime": 1618732800000,
			"NewImage": {
				"Id": {"S": "123"}
			}
		}
	}`
	malformedJSON := `{ bad json `
	missingNewImage := `{
		"eventName": "REMOVE",
		"dynamodb": {
			"ApproximateCreationDateTime": 1618732800000
		}
	}`

	req := events.KinesisFirehoseEvent{
		Records: []events.KinesisFirehoseEventRecord{
			{RecordID: "rec-good", Data: []byte(goodData)},
			{RecordID: "rec-bad", Data: []byte(malformedJSON)},
			{RecordID: "rec-missing", Data: []byte(missingNewImage)},
		},
	}

	resp := invokeRIE(t, req)

	require.Len(t, resp.Records, 3)

	require.Equal(t, "rec-good", resp.Records[0].RecordID)
	require.Equal(t, events.KinesisFirehoseTransformedStateOk, resp.Records[0].Result)

	require.Equal(t, "rec-bad", resp.Records[1].RecordID)
	require.Equal(t, events.KinesisFirehoseTransformedStateProcessingFailed, resp.Records[1].Result)
	require.Equal(t, []byte(malformedJSON), resp.Records[1].Data)

	require.Equal(t, "rec-missing", resp.Records[2].RecordID)
	require.Equal(t, events.KinesisFirehoseTransformedStateProcessingFailed, resp.Records[2].Result)
	require.Equal(t, []byte(missingNewImage), resp.Records[2].Data)
}

func TestStreamProcessor_RIE_EmptyBatch(t *testing.T) {
	req := events.KinesisFirehoseEvent{
		Records: []events.KinesisFirehoseEventRecord{},
	}

	resp := invokeRIE(t, req)

	require.Empty(t, resp.Records)
}

func TestStreamProcessor_RIE_LogsStructured(t *testing.T) {
	if rieContainer == nil {
		t.Skip("logs not supported by container instance")
	}

	// Send an empty batch to generate logs
	req := events.KinesisFirehoseEvent{
		Records: []events.KinesisFirehoseEventRecord{},
	}
	invokeRIE(t, req)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logs, err := rieContainer.Logs(ctx)
	require.NoError(t, err)
	defer logs.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, logs)
	require.NoError(t, err)

	output := buf.String()

	// Assert output contains structured json with our fields
	require.Contains(t, output, `"request_id"`)
	require.Contains(t, output, `"record_count":0`)
	require.Contains(t, output, `"level":"INFO"`)
}
