package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type fakeState struct {
	pingErr       error
	schemaVersion int
	schemaErr     error
	nasPools      []state.NASPoolRecord
	nasFolders    []state.NASFolderRecord
}

func (f fakeState) Ping(context.Context) error {
	return f.pingErr
}

func (f fakeState) SchemaVersion(context.Context) (int, error) {
	return f.schemaVersion, f.schemaErr
}

func (f fakeState) ClearTerminalJobs(context.Context) (int64, error) {
	return 0, nil
}

func (f fakeState) DiskNames(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (f fakeState) SetDiskName(context.Context, string, string) error {
	return nil
}

func (f fakeState) CreateNASPool(
	_ context.Context,
	name, rootPath, createdBy string,
	now time.Time,
) (state.NASPoolRecord, error) {
	return state.NASPoolRecord{
		ID:        "nsp-test",
		Name:      name,
		RootPath:  rootPath,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (f fakeState) ListNASPools(context.Context) ([]state.NASPoolRecord, error) {
	return f.nasPools, nil
}

func (f fakeState) CreateNASFolder(
	_ context.Context,
	poolID, name, kind, ownerUserID, createdBy string,
	now time.Time,
) (state.NASFolderRecord, error) {
	return state.NASFolderRecord{
		ID:           "nsf-test",
		PoolID:       poolID,
		PoolName:     "Main",
		Name:         name,
		Kind:         kind,
		OwnerUserID:  ownerUserID,
		RelativePath: "shared/nsf-test",
		CreatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (f fakeState) ListNASFolders(context.Context) ([]state.NASFolderRecord, error) {
	return f.nasFolders, nil
}

type fakeSecurity struct {
	initialized bool
	actor       security.Actor
	authErr     error
}

func defaultFakeSecurity() fakeSecurity {
	return fakeSecurity{
		initialized: true,
		actor: security.Actor{
			Type:        "user",
			ID:          "usr-test",
			Username:    "owner",
			DisplayName: "Owner",
			Roles:       []string{"owner"},
			Permissions: []string{
				"system.read",
				"events.read",
				"security.self.read",
				"security.sessions.manage",
				"security.users.read",
				"security.users.manage",
				"audit.read",
				"modules.read",
				"updates.read",
				"updates.manage",
				"files.read",
				"files.write",
				"files.manage",
			},
		},
	}
}

func (f fakeSecurity) Initialized(context.Context) (bool, error) {
	return f.initialized, nil
}

func (f fakeSecurity) Bootstrap(
	context.Context,
	string,
	string,
	string,
	string,
	security.RequestContext,
) (security.AuthResult, error) {
	return security.AuthResult{}, errors.New("not implemented in fake")
}

func (f fakeSecurity) Login(
	context.Context,
	string,
	string,
	security.RequestContext,
) (security.AuthResult, error) {
	return security.AuthResult{}, errors.New("not implemented in fake")
}

func (f fakeSecurity) Authenticate(context.Context, string) (security.Actor, error) {
	if f.authErr != nil {
		return security.Actor{}, f.authErr
	}
	return f.actor, nil
}

func (f fakeSecurity) Logout(context.Context, security.Actor, security.RequestContext) error {
	return nil
}

func (f fakeSecurity) CreateUser(
	context.Context,
	security.Actor,
	string,
	string,
	string,
	security.RequestContext,
) (security.User, error) {
	return security.User{
		ID:          "usr-member",
		Username:    "member",
		DisplayName: "Member",
		Roles:       []string{"member"},
	}, nil
}

func (f fakeSecurity) ListUsers(context.Context) ([]security.User, error) {
	return []security.User{
		{
			ID:          "usr-test",
			Username:    "owner",
			DisplayName: "Owner",
			Roles:       []string{"owner"},
		},
	}, nil
}

func (f fakeSecurity) ListAudit(context.Context, int) ([]security.AuditEntry, error) {
	return []security.AuditEntry{}, nil
}

func (f fakeSecurity) RecordAudit(
	context.Context,
	security.RequestContext,
	security.Actor,
	string,
	string,
	string,
	string,
	map[string]any,
) {
}

func testHandler(state fakeState) http.Handler {
	return testHandlerWithSecurity(state, defaultFakeSecurity())
}

func testHandlerWithSecurity(state fakeState, securityService SecurityService) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	return New(
		nodeID,
		logger,
		state,
		securityService,
		nil,
		nil,
		nil,
		nil,
		realtime.New(nodeID, logger),
	)
}

func TestHealth(t *testing.T) {
	handler := testHandler(fakeState{schemaVersion: 4})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID")
	}
	if rec.Header().Get("X-Correlation-ID") == "" {
		t.Fatal("missing X-Correlation-ID")
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %#v", body["status"])
	}
}

func TestCorrelationIDIsEchoed(t *testing.T) {
	handler := testHandler(fakeState{schemaVersion: 4})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Correlation-ID", "mobile-upload-42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Correlation-ID"); got != "mobile-upload-42" {
		t.Fatalf("correlation id = %q", got)
	}
}

func TestInvalidCorrelationIDFallsBackToRequestID(t *testing.T) {
	handler := testHandler(fakeState{schemaVersion: 4})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Correlation-ID", "contains spaces")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Correlation-ID") != rec.Header().Get("X-Request-ID") {
		t.Fatal("invalid correlation ID was not replaced")
	}
}

func TestHealthFailsWhenStateIsUnavailable(t *testing.T) {
	handler := testHandler(fakeState{pingErr: errors.New("offline")})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestProtectedRouteRequiresAuthentication(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.authErr = security.ErrUnauthorized
	handler := testHandlerWithSecurity(fakeState{schemaVersion: 4}, sec)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "authentication_required" {
		t.Fatalf("error code = %q", body.Error.Code)
	}
}

func TestSystem(t *testing.T) {
	const nodeID = "00000000-0000-4000-8000-000000000000"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New(
		nodeID,
		logger,
		fakeState{schemaVersion: 4},
		defaultFakeSecurity(),
		nil,
		nil,
		nil,
		nil,
		realtime.New(nodeID, logger),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		SchemaVersion int `json:"schema_version"`
		System        struct {
			NodeID string `json:"node_id"`
		} `json:"system"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.System.NodeID != nodeID {
		t.Fatalf("node_id = %q, want %q", body.System.NodeID, nodeID)
	}
	if body.SchemaVersion != 4 {
		t.Fatalf("schema_version = %d, want 4", body.SchemaVersion)
	}
}

func TestAPIErrorEnvelope(t *testing.T) {
	handler := testHandler(fakeState{schemaErr: errors.New("database unavailable")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	req.Header.Set("X-Correlation-ID", "diagnostic-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}

	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Code != "state_unavailable" {
		t.Fatalf("error code = %q", body.Error.Code)
	}
	if body.Error.RequestID == "" {
		t.Fatal("missing error request ID")
	}
	if body.Error.CorrelationID != "diagnostic-1" {
		t.Fatalf("correlation ID = %q", body.Error.CorrelationID)
	}
}

func TestNotFoundUsesErrorEnvelope(t *testing.T) {
	handler := testHandler(fakeState{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}

	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("error code = %q", body.Error.Code)
	}
}

func TestMethodNotAllowedUsesErrorEnvelope(t *testing.T) {
	handler := testHandler(fakeState{schemaVersion: 4})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q", rec.Header().Get("Allow"))
	}
}

func TestSetupStatusIsPublic(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.initialized = false
	handler := testHandlerWithSecurity(fakeState{}, sec)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/security/setup-status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Initialized bool `json:"initialized"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Initialized {
		t.Fatal("setup status unexpectedly initialized")
	}
}

func TestEventsRouteUpgradesToWebSocket(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	handler := New(
		nodeID,
		logger,
		fakeState{schemaVersion: 4},
		defaultFakeSecurity(),
		nil,
		nil,
		nil,
		nil,
		realtime.New(nodeID, logger, realtime.WithHeartbeat(time.Hour)),
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/events"
	conn, response, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test complete")

	if response == nil || response.Header.Get("X-Request-ID") == "" {
		t.Fatal("websocket upgrade missing X-Request-ID")
	}

	var connected realtime.Envelope
	if err := wsjson.Read(ctx, conn, &connected); err != nil {
		t.Fatalf("read connected event: %v", err)
	}
	if connected.Type != "core.connected" {
		t.Fatalf("event type = %q", connected.Type)
	}
	if connected.Source.NodeID != nodeID {
		t.Fatalf("node ID = %q", connected.Source.NodeID)
	}
}

func TestInvalidSessionModeIsRejectedBeforeLogin(t *testing.T) {
	handler := testHandler(fakeState{schemaVersion: 4})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		strings.NewReader(`{"username":"owner","password":"not-used-here","session_mode":"invalid"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "invalid_session_mode" {
		t.Fatalf("error code = %q", body.Error.Code)
	}
}

func TestAuthenticationBackendFailureIsUnavailable(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.authErr = errors.New("database failed")
	handler := testHandlerWithSecurity(fakeState{schemaVersion: 4}, sec)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "authentication_unavailable" {
		t.Fatalf("error code = %q", body.Error.Code)
	}
}

type fakeModules struct {
	items        []modules.Registered
	capabilities []string
}

func (f fakeModules) List(context.Context) ([]modules.Registered, error) {
	return f.items, nil
}

func (f fakeModules) Get(_ context.Context, id string) (modules.Registered, error) {
	for _, item := range f.items {
		if item.Manifest.ID == id {
			return item, nil
		}
	}
	return modules.Registered{}, modules.ErrModuleNotFound
}

func (f fakeModules) Capabilities(context.Context) ([]string, error) {
	return f.capabilities, nil
}

func TestModulesAPI(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	moduleService := fakeModules{
		items: []modules.Registered{
			{
				Manifest: modules.Manifest{
					SchemaVersion: 1,
					ID:            "storage",
					Name:          "Storage",
					Version:       "0.1.0",
					Core:          ">=0.1.0 <1.0.0",
					Lifecycle:     []string{"install"},
				},
				Status: "registered",
			},
		},
		capabilities: []string{"host.linux", "storage.block"},
	}
	handler := New(
		nodeID,
		logger,
		fakeState{schemaVersion: 4},
		defaultFakeSecurity(),
		nil,
		nil,
		moduleService,
		nil,
		realtime.New(nodeID, logger),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/modules", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("modules status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/modules/storage", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("module status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/modules/capabilities", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d", rec.Code)
	}
}

func TestUsersListRequiresPermission(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	handler := testHandlerWithSecurity(fakeState{}, sec)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/security/users", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestUsersList(t *testing.T) {
	handler := testHandler(fakeState{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/security/users", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Users []security.User `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Users) != 1 || body.Users[0].Username != "owner" {
		t.Fatalf("unexpected users: %#v", body.Users)
	}
}

func TestCreateUserWithBearerSession(t *testing.T) {
	handler := testHandler(fakeState{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/security/users",
		strings.NewReader(`{"username":"member","display_name":"Member","password":"correct horse battery staple"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}
