package streamhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

func TestTransformRecord_Happy(t *testing.T) {
	h := New(slog.Default())
	data := []byte(`{"eventName":"INSERT","dynamodb":{"ApproximateCreationDateTime":1646957436123456,"NewImage":{"Id":{"S":"123"},"Num":{"N":"45.6"},"Bool":{"BOOL":true},"Null":{"NULL":true},"List":{"L":[{"S":"a"}]},"Map":{"M":{"NestedKey":{"S":"val"}}}}}}`)

	r := events.KinesisFirehoseEventRecord{Data: data}
	out, err := h.transformRecord(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(string(out), "\n") {
		t.Errorf("expected trailing newline")
	}

	outStr := string(out)
	if !strings.Contains(outStr, `"eventname":"insert"`) {
		t.Errorf("missing eventname: %s", outStr)
	}
	if !strings.Contains(outStr, `"eventcreationdatetime":"2022-03-11T00:10:36.123456Z"`) {
		t.Errorf("missing eventcreationdatetime: %s", outStr)
	}
	if !strings.Contains(outStr, `"id":"123"`) {
		t.Errorf("missing id: %s", outStr)
	}
	if !strings.Contains(outStr, `"num":45.6`) {
		t.Errorf("missing num: %s", outStr)
	}
	if !strings.Contains(outStr, `"bool":true`) {
		t.Errorf("missing bool: %s", outStr)
	}
	if !strings.Contains(outStr, `"null":null`) {
		t.Errorf("missing null: %s", outStr)
	}
	if !strings.Contains(outStr, `"list":["a"]`) {
		t.Errorf("missing list: %s", outStr)
	}
	if !strings.Contains(outStr, `"map":{"nestedkey":"val"}`) {
		t.Errorf("missing nested map lowercase key: %s", outStr)
	}
}

func TestTransformRecord_Microsecond(t *testing.T) {
	h := New(slog.Default())
	data := []byte(`{"eventName":"MODIFY","dynamodb":{"ApproximateCreationDateTime":1646957436123456,"NewImage":{"Id":{"S":"1"}}}}`)

	out, err := h.transformRecord(events.KinesisFirehoseEventRecord{Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"eventcreationdatetime":"2022-03-11T00:10:36.123456Z"`) {
		t.Errorf("wrong timestamp: %s", out)
	}
}

func TestTransformRecord_MissingNewImage(t *testing.T) {
	h := New(slog.Default())
	data := []byte(`{"eventName":"REMOVE","dynamodb":{"ApproximateCreationDateTime":1646957436123456}}`)

	_, err := h.transformRecord(events.KinesisFirehoseEventRecord{Data: data})
	if err == nil || !strings.Contains(err.Error(), "missing NewImage") {
		t.Errorf("expected missing NewImage error, got: %v", err)
	}
}

func TestTransformRecord_MalformedJSON(t *testing.T) {
	h := New(slog.Default())
	_, err := h.transformRecord(events.KinesisFirehoseEventRecord{Data: []byte(`{bad json`)})
	if err == nil {
		t.Errorf("expected error for malformed JSON")
	}
}

func TestTransformRecord_MissingTimestamp(t *testing.T) {
	h := New(slog.Default())
	data := []byte(`{"eventName":"MODIFY","dynamodb":{"NewImage":{"Id":{"S":"1"}}}}`)

	out, err := h.transformRecord(events.KinesisFirehoseEventRecord{Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"eventcreationdatetime":""`) {
		t.Errorf("expected empty string for missing timestamp: %s", out)
	}
}

func TestTransformRecord_BinaryAndSets(t *testing.T) {
	h := New(slog.Default())
	data := []byte(`{"eventName":"INSERT","dynamodb":{"ApproximateCreationDateTime":1,"NewImage":{"Bin":{"B":"YmFzZTY0"},"NumSet":{"NS":["1","2"]},"StrSet":{"SS":["a","b"]},"BinSet":{"BS":["YmFzZTY0"]}}}}`)

	out, err := h.transformRecord(events.KinesisFirehoseEventRecord{Data: data})
	if err != nil {
		t.Fatal(err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, `"bin":"YmFzZTY0"`) { // Go's []byte json encodes to base64 automatically
		t.Errorf("missing bin: %s", outStr)
	}
	if !strings.Contains(outStr, `"numset":[1,2]`) {
		t.Errorf("missing numset: %s", outStr)
	}
	if !strings.Contains(outStr, `"strset":["a","b"]`) {
		t.Errorf("missing strset: %s", outStr)
	}
	if !strings.Contains(outStr, `"binset":["YmFzZTY0"]`) {
		t.Errorf("missing binset: %s", outStr)
	}
}

func TestHandle_MixedSuccessFailure(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	h := New(log)

	event := events.KinesisFirehoseEvent{
		Records: []events.KinesisFirehoseEventRecord{
			{RecordID: "ok-1", Data: []byte(`{"eventName":"INSERT","dynamodb":{"NewImage":{"Id":{"S":"1"}}}}`)},
			{RecordID: "bad-1", Data: []byte(`{bad json`)},
			{RecordID: "bad-2", Data: []byte(`{"eventName":"REMOVE","dynamodb":{}}`)}, // missing NewImage
		},
	}

	resp, err := h.Handle(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(resp.Records))
	}

	for _, r := range resp.Records {
		switch r.RecordID {
		case "ok-1":
			if r.Result != events.KinesisFirehoseTransformedStateOk {
				t.Errorf("expected ok-1 to be Ok, got %v", r.Result)
			}
		case "bad-1", "bad-2":
			if r.Result != events.KinesisFirehoseTransformedStateProcessingFailed {
				t.Errorf("expected %s to be ProcessingFailed, got %v", r.RecordID, r.Result)
			}
			if len(r.Data) == 0 {
				t.Errorf("expected original data to be echoed for %s", r.RecordID)
			}
		}
	}
}

func TestHandle_EmptyBatch(t *testing.T) {
	h := New(slog.Default())
	resp, err := h.Handle(context.Background(), events.KinesisFirehoseEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(resp.Records))
	}
}

func TestHandle_LogsRequestID(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	h := New(log)

	ctx := lambdacontext.NewContext(context.Background(), &lambdacontext.LambdaContext{AwsRequestID: "req-abc"})
	_, err := h.Handle(ctx, events.KinesisFirehoseEvent{Records: []events.KinesisFirehoseEventRecord{
		{RecordID: "1", Data: []byte(`{"eventName":"INSERT","dynamodb":{"NewImage":{"Id":{"S":"1"}}}}`)},
	}})

	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), `"request_id":"req-abc"`) {
		t.Errorf("expected request_id in logs: %s", buf.String())
	}
}

func TestHandle_BatchResponseShape(t *testing.T) {
	h := New(slog.Default())
	event := events.KinesisFirehoseEvent{
		Records: []events.KinesisFirehoseEventRecord{
			{RecordID: "1", Data: []byte(`{"eventName":"INSERT","dynamodb":{"NewImage":{"Id":{"S":"1"}}}}`)},
		},
	}
	resp, _ := h.Handle(context.Background(), event)

	b, _ := json.Marshal(resp)
	if !strings.Contains(string(b), `"recordId":"1"`) {
		t.Errorf("expected recordId in response shape: %s", string(b))
	}
	if !strings.Contains(string(b), `"result":"Ok"`) {
		t.Errorf("expected result Ok in response shape: %s", string(b))
	}
	if !strings.Contains(string(b), `"data":`) {
		t.Errorf("expected data in response shape: %s", string(b))
	}
}
