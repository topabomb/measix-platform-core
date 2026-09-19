// Package portalstatic serves the independent Portal artifact, with an isolated
// CSP and no dependency on Admin frontend assets or business handlers.
package portalstatic

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data: blob:; media-src blob:; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'; object-src 'none'"

func securityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
}

func New(root fs.FS) http.Handler {
	files := http.StripPrefix("/portal/", http.FileServer(http.FS(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		if r.URL.Path == "/portal" {
			http.Redirect(w, r, "/portal/", http.StatusPermanentRedirect)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/portal/")
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) {
			http.NotFound(w, r)
			return
		}
		info, err := fs.Stat(root, name)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// ParseUpstream validates the deployment-owned static Portal origin. Android never
// receives this URL: Core projects it under its own /portal/ origin.
func ParseUpstream(raw string) (*url.URL, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return nil, fmt.Errorf("invalid Portal upstream URL")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" ||
		u.RawPath != "" || strings.Contains(u.Path, "\\") {
		return nil, fmt.Errorf("invalid Portal upstream URL")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid Portal upstream URL")
		}
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, fmt.Errorf("invalid Portal upstream URL")
		}
	}
	if u.Path == "" {
		u.Path = "/"
	} else if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return u, nil
}

// NewRemote serves one deployment-selected static Portal through the Core origin.
// Incoming identity headers and upstream response authority never cross this boundary.
func NewRemote(raw string, client *http.Client) (http.Handler, error) {
	base, err := ParseUpstream(raw)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	remoteClient := *client
	remoteClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/portal" {
			http.Redirect(w, r, "/portal/", http.StatusPermanentRedirect)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/portal/")
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) {
			http.NotFound(w, r)
			return
		}
		target := *base
		target.Path = base.Path + name
		target.RawQuery = r.URL.RawQuery
		request, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), nil)
		if err != nil {
			http.Error(w, "Portal upstream request failed", http.StatusBadGateway)
			return
		}
		for _, header := range []string{"Accept", "If-None-Match", "If-Modified-Since"} {
			if value := r.Header.Get(header); value != "" {
				request.Header.Set(header, value)
			}
		}
		response, err := remoteClient.Do(request)
		if err != nil {
			http.Error(w, "Portal upstream unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			http.Error(w, "Portal upstream redirect rejected", http.StatusBadGateway)
			return
		}
		for _, header := range []string{"Content-Type", "Content-Language", "ETag", "Last-Modified"} {
			if value := response.Header.Get(header); value != "" {
				w.Header().Set(header, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, response.Body)
		}
	}), nil
}
