package adapter

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNativeModelProtocols(t *testing.T) {
	a := New()
	defer a.Close()
	for _, tc := range []struct {
		name, path, body, contains string
		status                     int
	}{
		{"gemini-text", "/v1beta/models/gemini-test:streamGenerateContent?alt=sse", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`, `"text":"Protocol verified"`, 200},
		{"gemini-tool", "/v1beta/models/gemini-test:streamGenerateContent?alt=sse", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"object"}}]}]}`, `"functionCall":{"args":{},"name":"lookup"}`, 200},
		{"gemini-wrong-body", "/v1beta/models/gemini-test:streamGenerateContent?alt=sse", `{"messages":[{"role":"user","content":"hello"}]}`, "", 400},
		{"gemini-missing-sse", "/v1beta/models/gemini-test:streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`, "", 400},
		{"claude-text", "/v1/messages", `{"model":"claude-test","max_tokens":100,"messages":[{"role":"user","content":"hello"}],"stream":true}`, "event: message_stop", 200},
		{"claude-tool", "/v1/messages", `{"model":"claude-test","max_tokens":100,"messages":[{"role":"user","content":"hello"}],"stream":true,"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`, `"type":"input_json_delta"`, 200},
		{"claude-missing-budget", "/v1/messages", `{"model":"claude-test","messages":[{"role":"user","content":"hello"}],"stream":true}`, "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, a.URL+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("anthropic-version", "2023-06-01")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != tc.status || !strings.Contains(string(body), tc.contains) {
				t.Fatalf("status=%d body=%s err=%v", resp.StatusCode, body, err)
			}
		})
	}
}
