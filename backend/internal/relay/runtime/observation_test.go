package runtime

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"testing"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/relay/protocolusage"
	"measix/platform/internal/wire/relaycontrolapi"
)

func TestUsageObservationDecodesGzipJSONCopy(t *testing.T) {
	body := gzipTestBody(t, []byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	observation := modelUsageObservation()
	response := &http.Response{Header: http.Header{
		"Content-Type":     []string{"application/json; charset=utf-8"},
		"Content-Encoding": []string{"gzip"},
	}}
	observation.configureResponse(response)
	for offset := 0; offset < len(body); offset += 7 {
		end := min(offset+7, len(body))
		observation.observeResponse(body[offset:end])
	}

	result := observation.finish(true)
	assertExactMeter(t, result, protocolusage.InputTokens, 11)
	assertExactMeter(t, result, protocolusage.OutputTokens, 7)
	assertExactMeter(t, result, protocolusage.TotalTokens, 18)
}

func TestUsageObservationDecodesGzipSSECopy(t *testing.T) {
	body := gzipTestBody(t, []byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n\ndata: [DONE]\n\n"))
	observation := modelUsageObservation()
	response := &http.Response{Header: http.Header{
		"Content-Type":     []string{"text/event-stream"},
		"Content-Encoding": []string{"gzip"},
	}}
	observation.configureResponse(response)
	observation.observeResponse(body)

	result := observation.finish(true)
	assertExactMeter(t, result, protocolusage.InputTokens, 5)
	assertExactMeter(t, result, protocolusage.OutputTokens, 3)
	assertExactMeter(t, result, protocolusage.TotalTokens, 8)
}

func modelUsageObservation() *usageObservation {
	return &usageObservation{resource: control.Resource{
		Kind:           relaycontrolapi.ResourceRouteResourceKind("MODEL"),
		ClientProtocol: relaycontrolapi.ResourceRouteClientProtocol("OPENAI_CHAT_COMPLETIONS"),
	}}
}

func gzipTestBody(t *testing.T, body []byte) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func assertExactMeter(t *testing.T, result protocolusage.Result, meter protocolusage.Meter, want int64) {
	t.Helper()
	for _, measurement := range result.Measurements {
		if measurement.Meter != meter {
			continue
		}
		if measurement.Completeness != protocolusage.Exact || measurement.Value == nil ||
			measurement.Value.Numerator != want || measurement.Value.Denominator != 1 {
			t.Fatalf("%s = %+v, want exact %d", meter, measurement, want)
		}
		return
	}
	t.Fatalf("missing %s measurement: %+v", meter, result.Measurements)
}
