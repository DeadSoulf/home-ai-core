package webui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const contentSecurityPolicy = "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'; form-action 'self'; img-src 'self' data:; font-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self' ws: wss:"

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

	setSecurityHeaders(w)

	cleanPath := filepath.Clean("/" + r.URL.Path)
	candidate := filepath.Join(h.webDir, filepath.FromSlash(strings.TrimPrefix(cleanPath, "/")))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, h.indexPath)
}

func (h *Handler) webAvailable() bool {
	info, err := os.Stat(h.indexPath)
	return err == nil && !info.IsDir()
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}
