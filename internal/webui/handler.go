package webui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Handler struct {
	api        http.Handler
	webDir     string
	indexPath  string
	fileServer http.Handler
}

func New(api http.Handler, webDir string) http.Handler {
	webDir = filepath.Clean(webDir)
	return &Handler{
		api:        api,
		webDir:     webDir,
		indexPath:  filepath.Join(webDir, "index.html"),
		fileServer: http.FileServer(http.Dir(webDir)),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/api/") {
		h.api.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.api.ServeHTTP(w, r)
		return
	}
	if !h.webAvailable() {
		h.api.ServeHTTP(w, r)
		return
	}

	cleanPath := filepath.Clean("/" + r.URL.Path)
	candidate := filepath.Join(h.webDir, filepath.FromSlash(strings.TrimPrefix(cleanPath, "/")))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		h.fileServer.ServeHTTP(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, h.indexPath)
}

func (h *Handler) webAvailable() bool {
	info, err := os.Stat(h.indexPath)
	return err == nil && !info.IsDir()
}
