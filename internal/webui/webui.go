// Package webui serves the built Timetower admin UI.
package webui

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	immutableCache = "private, max-age=31536000, immutable"
	noCache        = "no-cache"
)

type handler struct {
	root  string
	files http.Handler
}

// NewHandler returns a handler for a static SvelteKit build. It serves real
// files from root and falls back to index.html for client-side routes.
func NewHandler(root string) (http.Handler, error) {
	index := filepath.Join(root, "index.html")
	info, err := os.Stat(index)
	if err != nil {
		return nil, fmt.Errorf("web UI index: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("web UI index is a directory: %s", index)
	}
	return &handler{root: root, files: http.FileServer(http.Dir(root))}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.serveFile(w, r) {
		return
	}
	if looksLikeAsset(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

func (h *handler) serveFile(w http.ResponseWriter, r *http.Request) bool {
	clean := path.Clean("/" + r.URL.Path)
	full := filepath.Join(h.root, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return false
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(clean, "/_app/immutable/") {
		w.Header().Set("Cache-Control", immutableCache)
	} else {
		w.Header().Set("Cache-Control", noCache)
	}
	h.files.ServeHTTP(w, r)
	return true
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", noCache)

	// Serve index.html through the file server without letting it see a
	// client-side route such as /status.
	indexRequest := r.Clone(r.Context())
	indexRequest.URL.Path = "/"
	indexRequest.URL.RawPath = ""
	h.files.ServeHTTP(w, indexRequest)
}

func looksLikeAsset(requestPath string) bool {
	return strings.HasPrefix(requestPath, "/_app/") || path.Ext(requestPath) != ""
}

// WithAPI routes /api/* to the API handler and everything else to the web UI.
// The caller is responsible for wrapping web with the desired auth middleware.
func WithAPI(api, web http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		web.ServeHTTP(w, r)
	})
}
