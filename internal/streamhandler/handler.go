package streamhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"

	"github.com/sokoloff/aws-datalake-tools/internal/dynamo"
)

// Handler processes Kinesis Firehose events containing DynamoDB stream records.
type Handler struct {
	log *slog.Logger
}

// New creates a new Handler.
func New(log *slog.Logger) *Handler {
	return &Handler{
		log: log,
	}
}

// Handle processes a batch of Firehose records.
func (h *Handler) Handle(ctx context.Context, event events.KinesisFirehoseEvent) (events.KinesisFirehoseResponse, error) {
	requestID := "unknown"
	if lc, ok := lambdacontext.FromContext(ctx); ok {
		requestID = lc.AwsRequestID
	}

	log := h.log.With(slog.String("request_id", requestID))

	log.Info("processing batch", slog.Int("record_count", len(event.Records)))

	var response events.KinesisFirehoseResponse
	var okCount, failedCount int
	startTime := time.Now()

	for _, r := range event.Records {
		data, err := h.transformRecord(r)
		if err != nil {
			log.Error("failed to transform record",
				slog.String("record_id", r.RecordID),
				slog.Any("error", err),
			)
			failedCount++
			response.Records = append(response.Records, events.KinesisFirehoseResponseRecord{
				RecordID: r.RecordID,
				Result:   events.KinesisFirehoseTransformedStateProcessingFailed,
				Data:     r.Data, // Echo original data on failure
			})
			continue
		}

		okCount++
		response.Records = append(response.Records, events.KinesisFirehoseResponseRecord{
			RecordID: r.RecordID,
			Result:   events.KinesisFirehoseTransformedStateOk,
			Data:     data,
		})
	}

	log.Info("batch complete",
		slog.Int("record_count", len(event.Records)),
		slog.Int("ok", okCount),
		slog.Int("failed", failedCount),
		slog.Int64("duration_ms", time.Since(startTime).Milliseconds()),
	)

	return response, nil
}

type ddbPayload struct {
	EventName string `json:"eventName"`
	Dynamodb  struct {
		NewImage                    json.RawMessage `json:"NewImage"`
		ApproximateCreationDateTime json.Number     `json:"ApproximateCreationDateTime"`
	} `json:"dynamodb"`
}

func (h *Handler) transformRecord(r events.KinesisFirehoseEventRecord) ([]byte, error) {
	var payload ddbPayload
	if err := json.Unmarshal(r.Data, &payload); err != nil {
		return nil, fmt.Errorf("unmarshaling payload: %w", err)
	}

	if len(payload.Dynamodb.NewImage) == 0 {
		return nil, fmt.Errorf("missing NewImage")
	}

	avMap, err := attributevalue.UnmarshalMapJSON(payload.Dynamodb.NewImage)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling NewImage as ddb json: %w", err)
	}

	record := dynamo.ConvertAttributeMap(avMap)
	record, err = dynamo.NormalizeKeys(record)
	if err != nil {
		return nil, fmt.Errorf("normalizing keys: %w", err)
	}

	record["eventname"] = strings.ToLower(payload.EventName)

	if payload.Dynamodb.ApproximateCreationDateTime != "" {
		tsStr := string(payload.Dynamodb.ApproximateCreationDateTime)
		var tsInt int64
		if strings.Contains(tsStr, ".") {
			f, err := payload.Dynamodb.ApproximateCreationDateTime.Float64()
			if err != nil {
				return nil, fmt.Errorf("parsing float ApproximateCreationDateTime %q: %w", tsStr, err)
			}
			tsInt = int64(f)
		} else {
			i, err := payload.Dynamodb.ApproximateCreationDateTime.Int64()
			if err != nil {
				return nil, fmt.Errorf("parsing int ApproximateCreationDateTime %q: %w", tsStr, err)
			}
			tsInt = i
		}

		// Detect precision: values >= 1e14 are assumed to be microseconds
		// 1e14 is circa 1973 in milliseconds, and circa 5138 in microseconds,
		// but typically microseconds are 1.6e15+
		var ts time.Time
		if tsInt >= 1e14 {
			// Microseconds
			ts = time.Unix(0, tsInt*1000)
		} else {
			// Milliseconds
			ts = time.Unix(0, tsInt*1e6)
		}

		record["eventcreationdatetime"] = ts.UTC().Format(time.RFC3339Nano)
	} else {
		record["eventcreationdatetime"] = ""
	}

	outData, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}

	outData = append(outData, '\n')
	return outData, nil
}
