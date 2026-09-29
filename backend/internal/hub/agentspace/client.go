// Package agentspace adapts the independently owned Agent Space v1 API.
// It never accesses remote state files, disks, or the VM backend.
package agentspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"measix/platform/pkg/platformid"
)

const MaxControlBytes = 1 << 20

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

type Error struct {
	Code    string
	Status  int
	Unknown bool
}

func (e *Error) Error() string  { return "agent space: " + e.Code }
func IsNotFound(err error) bool { var e *Error; return errors.As(err, &e) && e.Status == 404 }

type Client struct {
	admin, mcp, dav *url.URL
	credential      string
	HTTP            *http.Client
	DAVIdleTimeout  time.Duration
}

// User is an adapter-private representation of the external service response,
// not a Core API DTO. Remote optional observations remain optional.
type User struct {
	Username         string          `json:"username"`
	AgentSpaceID     string          `json:"agentSpaceId"`
	MCPURL           string          `json:"mcpUrl"`
	DAVURL           string          `json:"davUrl"`
	Status           string          `json:"status"`
	HasCredential    bool            `json:"hasCredential"`
	HasDAVCredential bool            `json:"hasDavCredential"`
	StopPending      bool            `json:"stopPending"`
	WorkspaceStatus  json.RawMessage `json:"workspaceStatus"`
}
type Credential struct {
	Username     string `json:"username"`
	AgentSpaceID string `json:"agentSpaceId"`
	MCPURL       string `json:"mcpUrl"`
	DAVURL       string `json:"davUrl"`
	Token        string `json:"token"`
	KeyID        string `json:"keyId"`
}

func origin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, &Error{Code: "invalid_origin"}
	}
	u.Path = ""
	return u, nil
}
func New(admin, mcp, dav, credential string) (*Client, error) {
	a, err := origin(admin)
	if err != nil {
		return nil, err
	}
	m, err := origin(mcp)
	if err != nil {
		return nil, err
	}
	var d *url.URL
	if dav != "" {
		d, err = origin(dav)
		if err != nil {
			return nil, err
		}
	}
	if credential == "" || strings.ContainsAny(credential, "\r\n") {
		return nil, &Error{Code: "invalid_credential"}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 90 * time.Second
	return &Client{admin: a, mcp: m, dav: d, credential: credential, HTTP: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func Username(userID string) (string, error) {
	if platformid.Validate(platformid.User, userID) != nil {
		return "", &Error{Code: "invalid_user_id"}
	}
	return "measix_" + strings.TrimPrefix(userID, "usr_"), nil
}
func validSpace(spc string) bool {
	return platformid.Validate(platformid.User, "usr_"+strings.TrimPrefix(spc, "spc_")) == nil && strings.HasPrefix(spc, "spc_")
}
func userPath(name string) (string, error) {
	if !usernamePattern.MatchString(name) {
		return "", &Error{Code: "invalid_username"}
	}
	return "/admin/v1/users/" + name, nil
}
func (c *Client) call(ctx context.Context, method, path string, body, out any) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.admin.String()+path, reader)
	if err != nil {
		return 0, &Error{Code: "invalid_request"}
	}
	req.Header.Set("Authorization", "Bearer "+c.credential)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	write := method != http.MethodGet && method != http.MethodHead
	if err != nil {
		return 0, &Error{Code: "transport_unavailable", Unknown: write}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		code := "remote_rejected"
		switch res.StatusCode {
		case 401, 403:
			code = "management_credential_unavailable"
		case 404:
			var problem struct {
				Error string `json:"error"`
			}
			if json.NewDecoder(io.LimitReader(res.Body, MaxControlBytes)).Decode(&problem) == nil && problem.Error == "not_found" {
				code = "remote_not_found"
			} else {
				return res.StatusCode, &Error{Code: "remote_route_unavailable", Unknown: write}
			}
		case 409:
			code = "remote_conflict"
		}
		return res.StatusCode, &Error{Code: code, Status: res.StatusCode, Unknown: write && res.StatusCode >= 500}
	}
	if out != nil {
		data, err := io.ReadAll(io.LimitReader(res.Body, MaxControlBytes+1))
		if err != nil || len(data) > MaxControlBytes || json.Unmarshal(data, out) != nil {
			return res.StatusCode, &Error{Code: "invalid_remote_response", Unknown: write}
		}
	}
	return res.StatusCode, nil
}
func (c *Client) Check(ctx context.Context) error {
	var out struct {
		Ready bool `json:"ready"`
	}
	_, err := c.call(ctx, "GET", "/admin/v1/status", nil, &out)
	if err == nil && !out.Ready {
		return &Error{Code: "remote_not_ready"}
	}
	return err
}
func (c *Client) Get(ctx context.Context, name string) (User, error) {
	p, err := userPath(name)
	if err != nil {
		return User{}, err
	}
	var u User
	_, err = c.call(ctx, "GET", p, nil, &u)
	if err == nil && (u.Username != name || !validSpace(u.AgentSpaceID) || !c.matches(c.mcp, u.MCPURL, "/u/"+name+"/mcp")) {
		err = &Error{Code: "remote_identity_mismatch"}
	}
	return u, err
}
func (c *Client) Create(ctx context.Context, name string) (Credential, error) {
	if _, err := userPath(name); err != nil {
		return Credential{}, err
	}
	var out Credential
	_, err := c.call(ctx, "POST", "/admin/v1/users", map[string]string{"username": name}, &out)
	if err == nil && (out.Username != name || !validSpace(out.AgentSpaceID) || out.Token == "" || !c.matches(c.mcp, out.MCPURL, "/u/"+name+"/mcp")) {
		err = &Error{Code: "remote_identity_mismatch", Unknown: true}
	}
	return out, err
}
func (c *Client) SetEnabled(ctx context.Context, name, spc string, enabled bool) (int, error) {
	p, err := userPath(name)
	if err != nil {
		return 0, err
	}
	if !validSpace(spc) {
		return 0, &Error{Code: "invalid_space"}
	}
	return c.call(ctx, "PATCH", p, map[string]any{"agentSpaceId": spc, "enabled": enabled}, nil)
}
func (c *Client) Delete(ctx context.Context, name, spc string) (int, error) {
	p, err := userPath(name)
	if err != nil {
		return 0, err
	}
	if !validSpace(spc) {
		return 0, &Error{Code: "invalid_space"}
	}
	return c.call(ctx, "DELETE", p+"?agentSpaceId="+url.QueryEscape(spc), nil, nil)
}
func (c *Client) RotateMCP(ctx context.Context, name, spc string) (Credential, error) {
	p, err := userPath(name)
	if err != nil {
		return Credential{}, err
	}
	if !validSpace(spc) {
		return Credential{}, &Error{Code: "invalid_space"}
	}
	var out Credential
	_, err = c.call(ctx, "POST", p+"/token", map[string]string{"agentSpaceId": spc}, &out)
	if err == nil && out.Token == "" {
		err = &Error{Code: "invalid_remote_response", Unknown: true}
	}
	return out, err
}
func (c *Client) SetDAV(ctx context.Context, name, spc, token string) (Credential, error) {
	p, err := userPath(name)
	if err != nil {
		return Credential{}, err
	}
	if !validSpace(spc) || len(token) < 32 || len(token) > 256 || c.dav == nil {
		return Credential{}, &Error{Code: "invalid_dav_configuration"}
	}
	var out Credential
	_, err = c.call(ctx, "POST", p+"/dav-token", map[string]string{"agentSpaceId": spc, "token": token}, &out)
	if err == nil && (out.Username != name || out.AgentSpaceID != spc || out.Token != token || !c.matches(c.dav, out.DAVURL, "/u/"+name+"/dav/")) {
		err = &Error{Code: "dav_confirmation_mismatch", Unknown: true}
	}
	return out, err
}
func (c *Client) RevokeDAV(ctx context.Context, name, spc string) error {
	p, err := userPath(name)
	if err != nil {
		return err
	}
	if !validSpace(spc) {
		return &Error{Code: "invalid_space"}
	}
	_, err = c.call(ctx, "DELETE", p+"/dav-token?agentSpaceId="+url.QueryEscape(spc), nil, nil)
	return err
}
func (c *Client) matches(base *url.URL, raw, path string) bool {
	if base == nil {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == base.Scheme && u.Host == base.Host && u.Path == path && u.RawQuery == "" && u.Fragment == "" && u.User == nil && u.RawPath == ""
}
func RelativePath(p string, mutation bool) (string, error) {
	if p == "" && !mutation {
		return "", nil
	}
	if len(p) > 4096 || p == "" || strings.ContainsAny(p, "\\\x00\r\n") || strings.HasPrefix(p, "/") {
		return "", &Error{Code: "invalid_file_path"}
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".agent-space") {
			return "", &Error{Code: "invalid_file_path"}
		}
		lower := strings.ToLower(part)
		for _, encoded := range []string{"%2e", "%2f", "%5c", "%25", "%00"} {
			if strings.Contains(lower, encoded) {
				return "", &Error{Code: "invalid_file_path"}
			}
		}
	}
	return p, nil
}
func (c *Client) DAVURL(name, path string) (string, error) {
	if c.dav == nil {
		return "", &Error{Code: "dav_unavailable"}
	}
	if _, err := userPath(name); err != nil {
		return "", err
	}
	p, err := RelativePath(path, false)
	if err != nil {
		return "", err
	}
	u := *c.dav
	u.Path = "/u/" + name + "/dav/" + p
	return u.String(), nil
}
func (e *Error) GoString() string {
	return fmt.Sprintf("agentspace.Error{Code:%q, Status:%d, Unknown:%t}", e.Code, e.Status, e.Unknown)
}
