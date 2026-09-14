// Package portalstatic serves the independent Portal artifact, with an isolated
// CSP and no dependency on Admin frontend assets or business handlers.
package portalstatic

import (
	"io/fs"
	"net/http"
	"strings"
)

func New(root fs.FS) http.Handler {
	files := http.StripPrefix("/portal/", http.FileServer(http.FS(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data: blob:; media-src blob:; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'; object-src 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
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
