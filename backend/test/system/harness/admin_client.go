package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AdminClient is a thin HTTP client for the Admin API. It manages session
// cookies and CSRF tokens. It does not duplicate generated wire types.
type AdminClient struct {
	BaseURL    string
	HTTPClient *http.Client
	csrfToken  string
	cookies    []*http.Cookie
}

// NewAdminClient creates an admin client bound to the given Hub base URL.
func NewAdminClient(baseURL string) *AdminClient {
	return &AdminClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 30 * 1000 * 1000 * 1000},
	}
}

// Login performs admin login and stores the session cookie + CSRF token.
func (c *AdminClient) Login(ctx context.Context, username, password string) error {
	body := map[string]string{"username": username, "password": password}
	resp, err := c.doJSON(ctx, http.MethodPost, "/api/admin/v1/session/login", "", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login: status=%d", resp.StatusCode)
	}
	c.cookies = resp.Cookies()
	var result struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	c.csrfToken = result.CSRFToken
	return nil
}

// CSRFToken returns the current CSRF token.
func (c *AdminClient) CSRFToken() string { return c.csrfToken }

// CookieHeader returns the cookie header value for subsequent requests.
func (c *AdminClient) CookieHeader() string {
	var parts []string
	for _, cookie := range c.cookies {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

// Get performs a GET request and returns the raw response.
func (c *AdminClient) Get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", c.CookieHeader())
	return c.HTTPClient.Do(req)
}

// Post performs a POST request with a JSON body and returns the raw response.
func (c *AdminClient) Post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.doJSON(ctx, http.MethodPost, path, c.csrfToken, body)
}

// PostWithHeaders performs a POST request with a JSON body and extra headers
// (e.g. Idempotency-Key) and returns the raw response.
func (c *AdminClient) PostWithHeaders(ctx context.Context, path string, body interface{}, headers map[string]string) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", c.CookieHeader())
	req.Header.Set("X-CSRF-Token", c.csrfToken)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.HTTPClient.Do(req)
}

// Put performs a PUT request with a JSON body and returns the raw response.
func (c *AdminClient) Put(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.doJSON(ctx, http.MethodPut, path, c.csrfToken, body)
}

// Delete performs a DELETE request and returns the raw response.
func (c *AdminClient) Delete(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", c.CookieHeader())
	req.Header.Set("X-CSRF-Token", c.csrfToken)
	return c.HTTPClient.Do(req)
}

func (c *AdminClient) doJSON(ctx context.Context, method, path, csrf string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", c.CookieHeader())
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	return c.HTTPClient.Do(req)
}

// DecodeJSON decodes a response body into the target.
func DecodeJSON(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(target)
}

// ReadBody reads and returns the response body as a string.
func ReadBody(resp *http.Response) string {
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}
