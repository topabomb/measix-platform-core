package agentspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"measix/platform/internal/wire/adminapi"
)

const maxPreviewBytes = 1 << 20
const maxPreviewMetadataBytes = 16 << 10

// PreviewRequest keeps the source version in the query independent of HTTP
// preconditions on the derived representation. Range has no preview meaning.
type PreviewRequest struct {
	Method       string
	Path         string
	Size         int
	ExpectedETag string
	IfMatch      string
	IfNoneMatch  string
}

// PreviewResponse owns a bounded, validated GET body. HEAD and 304 validate only
// representation headers and return no data; they do not attest to body bytes.
// The HTTP owner can recheck its admission lease before delivering any bytes.
type PreviewResponse struct {
	Status int
	Header http.Header
	Data   []byte
}

func previewError(code string, status int) *Error { return &Error{Code: code, Status: status} }
func previewProtocolError() *Error                { return previewError("preview_protocol_error", 503) }
func previewUnavailable() *Error                  { return previewError("preview_unavailable", 503) }

func (c *Client) previewURL(name, token, expectedSpaceID string) (*url.URL, error) {
	if _, err := userPath(name); err != nil || !validSpace(expectedSpaceID) || token == "" || strings.ContainsAny(token, "\r\n") {
		return nil, previewError("invalid_preview_request", 400)
	}
	if c.dav == nil {
		return nil, previewUnavailable()
	}
	u := *c.dav
	u.Path = "/u/" + name + "/previews"
	return &u, nil
}

// Never follow redirects, including when tests or an embedding service supply
// their own HTTP client. The configured DAV origin alone chooses the target.
func (c *Client) previewDo(req *http.Request) (*http.Response, error) {
	client := *c.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return nil, previewUnavailable()
	}
	idle := c.DAVIdleTimeout
	if idle <= 0 {
		idle = 120 * time.Second
	}
	res.Body = IdleBody(res.Body, idle)
	return res, nil
}

func previewAuthError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return previewError("dav_credential_unavailable", 503)
	case http.StatusForbidden:
		return previewError("file_forbidden", 403)
	}
	return nil
}

func singlePreviewHeader(header http.Header, key string) (string, bool) {
	values := header.Values(key)
	return header.Get(key), len(values) == 1 && values[0] != ""
}

func previewMIME(header http.Header, expected string) bool {
	value, ok := singlePreviewHeader(header, "Content-Type")
	typeName, params, err := mime.ParseMediaType(value)
	if !ok || err != nil || typeName != expected {
		return false
	}
	if expected == "image/webp" {
		return len(params) == 0
	}
	for key, value := range params {
		if key != "charset" || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

// Reject duplicate JSON keys and trailing values while allowing future fields.
// Generated wire structs alone cannot distinguish required null/missing fields.
func previewObject(data []byte) (map[string]json.RawMessage, bool) {
	if !utf8.Valid(data) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, false
		}
		if _, exists := fields[key]; exists {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		fields[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, false
	}
	return fields, true
}

func previewField(fields map[string]json.RawMessage, key string, out any) bool {
	data, ok := fields[key]
	return ok && !bytes.Equal(bytes.TrimSpace(data), []byte("null")) && json.Unmarshal(data, out) == nil
}

func previewCapabilities(supported bool) adminapi.WorkspaceThumbnailCapabilities {
	out := adminapi.WorkspaceThumbnailCapabilities{SchemaVersion: 1, Supported: supported, ContentType: "image/webp", Sizes: []adminapi.WorkspaceThumbnailCapabilitiesSizes{}, MediaKinds: []adminapi.WorkspaceThumbnailCapabilitiesMediaKinds{}}
	if supported {
		out.Sizes = []adminapi.WorkspaceThumbnailCapabilitiesSizes{128, 256, 512}
		out.MediaKinds = []adminapi.WorkspaceThumbnailCapabilitiesMediaKinds{"IMAGE", "VIDEO"}
	}
	return out
}

// PreviewCapabilities is an authenticated, read-only control-plane observation.
// A 404/405 means unsupported only after the same credential reads its DAV root.
func (c *Client) PreviewCapabilities(ctx context.Context, name, token, expectedSpaceID string) (adminapi.WorkspaceThumbnailCapabilities, error) {
	var empty adminapi.WorkspaceThumbnailCapabilities
	u, err := c.previewURL(name, token, expectedSpaceID)
	if err != nil {
		return empty, err
	}
	req, _ := http.NewRequestWithContext(ctx, "OPTIONS", u.String(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	res, err := c.previewDo(req)
	if err != nil {
		return empty, err
	}
	defer res.Body.Close()
	if err = previewAuthError(res.StatusCode); err != nil {
		return empty, err
	}
	if res.StatusCode == 404 || res.StatusCode == 405 {
		res.Body.Close()
		if err = c.previewLegacyRoot(ctx, name, token); err != nil {
			return empty, err
		}
		return previewCapabilities(false), nil
	}
	if res.StatusCode != 200 {
		return empty, c.previewFailure(ctx, name, token, expectedSpaceID, "OPTIONS", res)
	}
	space, one := singlePreviewHeader(res.Header, "Agent-Space-Id")
	if !one || space != expectedSpaceID || !previewMIME(res.Header, "application/json") || res.Header.Get("Content-Encoding") != "" {
		return empty, previewProtocolError()
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxPreviewMetadataBytes+1))
	if err != nil {
		return empty, previewUnavailable()
	}
	if len(data) > maxPreviewMetadataBytes {
		return empty, previewProtocolError()
	}
	fields, ok := previewObject(data)
	var out adminapi.WorkspaceThumbnailCapabilities
	if !ok || !previewField(fields, "schemaVersion", &out.SchemaVersion) || out.SchemaVersion != 1 || !previewField(fields, "supported", &out.Supported) || !out.Supported || !previewField(fields, "contentType", &out.ContentType) || out.ContentType != "image/webp" || !previewField(fields, "sizes", &out.Sizes) || !previewField(fields, "mediaKinds", &out.MediaKinds) {
		return empty, previewProtocolError()
	}
	sizes := map[adminapi.WorkspaceThumbnailCapabilitiesSizes]bool{}
	for _, size := range out.Sizes {
		if sizes[size] || size != 128 && size != 256 && size != 512 {
			return empty, previewProtocolError()
		}
		sizes[size] = true
	}
	kinds := map[adminapi.WorkspaceThumbnailCapabilitiesMediaKinds]bool{}
	for _, kind := range out.MediaKinds {
		if kinds[kind] || kind != "IMAGE" && kind != "VIDEO" {
			return empty, previewProtocolError()
		}
		kinds[kind] = true
	}
	if len(sizes) != 3 || len(kinds) != 2 {
		return empty, previewProtocolError()
	}
	return previewCapabilities(true), nil
}

func (c *Client) previewLegacyRoot(ctx context.Context, name, token string) error {
	raw, err := c.DAVURL(name, "")
	if err != nil {
		return previewUnavailable()
	}
	req, _ := http.NewRequestWithContext(ctx, "PROPFIND", raw, strings.NewReader(properties))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml")
	res, err := c.previewDo(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err = previewAuthError(res.StatusCode); err != nil {
		return err
	}
	if res.StatusCode != 207 {
		return previewUnavailable()
	}
	multi, err := readMulti(res.Body)
	if err != nil || len(multi.Responses) != 1 {
		return previewUnavailable()
	}
	root := multi.Responses[0]
	path, err := c.href(name, root.Href)
	if err != nil || root.Href == "" || path != "" || root.Status != "" && (statusCode(root.Status) < 200 || statusCode(root.Status) >= 300) {
		return previewUnavailable()
	}
	directory := false
	for _, prop := range root.Props {
		status := statusCode(prop.Status)
		if status == 404 {
			continue
		}
		if status < 200 || status >= 300 {
			return previewUnavailable()
		}
		directory = directory || prop.Properties.Type.Collection != nil
	}
	if !directory {
		return previewUnavailable()
	}
	return nil
}

// ETag opaque bytes follow RFC 9110 section 8.8.3. Commas inside quotes are
// ordinary bytes; source expectedEtag remains exactly one strong entity tag.
func previewEntityTag(value string, weak bool) bool {
	if weak {
		value = strings.TrimPrefix(value, "W/")
	}
	if len(value) < 2 || len(value) > 1024 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	for _, ch := range []byte(value[1 : len(value)-1]) {
		if ch < 0x21 || ch == '"' || ch == 0x7f {
			return false
		}
	}
	return true
}

func previewConditionTags(value string) ([]string, bool) {
	if value == "" {
		return nil, true
	}
	if len(value) > 8192 {
		return nil, false
	}
	value = strings.Trim(value, " \t")
	if value == "*" {
		return []string{"*"}, true
	}
	var tags []string
	for value != "" {
		end := 0
		if strings.HasPrefix(value, "W/") {
			end = 2
		}
		if len(value) <= end || value[end] != '"' {
			return nil, false
		}
		last := strings.IndexByte(value[end+1:], '"')
		if last < 0 {
			return nil, false
		}
		last += end + 2
		if !previewEntityTag(value[:last], true) {
			return nil, false
		}
		tags = append(tags, value[:last])
		value = strings.TrimLeft(value[last:], " \t")
		if value == "" {
			return tags, true
		}
		if value[0] != ',' {
			return nil, false
		}
		value = strings.TrimLeft(value[1:], " \t")
		if value == "" {
			return nil, false
		}
	}
	return nil, false
}

func previewCondition(value string) bool {
	_, ok := previewConditionTags(value)
	return ok
}

func previewConditionMatches(value, etag string, weak bool) bool {
	tags, ok := previewConditionTags(value)
	if !ok {
		return false
	}
	for _, tag := range tags {
		if tag == "*" {
			return true
		}
		if weak {
			tag = strings.TrimPrefix(tag, "W/")
		}
		if tag == etag {
			return true
		}
	}
	return false
}

func (c *Client) Preview(ctx context.Context, name, token, expectedSpaceID string, request PreviewRequest) (*PreviewResponse, error) {
	if request.Size == 0 {
		request.Size = 256
	}
	_, pathErr := FilePath(request.Path, true)
	if request.Method != "GET" && request.Method != "HEAD" || pathErr != nil || request.Size != 128 && request.Size != 256 && request.Size != 512 || !previewEntityTag(request.ExpectedETag, false) || !previewCondition(request.IfMatch) || !previewCondition(request.IfNoneMatch) {
		return nil, previewError("invalid_preview_request", 400)
	}
	u, err := c.previewURL(name, token, expectedSpaceID)
	if err != nil {
		return nil, err
	}
	u.RawQuery = url.Values{"path": {request.Path}, "size": {strconv.Itoa(request.Size)}, "expectedEtag": {request.ExpectedETag}}.Encode()
	req, _ := http.NewRequestWithContext(ctx, request.Method, u.String(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "image/webp")
	req.Header.Set("Accept-Encoding", "identity")
	if request.IfMatch != "" {
		req.Header.Set("If-Match", request.IfMatch)
	}
	if request.IfNoneMatch != "" {
		req.Header.Set("If-None-Match", request.IfNoneMatch)
	}
	res, err := c.previewDo(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err = previewAuthError(res.StatusCode); err != nil {
		return nil, err
	}
	if res.StatusCode != 200 && res.StatusCode != 304 {
		return nil, c.previewFailure(ctx, name, token, expectedSpaceID, request.Method, res)
	}
	if res.StatusCode == 304 && request.IfNoneMatch == "" {
		return nil, previewProtocolError()
	}
	space, oneSpace := singlePreviewHeader(res.Header, "Agent-Space-Id")
	source, oneSource := singlePreviewHeader(res.Header, "Preview-Source-ETag")
	etag, oneETag := singlePreviewHeader(res.Header, "ETag")
	validMIME := previewMIME(res.Header, "image/webp")
	// HTTP intermediaries may omit representation content headers on 304.
	if res.StatusCode == 304 && len(res.Header.Values("Content-Type")) == 0 {
		validMIME = true
	}
	if !oneSpace || space != expectedSpaceID || !oneSource || source != request.ExpectedETag || !oneETag || !previewSHAETag(etag) || !validMIME || res.Header.Get("Content-Encoding") != "" {
		return nil, previewProtocolError()
	}
	if request.IfMatch != "" && !previewConditionMatches(request.IfMatch, etag, false) || previewConditionMatches(request.IfNoneMatch, etag, true) != (res.StatusCode == 304) {
		return nil, previewProtocolError()
	}
	header := http.Header{"Agent-Space-Id": {space}, "Preview-Source-Etag": {source}, "Etag": {etag}, "Content-Type": {"image/webp"}, "Content-Disposition": {`inline; filename="thumbnail.webp"`}, "Cache-Control": {"no-store"}, "X-Content-Type-Options": {"nosniff"}}
	if res.StatusCode == 304 {
		return &PreviewResponse{Status: 304, Header: header}, nil
	}
	length, oneLength := singlePreviewHeader(res.Header, "Content-Length")
	n, err := strconv.ParseInt(length, 10, 64)
	if !oneLength || err != nil || n < 1 || n > maxPreviewBytes || strconv.FormatInt(n, 10) != length || res.ContentLength != n {
		return nil, previewProtocolError()
	}
	header.Set("Content-Length", length)
	if request.Method == "HEAD" {
		return &PreviewResponse{Status: 200, Header: header}, nil
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxPreviewBytes+1))
	if err != nil || int64(len(data)) != n || !validPreviewWebP(data, request.Size) {
		return nil, previewProtocolError()
	}
	sum := sha256.Sum256(data)
	if etag != `"`+hex.EncodeToString(sum[:])+`"` {
		return nil, previewProtocolError()
	}
	return &PreviewResponse{Status: 200, Header: header, Data: data}, nil
}

func previewSHAETag(value string) bool {
	if len(value) != 66 || value[0] != '"' || value[65] != '"' {
		return false
	}
	for _, ch := range value[1:65] {
		if ch < '0' || ch > '9' && ch < 'a' || ch > 'f' {
			return false
		}
	}
	return true
}

var previewProblemCodes = map[int][]string{
	400: {"invalid_preview_request"}, 404: {"file_not_found"}, 412: {"file_version_conflict"}, 413: {"preview_source_limit"}, 415: {"preview_unsupported_media"}, 422: {"preview_decode_failed"}, 429: {"preview_busy"}, 503: {"preview_unavailable", "preview_dependency_unavailable", "preview_timeout"},
}

func (c *Client) previewFailure(ctx context.Context, name, token, space, method string, res *http.Response) error {
	if method == "HEAD" && res.StatusCode != 404 && res.StatusCode != 405 {
		if codes := previewProblemCodes[res.StatusCode]; len(codes) > 0 {
			err := previewError(codes[0], res.StatusCode)
			if res.StatusCode == 429 {
				err.RetryAfter = "1"
			}
			return err
		}
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			return previewProtocolError()
		}
		return previewUnavailable()
	}
	if method != "HEAD" && previewMIME(res.Header, "application/problem+json") {
		data, err := io.ReadAll(io.LimitReader(res.Body, maxPreviewMetadataBytes+1))
		if err != nil || len(data) > maxPreviewMetadataBytes {
			return previewProtocolError()
		}
		fields, ok := previewObject(data)
		var kind, title, code string
		var status int
		if !ok || !previewField(fields, "type", &kind) || kind != "about:blank" || !previewField(fields, "title", &title) || title == "" || !previewField(fields, "status", &status) || status != res.StatusCode || !previewField(fields, "code", &code) {
			return previewProtocolError()
		}
		for _, allowed := range previewProblemCodes[status] {
			if code == allowed {
				err := previewError(code, status)
				if status == 429 {
					err.RetryAfter = "1"
				}
				return err
			}
		}
		if status != 404 && status != 405 {
			return previewUnavailable()
		}
	}
	if res.StatusCode == 404 || res.StatusCode == 405 {
		res.Body.Close()
		capabilities, err := c.PreviewCapabilities(ctx, name, token, space)
		if err != nil {
			return err
		}
		if !capabilities.Supported {
			return previewError("preview_not_supported", 501)
		}
		if res.StatusCode == 404 {
			return previewError("file_not_found", 404)
		}
		return previewProtocolError()
	}
	if res.StatusCode >= 300 && res.StatusCode < 400 || res.StatusCode < 500 {
		return previewProtocolError()
	}
	return previewUnavailable()
}
