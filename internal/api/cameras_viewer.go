package api

import (
	_ "embed"
	"net/http"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

//go:embed websdk_viewer.html
var webSDKViewerHTML []byte

func (s *server) camerasWebSDKViewer(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	status := s.cameras.Status().WebSDK
	if !status.Available {
		writeAPIError(w, r, http.StatusServiceUnavailable, "camera_websdk_unavailable", "WebSDK V3.3.1 runtime is not installed", nil)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set(
		"Content-Security-Policy",
		"default-src 'self'; base-uri 'none'; frame-ancestors 'self'; object-src 'none'; "+
			"script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: blob: http://127.0.0.1:*; "+
			"connect-src 'self' ws://127.0.0.1:* wss://127.0.0.1:* http://127.0.0.1:* https://127.0.0.1:*",
	)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(webSDKViewerHTML)
}
