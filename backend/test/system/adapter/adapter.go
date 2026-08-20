// Package adapter provides the deterministic upstream Test Adapter used by the
// S0.1 system harness. It is a real HTTP service (not a mock) that deterministically
// provides the four required transport profiles plus failure/cancel injection and
// safe request capture. It is intentionally not a production component.
package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// RequestFact records transport facts that are safe to assert on. Secrets,
// credentials and plain prompt bodies are never persisted.
type RequestFact struct {
	Path             string
	Method           string
	BodyJSON         map[string]interface{}
	MultipartFields  map[string]string
	MultipartFiles   map[string][]byte
	ContentType      string
	XMeasixRequestId string
	Headers          map[string]string
}

// Adapter is a deterministic upstream Test Adapter.
type Adapter struct {
	URL      string
	ttsBytes []byte

	mu            sync.Mutex
	facts         []*RequestFact
	cancelled     bool
	timeout       time.Duration
	server        *httptest.Server
	injectHeaders map[string]string
	injectStatus  int
}

// New starts a deterministic adapter on a random loopback port.
func New() *Adapter {
	a := &Adapter{
		ttsBytes: []byte{0x49, 0x44, 0x33, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}
	a.server = httptest.NewServer(http.HandlerFunc(a.serve))
	a.URL = a.server.URL
	return a
}

// Close shuts down the adapter server.
func (a *Adapter) Close() { a.server.Close() }

// Bytes returns the deterministic TTS binary payload.
func (a *Adapter) Bytes() []byte { return a.ttsBytes }

// InjectHeaders sets headers that the adapter will include in its next
// chat completion response. This is used to test Relay's header-stripping
// behavior by having the upstream actually return Set-Cookie/Location.
// Pass nil and status 0 to clear injection.
func (a *Adapter) InjectHeaders(headers map[string]string, status int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.injectHeaders = headers
	a.injectStatus = status
}

// LastRequest returns the most recent captured fact for a path, or nil.
func (a *Adapter) LastRequest(path string) *RequestFact {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := len(a.facts) - 1; i >= 0; i-- {
		if a.facts[i].Path == path {
			return a.facts[i]
		}
	}
	return nil
}

// ClearFacts removes all captured request facts. Useful for asserting that
// subsequent denied requests are not forwarded to the upstream.
func (a *Adapter) ClearFacts() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.facts = nil
}

// AllFacts returns a snapshot of all captured request facts.
func (a *Adapter) AllFacts() []*RequestFact {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]*RequestFact, len(a.facts))
	copy(out, a.facts)
	return out
}

// Cancelled reports whether the adapter has observed any client cancellation.
func (a *Adapter) Cancelled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cancelled
}

// SetTimeout makes every subsequent request sleep before responding.
func (a *Adapter) SetTimeout(d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.timeout = d
}

func (a *Adapter) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if a.timeout > 0 {
		time.Sleep(a.timeout)
	}
	a.mu.Unlock()

	fact := a.capture(r)
	a.record(fact)

	go func() {
		<-r.Context().Done()
		a.mu.Lock()
		a.cancelled = true
		a.mu.Unlock()
	}()

	switch {
	case r.URL.Path == "/v1/chat/completions":
		a.handleChat(w, r, fact)
	case r.URL.Path == "/v1/audio/speech":
		a.handleSpeech(w, r)
	case r.URL.Path == "/v1/audio/transcriptions":
		a.handleTranscriptions(w, r)
	case r.URL.Path == "/mcp":
		a.handleMCP(w, r)
	case strings.HasPrefix(r.URL.Path, "/v1/errors/"):
		code := 400
		_, _ = fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/v1/errors/"), "%d", &code)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":{"code":%d}}`, code)))
	default:
		http.NotFound(w, r)
	}
}

func (a *Adapter) capture(r *http.Request) *RequestFact {
	fact := &RequestFact{
		Path:             r.URL.Path,
		Method:           r.Method,
		MultipartFields:  map[string]string{},
		MultipartFiles:   map[string][]byte{},
		ContentType:      r.Header.Get("Content-Type"),
		XMeasixRequestId: r.Header.Get("X-Measix-Request-Id"),
		Headers:          safeHeaders(r.Header),
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(8 << 20); err == nil {
			for k, v := range r.MultipartForm.Value {
				if len(v) > 0 {
					fact.MultipartFields[k] = v[0]
				}
			}
			for k, files := range r.MultipartForm.File {
				if len(files) == 0 {
					continue
				}
				file, err := files[0].Open()
				if err != nil {
					continue
				}
				data, err := io.ReadAll(file)
				_ = file.Close()
				if err == nil {
					fact.MultipartFiles[k] = data
				}
			}
		}
		return fact
	}
	if r.Method == http.MethodPost && r.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = r.Body.Close()
		var m map[string]interface{}
		if json.Unmarshal(body, &m) == nil {
			fact.BodyJSON = m
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}
	return fact
}

// safeHeaders records only non-sensitive header keys (lowercased) so the tests
// can assert on sanitization without persisting credentials. Authorization and
// cookies are excluded.
func safeHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		lower := strings.ToLower(k)
		if lower == "authorization" || lower == "cookie" || lower == "set-cookie" {
			continue
		}
		if len(v) > 0 {
			out[lower] = v[0]
		}
	}
	return out
}

func (a *Adapter) record(fact *RequestFact) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.facts = append(a.facts, fact)
}

func (a *Adapter) handleChat(w http.ResponseWriter, r *http.Request, fact *RequestFact) {
	// Check if header injection is active (for SEC-020 strip tests).
	a.mu.Lock()
	injectHeaders := a.injectHeaders
	injectStatus := a.injectStatus
	// Clear injection after first use so subsequent requests are normal.
	a.injectHeaders = nil
	a.injectStatus = 0
	a.mu.Unlock()

	if injectHeaders != nil {
		for k, v := range injectHeaders {
			w.Header().Set(k, v)
		}
		status := injectStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"id":"1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
		return
	}

	streaming := false
	if fact != nil && fact.BodyJSON != nil {
		if v, ok := fact.BodyJSON["stream"].(bool); ok {
			streaming = v
		}
	}
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"hel"}}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","choices":[{"delta":{"content":"lo"}}]}`,
			`data: [DONE]`,
		}
		for _, c := range chunks {
			_, _ = io.WriteString(w, c+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"id":"1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
}

func (a *Adapter) handleSpeech(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "audio/mpeg")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(a.ttsBytes)
}

func (a *Adapter) handleTranscriptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"text":"transcribed"}`)
}

func (a *Adapter) handleMCP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"tool-a"}]}}`)
}
