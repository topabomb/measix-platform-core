package capability

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"measix/platform/internal/wire/adminapi"
)

var (
	ErrMcpDiscoveryProtocol    = errors.New("MCP returned an invalid or incomplete tool catalog")
	ErrMcpDiscoveryUnavailable = errors.New("MCP discovery could not connect or authenticate; check the applied connection")
	ErrMcpDiscoveryLimit       = errors.New("MCP discovery exceeded the page, tool or response limit")
	ErrMcpDiscoveryTimeout     = errors.New("MCP discovery timed out; check the service and retry")
	ErrMcpSourceUnavailable    = errors.New("MCP discovery needs an applied source and, for a workspace, a connected user")
	ErrMcpSourceChanged        = errors.New("MCP source changed during discovery; save and discover again")
	ErrMcpToolEvidence         = errors.New("MCP approval must match server-owned discovery evidence")
)

func McpToolContractHash(def adminapi.McpToolDefinition) (string, error) {
	if !validMcpTool(def) {
		return "", ErrMcpDiscoveryProtocol
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return "", ErrMcpDiscoveryProtocol
	}
	canonical, err := jsoncanonicalizer.Transform(raw)
	if err != nil {
		return "", ErrMcpDiscoveryProtocol
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
func validMcpTool(def adminapi.McpToolDefinition) bool {
	name, ok := def["name"].(string)
	if !ok || strings.TrimSpace(name) == "" || strings.Contains(name, "*") {
		return false
	}
	schema, ok := def["inputSchema"].(map[string]any)
	if !ok || schema["type"] != "object" {
		return false
	}
	for _, field := range []string{"description", "title"} {
		if v, present := def[field]; present {
			if _, ok := v.(string); !ok {
				return false
			}
		}
	}
	for _, field := range []string{"outputSchema", "annotations", "_meta"} {
		if v, present := def[field]; present {
			m, ok := v.(map[string]any)
			if !ok {
				return false
			}
			if field == "outputSchema" && m["type"] != "object" {
				return false
			}
		}
	}
	return true
}

type mcpDiscoveryClient struct {
	ctx              context.Context
	endpoint         string
	headers          http.Header
	client           *http.Client
	session, version string
	consumed         int
	id               int
}

// DiscoverMcpCatalog executes only the standard discovery handshake and tools/list.
// Transport errors deliberately contain no endpoint, credentials or remote body.
func DiscoverMcpCatalog(ctx context.Context, endpoint string, headers http.Header) ([]adminapi.McpDiscoveredTool, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 10 * time.Second
	defer transport.CloseIdleConnections()
	d := &mcpDiscoveryClient{ctx: ctx, endpoint: endpoint, headers: headers.Clone(), client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	defer d.close()
	result, err := d.rpc("initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "MEASIX Admin discovery", "version": "1"}}, false)
	if err != nil {
		return nil, err
	}
	var init struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
	}
	if json.Unmarshal(result, &init) != nil {
		return nil, ErrMcpDiscoveryProtocol
	}
	switch init.ProtocolVersion {
	case "2025-11-25", "2025-06-18", "2025-03-26":
	default:
		return nil, ErrMcpDiscoveryProtocol
	}
	if tools, ok := init.Capabilities["tools"]; !ok || len(tools) == 0 || tools[0] != '{' {
		return nil, ErrMcpDiscoveryProtocol
	}
	d.version = init.ProtocolVersion
	if _, err = d.rpc("notifications/initialized", nil, true); err != nil {
		return nil, err
	}
	out := []adminapi.McpDiscoveredTool{}
	names := map[string]bool{}
	cursors := map[string]bool{}
	cursor := ""
	for page := 0; page < 64; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := d.rpc("tools/list", params, false)
		if err != nil {
			return nil, err
		}
		// Canonicalization also rejects duplicate JSON keys before map decoding.
		if _, err = jsoncanonicalizer.Transform(raw); err != nil {
			return nil, ErrMcpDiscoveryProtocol
		}
		var list struct {
			Tools      *[]adminapi.McpToolDefinition `json:"tools"`
			NextCursor *string                       `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &list) != nil || list.Tools == nil {
			return nil, ErrMcpDiscoveryProtocol
		}
		for _, def := range *list.Tools {
			hash, err := McpToolContractHash(def)
			if err != nil {
				return nil, err
			}
			name := def["name"].(string)
			if names[name] {
				return nil, ErrMcpDiscoveryProtocol
			}
			names[name] = true
			out = append(out, adminapi.McpDiscoveredTool{Name: name, Definition: def, ContractHash: hash})
			if len(out) > 4096 {
				return nil, ErrMcpDiscoveryLimit
			}
		}
		if list.NextCursor == nil {
			sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
			return out, nil
		}
		cursor = *list.NextCursor
		if cursor == "" || cursors[cursor] {
			return nil, ErrMcpDiscoveryProtocol
		}
		cursors[cursor] = true
	}
	return nil, ErrMcpDiscoveryLimit
}

func (d *mcpDiscoveryClient) request(ctx context.Context, method string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, d.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, ErrMcpSourceUnavailable
	}
	req.Header = d.headers.Clone()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if d.session != "" {
		req.Header.Set("Mcp-Session-Id", d.session)
	}
	if d.version != "" {
		req.Header.Set("MCP-Protocol-Version", d.version)
	}
	response, err := d.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrMcpDiscoveryTimeout
		}
		return nil, ErrMcpDiscoveryUnavailable
	}
	return response, nil
}
func (d *mcpDiscoveryClient) rpc(method string, params any, notification bool) (json.RawMessage, error) {
	d.id++
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	if !notification {
		msg["id"] = d.id
	}
	body, _ := json.Marshal(msg)
	response, err := d.request(d.ctx, "POST", body)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrMcpDiscoveryUnavailable
	}
	if method == "initialize" {
		d.session = response.Header.Get("Mcp-Session-Id")
		for _, c := range d.session {
			if c < 0x21 || c > 0x7e {
				return nil, ErrMcpDiscoveryProtocol
			}
		}
	}
	if notification {
		if response.StatusCode != 202 {
			return nil, ErrMcpDiscoveryProtocol
		}
		return nil, nil
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return nil, ErrMcpDiscoveryProtocol
	}
	budget := MaxSnapshotBytes - d.consumed
	if budget <= 0 {
		return nil, ErrMcpDiscoveryLimit
	}
	reader := &io.LimitedReader{R: response.Body, N: int64(budget) + 1}
	decode := func(raw []byte) (json.RawMessage, bool, error) {
		if _, err := jsoncanonicalizer.Transform(raw); err != nil {
			return nil, false, ErrMcpDiscoveryProtocol
		}
		var msg struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Result  json.RawMessage `json:"result"`
			Error   json.RawMessage `json:"error"`
			Method  string          `json:"method"`
		}
		if json.Unmarshal(raw, &msg) != nil || msg.JSONRPC != "2.0" {
			return nil, false, ErrMcpDiscoveryProtocol
		}
		if len(msg.ID) == 0 && msg.Method != "" {
			return nil, false, nil
		}
		if string(msg.ID) != string(mustJSON(d.id)) || len(msg.Error) > 0 || len(msg.Result) == 0 {
			return nil, false, ErrMcpDiscoveryProtocol
		}
		return msg.Result, true, nil
	}
	var result json.RawMessage
	if media == "application/json" {
		raw, err := io.ReadAll(reader)
		if err != nil {
			if d.ctx.Err() != nil {
				return nil, ErrMcpDiscoveryTimeout
			}
			return nil, ErrMcpDiscoveryUnavailable
		}
		if len(raw) > budget {
			return nil, ErrMcpDiscoveryLimit
		}
		value, ok, err := decode(raw)
		if err != nil || !ok {
			return nil, ErrMcpDiscoveryProtocol
		}
		result = value
	} else if media == "text/event-stream" {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), budget+1)
		data := []string{}
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(data) == 0 {
					continue
				}
				raw := []byte(strings.Join(data, "\n"))
				data = nil
				if len(bytes.TrimSpace(raw)) == 0 {
					continue
				}
				value, ok, err := decode(raw)
				if err != nil {
					return nil, err
				}
				if ok {
					result = value
					break
				}
			} else if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		if result == nil {
			if reader.N == 0 {
				return nil, ErrMcpDiscoveryLimit
			}
			if d.ctx.Err() != nil {
				return nil, ErrMcpDiscoveryTimeout
			}
			return nil, ErrMcpDiscoveryProtocol
		}
	} else {
		return nil, ErrMcpDiscoveryProtocol
	}
	consumed := budget + 1 - int(reader.N)
	if consumed > budget {
		return nil, ErrMcpDiscoveryLimit
	}
	d.consumed += consumed
	return result, nil
}
func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
func (d *mcpDiscoveryClient) close() {
	if d.session == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(d.ctx), 2*time.Second)
	defer cancel()
	if response, err := d.request(ctx, "DELETE", nil); err == nil {
		response.Body.Close()
	}
}
