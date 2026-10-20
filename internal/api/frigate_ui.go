package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

// Handler preserves Core's HTTP API and exposes the official module UI on a
// DIFFERENT origin. Never mount Frigate's JavaScript in Core's origin: it must
// not be able to read Core localStorage, CSRF tokens, or its parent document.
type Handler struct {
	http.Handler
	server *server
}

type FrigateUIConfig struct {
	ListenAddress     string
	CoreListenAddress string
	InsecureLoopback  bool
	Context           context.Context
}
type frigateUIState struct {
	mu             sync.RWMutex
	port, corePort string
	ready          bool
	problem        string
	context        context.Context
	recheck        time.Duration
}

// ConfigureFrigateUI is called once before either listener starts accepting.
// The same server, permissions, Job Engine lock and endpoint resolver are used.
func (h *Handler) ConfigureFrigateUI(cfg FrigateUIConfig) http.Handler {
	_, port, _ := net.SplitHostPort(cfg.ListenAddress)
	_, corePort, _ := net.SplitHostPort(cfg.CoreListenAddress)
	ctx := cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	h.server.frigateUI = &frigateUIState{port: port, corePort: corePort, context: ctx, recheck: 15 * time.Second}
	return http.HandlerFunc(h.server.serveFrigateUI)
}
func (h *Handler) SetFrigateUIReady(ready bool, problem string) {
	state := h.server.frigateUI
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.ready = ready
	state.problem = problem
}

// FrigateFrameOrigin is also consumed by Core's web handler when constructing
// its precise frame-src/Permissions-Policy. No arbitrary https: framing grant.
func (h *Handler) FrigateFrameOrigin(r *http.Request) string {
	state := h.server.frigateUI
	if state == nil || state.port == "" {
		return ""
	}
	origin, err := frigateOrigin(r, state.port)
	if err != nil {
		return ""
	}
	return origin
}
func frigateOrigin(r *http.Request, port string) (string, error) {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("invalid UI port")
	}
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	base, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || base.Host != r.Host || base.User != nil || base.Path != "" || base.RawQuery != "" || base.Fragment != "" || base.Hostname() == "" {
		return "", errors.New("invalid public hostname")
	}
	host := base.Hostname()
	if strings.ContainsAny(host, "%\\ \t\r\n'\"") {
		return "", errors.New("invalid public hostname")
	}
	if r.TLS == nil {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", errors.New("Frigate UI requires HTTPS")
		}
	}
	base.Host = net.JoinHostPort(host, port)
	return base.String(), nil
}
func (s *server) frigateUIStatus(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	w.Header().Set("Cache-Control", "no-store")
	state := s.frigateUI
	if state == nil || state.port == "" {
		writeJSON(w, 200, map[string]any{"available": false, "error": "Full Frigate UI listener is disabled"})
		return
	}
	origin, err := frigateOrigin(r, state.port)
	if err != nil || origin == requestOrigin(r) {
		writeAPIError(w, r, 503, "frigate_ui_origin_invalid", "Frigate requires a separate HTTPS origin", nil)
		return
	}
	state.mu.RLock()
	ready, problem := state.ready, state.problem
	state.mu.RUnlock()
	if !ready && problem == "" {
		problem = "Full Frigate UI listener is starting"
	}
	writeJSON(w, 200, map[string]any{"available": ready, "url": origin + "/", "error": problem, "authentication": "home-ai-core", "official_ui": true})
}
func requestOrigin(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// The protected gateway serves the upstream's ORIGINAL HTML, JS, REST API,
// WebSockets, MSE, HLS/VOD, downloads, exports and configuration without an API
// allowlist or a reimplementation. It never exposes Docker's unauthenticated
// port 5000 and never forwards a Core credential to the official container.
func (s *server) serveFrigateUI(w http.ResponseWriter, r *http.Request) {
	state := s.frigateUI
	if state == nil || state.port == "" {
		http.Error(w, "Frigate UI is disabled", 503)
		return
	}
	own, err := frigateOrigin(r, state.port)
	if err != nil || own != requestOrigin(r) {
		http.Error(w, "Invalid Frigate UI origin", 400)
		return
	}
	parent, err := frigateOrigin(r, state.corePort)
	if err != nil || parent == own {
		http.Error(w, "Frigate must have an isolated origin", 503)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "frame-ancestors "+parent)
	// Core tokens are HttpOnly cookies. An upstream token is never accepted as a
	// substitute for a Core session, and the browser never needs a URL token.
	token, source := sessionToken(r)
	actor, err := s.security.Authenticate(r.Context(), token)
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, security.ErrUnauthorized) {
			status = 503
		}
		frigateUIError(w, status, "Войдите в HomeAiCore и откройте Frigate из левого меню.")
		return
	}
	if !actor.Has("modules.manage") {
		frigateUIError(w, 403, "Доступ к Frigate разрешён только администраторам модулей.")
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		frigateUIError(w, 403, "Cross-site Frigate request refused")
		return
	}
	websocket := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
	mutation := r.Method != http.MethodGet && r.Method != http.MethodHead
	// Frigate uses its own frontend's CSRF header. We cannot reuse Core's
	// synchronizer token across origins. Require the exact browser Origin on ALL
	// writes and WS handshakes; no CORS preflights or null Origin are accepted.
	if mutation || websocket {
		if source != authBearer && r.Header.Get("Origin") != own {
			frigateUIError(w, 403, "Frigate request requires its own origin")
			return
		}
		if source == authBearer && r.Header.Get("Origin") != "" && r.Header.Get("Origin") != own {
			frigateUIError(w, 403, "Cross-origin Frigate request refused")
			return
		}
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		frigateUIError(w, 405, "Unsupported Frigate method")
		return
	}
	if s.modules == nil {
		frigateUIError(w, 503, "Frigate не установлен.")
		return
	}
	installed, err := s.modules.Get(r.Context(), frigate.ModuleID)
	if err != nil || installed.Status != "enabled" {
		frigateUIError(w, 503, "Frigate остановлен или недоступен. Запустите модуль в HomeAiCore.")
		return
	}
	if mutation {
		if r.ContentLength > 32<<20 {
			frigateUIError(w, 413, "Frigate request is too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		// Avoid native configuration/database mutations racing a backup/update.
		if !s.moduleSubmitMu.TryLock() {
			writeModuleSubmitError(w, r, errModuleOperationInProgress)
			return
		}
		defer s.moduleSubmitMu.Unlock()
		active, err := s.activeModuleJobs(r.Context())
		if err != nil {
			frigateUIError(w, 503, "Cannot check module operation state")
			return
		}
		if len(active) != 0 {
			writeModuleSubmitError(w, r, errModuleOperationInProgress)
			return
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if state.context != nil {
		stop := context.AfterFunc(state.context, cancel)
		defer stop()
	}
	address, err := s.resolveFrigateUI(ctx)
	if err != nil {
		frigateUIError(w, 502, "Не удалось подключиться к контейнеру Frigate.")
		return
	}
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort(address, "5000")}
	// Revoke long streams and WebSockets after logout, permission changes,
	// stop/removal, or Core shutdown, not just on the next browser page refresh.
	interval := state.recheck
	if interval <= 0 {
		interval = 15 * time.Second
	}
	go func() {
		timer := time.NewTicker(interval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				check, done := context.WithTimeout(ctx, 5*time.Second)
				current, e := s.security.Authenticate(check, token)
				module, me := s.modules.Get(check, frigate.ModuleID)
				done()
				if e != nil || !current.Has("modules.manage") || me != nil || module.Status != "enabled" {
					cancel()
					return
				}
			}
		}
	}()
	transport := s.frigateTransport
	if transport == nil {
		transport = frigateUITransport
	}
	proxy := &httputil.ReverseProxy{
		Transport: transport, FlushInterval: -1, ErrorLog: log.New(io.Discard, "", 0),
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			headers := make(http.Header)
			// Keep protocol/media headers only, never arbitrary user/proxy/secret data.
			for _, key := range []string{"Accept", "Accept-Encoding", "Accept-Language", "Content-Type", "Range", "If-Range", "If-None-Match", "If-Modified-Since", "Cache-Control", "X-Cache-Bypass", "Pragma", "User-Agent", "Connection", "Upgrade", "Sec-Websocket-Key", "Sec-Websocket-Version", "Sec-Websocket-Extensions", "Sec-Websocket-Protocol"} {
				if values := p.Out.Header.Values(key); len(values) > 0 {
					headers[key] = append([]string(nil), values...)
				}
			}
			p.Out.Header = headers
			p.Out.Host = r.Host
			p.Out.Header.Set("X-Forwarded-Host", r.Host)
			p.Out.Header.Set("X-Forwarded-Proto", strings.SplitN(own, ":", 2)[0])
			p.Out.Header.Set("X-CSRF-TOKEN", "home-ai-frigate-origin-verified")
			if websocket || mutation {
				p.Out.Header.Set("Origin", own)
			}
			p.Out.Header.Del("X-Ingress-Path") // Native app runs at / on its own origin.
		},
		ModifyResponse: func(response *http.Response) error {
			for key := range response.Header {
				lower := strings.ToLower(key)
				if strings.HasPrefix(lower, "access-control-") || lower == "set-cookie" || lower == "clear-site-data" || lower == "x-frame-options" || lower == "content-security-policy" || lower == "content-security-policy-report-only" {
					response.Header.Del(key)
				}
			}
			// Cookie namespaces are shared across ports. Frigate must NEVER set, clear
			// or replace a Core cookie. Authentication belongs to Core in this gateway.
			response.Header.Set("Cache-Control", "private, no-store")
			response.Header.Set("Referrer-Policy", "no-referrer")
			response.Header.Set("X-Content-Type-Options", "nosniff")
			response.Header.Set("Content-Security-Policy", "frame-ancestors "+parent)
			response.Header.Set("Permissions-Policy", "camera=(), microphone=(self), geolocation=()")
			if location := response.Header.Get("Location"); location != "" {
				next, e := url.Parse(location)
				if e != nil {
					return errors.New("invalid upstream redirect")
				}
				if next.IsAbs() || next.Host != "" {
					if next.Host != target.Host && next.Host != r.Host {
						return errors.New("external Frigate redirect refused")
					}
					next.Scheme = strings.SplitN(own, ":", 2)[0]
					next.Host = r.Host
					response.Header.Set("Location", next.String())
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			frigateUIError(w, 502, "Frigate временно недоступен. Проверьте состояние модуля в HomeAiCore.")
		},
	}
	proxy.ServeHTTP(w, r.WithContext(ctx))
	if mutation {
		s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor, "modules.frigate.native.request", "module", frigate.ModuleID, "submitted", map[string]any{"method": r.Method, "path": r.URL.Path})
	}
}

var frigateUITransport = &http.Transport{
	Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ResponseHeaderTimeout: 30 * time.Second, MaxIdleConns: 100, MaxIdleConnsPerHost: 32, IdleConnTimeout: 60 * time.Second,
}

func (s *server) resolveFrigateUI(ctx context.Context) (string, error) {
	resolver := s.frigateEndpoint
	if resolver == nil {
		resolver = modules.ResolveFrigateEndpoint
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	address, err := resolver(bounded)
	if err != nil {
		return "", err
	}
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() || !ip.IsPrivate() || ip.IsLoopback() {
		return "", errors.New("invalid managed Frigate address")
	}
	return ip.String(), nil
}
func frigateUIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, message)
}
