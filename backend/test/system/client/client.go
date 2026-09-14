// Package client implements the S0.1 Test Client. It only uses the client-facing
// runtime topology (/runtime/v1/resources/{resourceId}{runtimePath}) and the
// generation/interaction correlation headers from the Control Protocol. It never
// needs upstreamId, runtimeRouteId, base URL or Secret.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

// Options configures the Test Client.
type Options struct {
	RuntimeBaseURL    string
	AccessToken       string
	ManagedGeneration int
	InteractionID     string
	// SpoofHeaders are extra headers sent by the client. They exist to exercise
	// the Relay's inbound sanitization (a malicious/compromised client attempting
	// to forge internal X-Measix-* or forwarding headers); the Relay must strip
	// them and never forward them to the upstream.
	SpoofHeaders map[string]string
	HTTPClient   *http.Client
}

// Client is a client-facing Test Client for the S0.1 runtime contract.
type Client struct {
	base        string
	token       string
	generation  int
	interaction string
	spoof       map[string]string
	http        *http.Client
}

// New builds a Test Client bound to a runtime base URL and an access token.
func New(opts Options) *Client {
	c := opts.HTTPClient
	if c == nil {
		c = http.DefaultClient
	}
	return &Client{
		base:        strings.TrimRight(opts.RuntimeBaseURL, "/"),
		token:       opts.AccessToken,
		generation:  opts.ManagedGeneration,
		interaction: opts.InteractionID,
		spoof:       opts.SpoofHeaders,
		http:        c,
	}
}

// ProblemError is a non-2xx runtime response returned by the Relay.
type ProblemError struct {
	Status    int
	Code      string
	Title     string
	Forwarded *bool
}

func (p ProblemError) Error() string {
	return fmt.Sprintf("runtime problem: status=%d code=%s title=%q", p.Status, p.Code, p.Title)
}

// ChatCompletion issues a request/response chat completion call.
func (c *Client) ChatCompletion(ctx context.Context, resourceID, runtimePath, body string) ([]byte, string, error) {
	return c.roundTrip(ctx, resourceID, runtimePath, []byte(body), "application/json")
}

// ChatCompletionStream issues a streaming chat completion call and delivers each
// SSE "data:" payload to onData. The terminal [DONE] sentinel is also delivered.
func (c *Client) ChatCompletionStream(ctx context.Context, resourceID, runtimePath, body string, onData func([]byte)) error {
	req, err := c.newRequest(ctx, resourceID, runtimePath, strings.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return decodeProblem(resp)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		onData([]byte(payload))
	}
	return scanner.Err()
}

// Speech issues a TTS speech request and returns the binary body.
func (c *Client) Speech(ctx context.Context, resourceID, runtimePath, body string) ([]byte, string, error) {
	return c.roundTrip(ctx, resourceID, runtimePath, []byte(body), "application/json")
}

// Transcription issues a multipart transcription request.
func (c *Client) Transcription(ctx context.Context, resourceID, runtimePath, model, filename string, file []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("model", model)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(file); err != nil {
		return nil, "", err
	}
	_ = writer.Close()
	return c.roundTrip(ctx, resourceID, runtimePath, buf.Bytes(), writer.FormDataContentType())
}

// MCP issues an MCP Streamable HTTP request.
func (c *Client) MCP(ctx context.Context, resourceID, runtimePath, body string) ([]byte, string, error) {
	return c.roundTrip(ctx, resourceID, runtimePath, []byte(body), "application/json")
}

func (c *Client) roundTrip(ctx context.Context, resourceID, runtimePath string, body []byte, contentType string) ([]byte, string, error) {
	req, err := c.newRequest(ctx, resourceID, runtimePath, bytes.NewReader(body), contentType)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, resp.Header.Get("Content-Type"), decodeProblem(resp)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header.Get("Content-Type"), err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (c *Client) newRequest(ctx context.Context, resourceID, runtimePath string, body io.Reader, contentType string) (*http.Request, error) {
	u := c.base + "/runtime/v1/resources/" + url.PathEscape(resourceID) + runtimePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-Measix-Managed-Generation", fmt.Sprintf("%d", c.generation))
	if c.interaction != "" {
		req.Header.Set("X-Measix-Interaction-Id", c.interaction)
	}
	for k, v := range c.spoof {
		req.Header.Set(k, v)
	}
	return req, nil
}

func decodeProblem(resp *http.Response) error {
	var p struct {
		Title     string `json:"title"`
		Code      string `json:"code"`
		Forwarded *bool  `json:"forwarded"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&p)
	return ProblemError{Status: resp.StatusCode, Code: p.Code, Title: p.Title, Forwarded: p.Forwarded}
}
