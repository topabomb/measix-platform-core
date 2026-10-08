package adapter

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResponsesUsesInputAndNamedSSEEvents(t *testing.T) {
	a := New()
	defer a.Close()
	for _, tc := range []struct {
		name, body string
		status     int
		event      string
	}{
		{"text", `{"model":"test-model","input":[{"role":"user","content":"hello"}],"stream":true,"store":false}`, 200, "response.output_text.delta"},
		{"tool", `{"model":"test-model","input":[{"role":"user","content":"hello"}],"stream":true,"store":false,"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{}}}]}`, 200, "response.function_call_arguments.done"},
		{"chat-body-is-not-responses", `{"model":"test-model","messages":[{"role":"user","content":"hello"}],"stream":true,"store":false}`, 400, ""},
		{"server-state-not-in-profile", `{"model":"test-model","input":[{"role":"user","content":"hello"}],"stream":true,"store":false,"previous_response_id":"resp_old"}`, 400, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(a.URL+"/v1/responses", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.status {
				t.Fatalf("status=%d body=%s", resp.StatusCode, body)
			}
			if tc.status == 200 && (resp.Header.Get("Content-Type") != "text/event-stream" || !strings.Contains(string(body), "event: "+tc.event) || !strings.Contains(string(body), "event: response.completed")) {
				t.Fatalf("invalid Responses stream: %s", body)
			}
		})
	}
}
