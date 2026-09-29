package agentspace

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"measix/platform/internal/wire/adminapi"
)

const maxXMLBytes = 4 << 20
const maxEntries = 2000
const properties = `<?xml version="1.0"?><propfind xmlns="DAV:"><prop><resourcetype/><getcontentlength/><getlastmodified/><getetag/><quota-used-bytes/><quota-available-bytes/></prop></propfind>`

type davProperties struct {
	Type struct {
		Collection *struct{} `xml:"DAV: collection"`
	} `xml:"DAV: resourcetype"`
	Length    *int64 `xml:"DAV: getcontentlength"`
	Modified  string `xml:"DAV: getlastmodified"`
	Etag      string `xml:"DAV: getetag"`
	Used      *int64 `xml:"DAV: quota-used-bytes"`
	Available *int64 `xml:"DAV: quota-available-bytes"`
}
type davResponse struct {
	Href   string `xml:"DAV: href"`
	Status string `xml:"DAV: status"`
	Props  []struct {
		Properties davProperties `xml:"DAV: prop"`
		Status     string        `xml:"DAV: status"`
	} `xml:"DAV: propstat"`
}
type multistatus struct {
	XMLName   xml.Name      `xml:"DAV: multistatus"`
	Responses []davResponse `xml:"DAV: response"`
}

func statusCode(s string) int {
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(parts[1])
	return n
}
func readMulti(body io.Reader) (multistatus, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxXMLBytes+1))
	if err != nil {
		return multistatus{}, &Error{Code: "file_transport_unavailable"}
	}
	if len(data) > maxXMLBytes {
		return multistatus{}, &Error{Code: "file_listing_limit"}
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return multistatus{}, &Error{Code: "invalid_dav_xml"}
		}
		if _, ok := token.(xml.Directive); ok {
			return multistatus{}, &Error{Code: "invalid_dav_xml"}
		}
	}
	var out multistatus
	if xml.Unmarshal(data, &out) != nil || len(out.Responses) > maxEntries {
		return out, &Error{Code: "invalid_dav_xml"}
	}
	return out, nil
}
func (c *Client) href(name, raw string) (string, error) {
	rootRaw, err := c.DAVURL(name, "")
	if err != nil {
		return "", err
	}
	root, _ := url.Parse(rootRaw)
	relative, err := url.Parse(raw)
	if err != nil {
		return "", &Error{Code: "invalid_dav_href"}
	}
	resolved := root.ResolveReference(relative)
	if resolved.Scheme != root.Scheme || resolved.Host != root.Host || resolved.User != nil || resolved.RawQuery != "" || resolved.Fragment != "" || !strings.HasPrefix(resolved.Path, root.Path) {
		return "", &Error{Code: "invalid_dav_href"}
	}
	p := strings.TrimSuffix(strings.TrimPrefix(resolved.Path, root.Path), "/")
	return RelativePath(p, false)
}
func (c *Client) davRequest(ctx context.Context, name, token, p, method string, body io.Reader, headers http.Header) (*http.Response, error) {
	raw, err := c.DAVURL(name, p)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, body)
	if err != nil {
		return nil, &Error{Code: "invalid_file_request"}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for _, key := range []string{"Range", "If-Match", "If-None-Match", "Depth", "Destination", "Overwrite", "If", "Content-Type"} {
		if value := headers.Get(key); value != "" {
			req.Header.Set(key, value)
		}
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &Error{Code: "file_transport_unavailable", Unknown: method != "GET" && method != "HEAD" && method != "PROPFIND"}
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 || res.StatusCode == 304 {
		// Metadata and multi-status results consume a transfer slot too. A
		// response-header timeout alone cannot bound a stalled XML body.
		if method == "PROPFIND" || res.StatusCode == http.StatusMultiStatus {
			idle := c.DAVIdleTimeout
			if idle <= 0 {
				idle = 120 * time.Second
			}
			res.Body = IdleBody(res.Body, idle)
		}
		return res, nil
	}
	defer res.Body.Close()
	code := "file_service_unavailable"
	switch res.StatusCode {
	case 401, 403:
		code = "dav_credential_unavailable"
	case 404:
		code = "file_not_found"
	case 409:
		code = "file_conflict"
	case 412:
		code = "file_version_conflict"
	case 423:
		code = "file_locked"
	case 507:
		code = "file_storage_full"
	case 416:
		code = "file_range_invalid"
	}
	if res.StatusCode == 404 {
		if p == "" {
			code = "file_service_unavailable"
		} else {
			root, e := c.Stat(ctx, name, token, "")
			if e != nil || root.Kind != "DIRECTORY" {
				code = "file_service_unavailable"
			}
		}
	}
	return nil, &Error{Code: code, Status: res.StatusCode, Unknown: res.StatusCode >= 500 && method != "GET" && method != "HEAD" && method != "PROPFIND"}
}
func (c *Client) list(ctx context.Context, name, token, p, depth string) (adminapi.WorkspaceFileList, error) {
	out := adminapi.WorkspaceFileList{Entries: []adminapi.WorkspaceFileEntry{}}
	response, err := c.davRequest(ctx, name, token, p, "PROPFIND", strings.NewReader(properties), http.Header{"Depth": []string{depth}, "Content-Type": []string{"application/xml"}})
	if err != nil {
		return out, err
	}
	defer response.Body.Close()
	if response.StatusCode != 207 {
		return out, &Error{Code: "invalid_dav_response"}
	}
	multi, err := readMulti(response.Body)
	if err != nil {
		return out, err
	}
	for _, r := range multi.Responses {
		path, e := c.href(name, r.Href)
		if e != nil {
			return out, e
		}
		if path != p {
			prefix := ""
			if p != "" {
				prefix = p + "/"
			}
			if depth == "0" || !strings.HasPrefix(path, prefix) || strings.Contains(strings.TrimPrefix(path, prefix), "/") {
				return out, &Error{Code: "invalid_dav_href"}
			}
		}
		if r.Status != "" && (statusCode(r.Status) < 200 || statusCode(r.Status) >= 300) {
			return out, &Error{Code: "file_listing_partial"}
		}
		entry := adminapi.WorkspaceFileEntry{Path: path, Kind: "FILE"}
		ok := false
		for _, prop := range r.Props {
			code := statusCode(prop.Status)
			if code == 404 {
				continue
			}
			if code < 200 || code >= 300 {
				return out, &Error{Code: "file_listing_partial"}
			}
			ok = true
			value := prop.Properties
			if value.Type.Collection != nil {
				entry.Kind = "DIRECTORY"
			}
			if value.Length != nil {
				entry.Size = value.Length
			}
			if value.Etag != "" {
				// The pinned Agent Space DAV helper returns its opaque metadata
				// value without HTTP quotes in PROPFIND, but quotes it in HEAD/GET.
				etag := value.Etag
				if !strings.HasPrefix(etag, `"`) && !strings.ContainsAny(etag, "\"\r\n<>[]() \\/") {
					etag = `"` + etag + `"`
				}
				if validETag(etag) {
					entry.Etag = &etag
				}
			}
			if value.Modified != "" {
				if modified, e := http.ParseTime(value.Modified); e == nil {
					entry.ModifiedAt = &modified
				}
			}
			if path == p {
				out.UsedBytes = value.Used
				out.AvailableBytes = value.Available
			}
		}
		if !ok {
			return out, &Error{Code: "file_listing_partial"}
		}
		if depth == "0" || path != p {
			out.Entries = append(out.Entries, entry)
		}
	}
	return out, nil
}
func (c *Client) List(ctx context.Context, name, token, p string) (adminapi.WorkspaceFileList, error) {
	return c.list(ctx, name, token, p, "1")
}
func (c *Client) Stat(ctx context.Context, name, token, p string) (adminapi.WorkspaceFileEntry, error) {
	out, err := c.list(ctx, name, token, p, "0")
	if err != nil {
		return adminapi.WorkspaceFileEntry{}, err
	}
	if len(out.Entries) != 1 || out.Entries[0].Path != p {
		return adminapi.WorkspaceFileEntry{}, &Error{Code: "invalid_dav_response"}
	}
	return out.Entries[0], nil
}
func validETag(value string) bool {
	if len(value) < 2 || len(value) > 1024 || value[0] != '"' || value[len(value)-1] != '"' || strings.ContainsAny(value, "<>[]()") {
		return false
	}
	for _, c := range []byte(value[1 : len(value)-1]) {
		if c < 0x21 || c == '"' || c == 0x7f {
			return false
		}
	}
	return true
}
func (c *Client) Content(ctx context.Context, name, token, p, method string, body io.Reader, headers http.Header) (*http.Response, error) {
	if method != "GET" && method != "HEAD" && method != "PUT" {
		return nil, &Error{Code: "invalid_file_request"}
	}
	if _, err := RelativePath(p, true); err != nil {
		return nil, err
	}
	if headers.Get("If-Match") != "" && !validETag(headers.Get("If-Match")) {
		return nil, &Error{Code: "invalid_file_condition"}
	}
	if method == "PUT" {
		create := headers.Get("If-None-Match") == "*"
		replace := validETag(headers.Get("If-Match"))
		if create == replace {
			return nil, &Error{Code: "file_condition_required"}
		}
	}
	filtered := http.Header{}
	for _, key := range []string{"Range", "If-Match", "If-None-Match", "Content-Type"} {
		if value := headers.Get(key); value != "" {
			filtered.Set(key, value)
		}
	}
	return c.davRequest(ctx, name, token, p, method, body, filtered)
}
func (c *Client) Mutate(ctx context.Context, name, token string, input adminapi.WorkspaceFileMutation) (adminapi.WorkspaceFileResult, error) {
	out := adminapi.WorkspaceFileResult{Outcome: "SUCCEEDED", Failures: []adminapi.WorkspaceFileFailure{}, Truncated: false}
	if _, err := RelativePath(input.Path, true); err != nil {
		return out, err
	}
	method := string(input.Action)
	headers := http.Header{}
	if input.SourceEtag != nil {
		if !validETag(*input.SourceEtag) {
			return out, &Error{Code: "invalid_file_condition"}
		}
		headers.Set("If-Match", *input.SourceEtag)
	}
	switch method {
	case "MKCOL":
	case "DELETE":
		source, err := c.Stat(ctx, name, token, input.Path)
		if err != nil {
			return out, err
		}
		if source.Kind == "DIRECTORY" {
			if input.RecursiveConfirmed == nil || !*input.RecursiveConfirmed {
				return out, &Error{Code: "recursive_confirmation_required"}
			}
		} else if input.SourceEtag == nil {
			return out, &Error{Code: "file_condition_required"}
		}
	case "COPY", "MOVE":
		if input.Destination == nil {
			return out, &Error{Code: "invalid_file_destination"}
		}
		if _, err := RelativePath(*input.Destination, true); err != nil {
			return out, err
		}
		dest, err := c.DAVURL(name, *input.Destination)
		if err != nil {
			return out, err
		}
		source, e := c.Stat(ctx, name, token, input.Path)
		if e != nil {
			return out, e
		}
		if source.Kind == "FILE" && input.SourceEtag == nil {
			return out, &Error{Code: "file_condition_required"}
		}
		headers.Set("Destination", dest)
		headers.Set("Overwrite", "F")
		if input.Overwrite != nil && *input.Overwrite {
			if input.TargetEtag == nil || !validETag(*input.TargetEtag) {
				return out, &Error{Code: "file_condition_required"}
			}
			target, e := c.Stat(ctx, name, token, *input.Destination)
			if e != nil {
				return out, e
			}
			if source.Kind != "FILE" || target.Kind != "FILE" {
				return out, &Error{Code: "directory_overwrite_forbidden"}
			}
			headers.Set("Overwrite", "T")
			headers.Set("If", "<"+dest+"> (["+*input.TargetEtag+"])")
		}
	default:
		return out, &Error{Code: "invalid_file_request"}
	}
	response, err := c.davRequest(ctx, name, token, input.Path, method, nil, headers)
	if err != nil {
		return out, err
	}
	defer response.Body.Close()
	if response.StatusCode == 207 {
		multi, e := readMulti(response.Body)
		if e != nil {
			return out, &Error{Code: "invalid_dav_response", Unknown: true}
		}
		for _, r := range multi.Responses {
			path, e := c.href(name, r.Href)
			if e != nil {
				return out, &Error{Code: "invalid_dav_href", Unknown: true}
			}
			code := statusCode(r.Status)
			if r.Status == "" {
				code = 200
				for _, prop := range r.Props {
					if status := statusCode(prop.Status); status < 200 || status >= 300 {
						code = status
						break
					}
				}
				if len(r.Props) == 0 {
					code = 0
				}
			}
			if code < 200 || code >= 300 {
				out.Outcome = "PARTIAL"
				if len(out.Failures) < 100 {
					out.Failures = append(out.Failures, adminapi.WorkspaceFileFailure{Path: path, Status: code, Code: "remote_file_failure"})
				} else {
					out.Truncated = true
				}
			}
		}
	}
	return out, nil
}

// IdleBody closes a stalled stream; it never imposes a total size/time limit on
// a transfer that continues making progress.
func IdleBody(body io.ReadCloser, idle time.Duration) io.ReadCloser {
	return newProgressBody(body, idle)
}
