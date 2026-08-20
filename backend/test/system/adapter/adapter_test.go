package adapter_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"measix/platform/test/system/adapter"
)

// CAP-C4 scenarios: the deterministic adapter must provide the four S0.1 required
// transport profiles plus MCP Streamable HTTP and observable failure injection.

func TestCAPC4001ChatRequestResponse(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	req := `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`
	resp := doJSON(t, a.URL, http.MethodPost, "/v1/chat/completions", req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body := readAll(t, resp)
	if !bytes.Contains(body, []byte(`"role":"assistant"`)) || !bytes.Contains(body, []byte(`"content":"hello"`)) {
		t.Fatalf("unexpected chat body: %s", body)
	}
	fact := a.LastRequest("/v1/chat/completions")
	if fact == nil || fact.Path != "/v1/chat/completions" || fact.Method != http.MethodPost {
		t.Fatalf("request not captured: %+v", fact)
	}
}

func TestCAPC4002ChatStreamingSSE(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	req := `{"model":"gpt-test","messages":[],"stream":true}`
	resp := doJSON(t, a.URL, http.MethodPost, "/v1/chat/completions", req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("not SSE content-type: %q", ct)
	}
	scanner := bufio.NewScanner(resp.Body)
	chunks := 0
	var joined strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			chunks++
			joined.WriteString(strings.TrimPrefix(line, "data:"))
		}
	}
	if chunks < 2 {
		t.Fatalf("expected multiple SSE chunks, got %d", chunks)
	}
	if !bytes.Contains([]byte(joined.String()), []byte("[DONE]")) {
		t.Fatalf("missing [DONE] sentinel: %s", joined.String())
	}
}

func TestCAPC4010TTSBinary(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	req := `{"model":"tts-test","input":"hello world","voice":"alloy"}`
	resp := doJSON(t, a.URL, http.MethodPost, "/v1/audio/speech", req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "audio/mpeg") {
		t.Fatalf("not audio/mpeg: %q", ct)
	}
	body := readAll(t, resp)
	if !bytes.Equal(body, a.Bytes()) {
		t.Fatalf("binary bytes corrupted: %d vs %d", len(body), len(a.Bytes()))
	}
	fact := a.LastRequest("/v1/audio/speech")
	if fact == nil {
		t.Fatalf("request not captured")
	}
	if fact.BodyJSON["input"] != "hello world" || fact.BodyJSON["voice"] != "alloy" {
		t.Fatalf("voice/input semantics not captured: %+v", fact)
	}
}

func TestCAPC4020ASRMultipart(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "whisper-test")
	part, err := writer.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("RIFF-test-bytes"))
	_ = writer.Close()
	req, _ := http.NewRequest(http.MethodPost, a.URL+"/v1/audio/transcriptions", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := readAll(t, resp)
	if !bytes.Contains(out, []byte(`"text":"transcribed"`)) {
		t.Fatalf("unexpected transcription: %s", out)
	}
	fact := a.LastRequest("/v1/audio/transcriptions")
	if fact == nil || fact.MultipartFields["model"] != "whisper-test" || !bytes.Contains(fact.MultipartFiles["file"], []byte("RIFF-test-bytes")) {
		t.Fatalf("multipart fields not captured: %+v", fact)
	}
}

func TestCAPC4030MCPStreamableHTTP(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	resp := doJSON(t, a.URL, http.MethodPost, "/mcp", req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body := readAll(t, resp)
	if !bytes.Contains(body, []byte(`"tools"`)) {
		t.Fatalf("unexpected MCP body: %s", body)
	}
	fact := a.LastRequest("/mcp")
	if fact == nil || fact.Path != "/mcp" {
		t.Fatalf("MCP request not captured")
	}
}

func TestCAPC404xx5xxTimeout(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	tests := []struct {
		path string
		want int
	}{
		{"/v1/chat/completions", http.StatusOK},
		{"/v1/errors/400", http.StatusBadRequest},
		{"/v1/errors/500", http.StatusInternalServerError},
		{"/v1/errors/429", http.StatusTooManyRequests},
	}
	for _, tc := range tests {
		resp := doJSON(t, a.URL, http.MethodPost, tc.path, `{}`)
		if resp.StatusCode != tc.want {
			t.Fatalf("%s: status=%d want=%d", tc.path, resp.StatusCode, tc.want)
		}
		resp.Body.Close()
	}
	a.SetTimeout(time.Second * 1)
	done := make(chan int, 1)
	go func() {
		resp := doJSON(t, a.URL, http.MethodPost, "/v1/chat/completions", `{"model":"gpt-test"}`)
		done <- resp.StatusCode
		resp.Body.Close()
	}()
	select {
	case status := <-done:
		if status != http.StatusOK {
			t.Fatalf("timeout path should still complete, got %d", status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("adapter timeout path did not respond")
	}
}

func TestCAPC4050CancellationObserved(t *testing.T) {
	a := adapter.New()
	defer a.Close()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.URL+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	if _, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if a.Cancelled() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("adapter did not observe client cancellation")
}

func doJSON(t *testing.T, baseURL, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func readAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
