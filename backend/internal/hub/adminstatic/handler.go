package adminstatic

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

type Handler struct{ root fs.FS }

func New(root fs.FS) *Handler { return &Handler{root: root} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/admin/")
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "index.html"
	}
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		http.NotFound(w, r)
		return
	}
	if data, err := fs.ReadFile(h.root, clean); err == nil {
		h.writeFile(w, clean, data)
		return
	}
	if strings.HasPrefix(clean, "assets/") || path.Ext(clean) != "" {
		http.NotFound(w, r)
		return
	}
	index, err := fs.ReadFile(h.root, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.writeFile(w, "index.html", index)
}

func (h *Handler) writeFile(w http.ResponseWriter, name string, data []byte) {
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
