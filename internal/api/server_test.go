package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

type fakeState struct {
	pingErr         error
	schemaVersion   int
	schemaErr       error
	nasPools        []state.NASPoolRecord
	nasFolders      []state.NASFolderRecord
	storagePurposes []state.StoragePurposeRecord
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

func (f fakeState) SetStoragePurpose(
	_ context.Context,
	devicePath, filesystemUUID, purpose string,
	now time.Time,
) (state.StoragePurposeRecord, error) {
	return state.StoragePurposeRecord{
		DevicePath:     devicePath,
		FilesystemUUID: filesystemUUID,
		Purpose:        purpose,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func (f fakeState) ClearStoragePurpose(context.Context, string, string) error {
	return nil
}

func (f fakeState) ListStoragePurposes(context.Context) ([]state.StoragePurposeRecord, error) {
	return f.storagePurposes, nil
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

func (f fakeState) NASFolder(_ context.Context, folderID string) (state.NASFolderRecord, error) {
	for _, folder := range f.nasFolders {
		if folder.ID == folderID {
			return folder, nil
		}
	}
	return state.NASFolderRecord{}, state.ErrNASFolderNotFound
}

func (f fakeState) DeleteNASFolder(context.Context, string) error {
	return nil
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
	_ context.Context,
	_ security.Actor,
	input security.UserProfileInput,
	_ security.RequestContext,
) (security.User, error) {
	profile := input.Profile
	if profile == "" {
		profile = security.ProfileMember
	}
	return security.User{
		ID:          "usr-member",
		Username:    input.Username,
		DisplayName: input.DisplayName,
		Roles:       []string{profile},
		Profile:     profile,
		Permissions: append([]string(nil), input.Permissions...),
		ResourcePermissions: append([]security.PermissionScope(nil), input.ResourcePermissions...),
	}, nil
}

func (f fakeSecurity) UpdateUser(
	_ context.Context,
	_ security.Actor,
	userID string,
	input security.UserProfileInput,
	_ security.RequestContext,
) (security.User, error) {
	return security.User{
		ID:                  userID,
		Username:            "member",
		DisplayName:         input.DisplayName,
		Disabled:            input.Disabled,
		Roles:               []string{input.Profile},
		Profile:             input.Profile,
		Permissions:         append([]string(nil), input.Permissions...),
		ResourcePermissions: append([]security.PermissionScope(nil), input.ResourcePermissions...),
	}, nil
}

func (f fakeSecurity) ListUsers(context.Context) ([]security.User, error) {
	return []security.User{
		{
			ID:          "usr-test",
			Username:    "owner",
			DisplayName: "Owner",
			Roles:       []string{"owner"},
			Profile:     security.ProfileAdministrator,
			Owner:       true,
		},
	}, nil
}

func (f fakeSecurity) AccessCatalog(context.Context) (security.AccessCatalog, error) {
	return security.AccessCatalog{
		Permissions: []security.PermissionDefinition{
			{Name: "files.read", Description: "Read files"},
			{Name: "files.write", Description: "Write files"},
			{Name: "system.read", Description: "Read system"},
		},
		Profiles: []security.ProfileTemplate{
			{
				Name:               security.ProfileAdministrator,
				Description:        "Administrator",
				DefaultPermissions: []string{"files.read", "files.write", "system.read"},
				FullAccess:         true,
			},
			{
				Name:               security.ProfileParent,
				Description:        "Parent",
				DefaultPermissions: []string{"system.read"},
			},
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

func TestFileFoldersFilterScopedAccess(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{
		{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf-visible"},
		{Permission: "files.write", ResourceType: "file_folder", ResourceID: "nsf-visible"},
	}
	handler := testHandlerWithSecurity(fakeState{
		nasFolders: []state.NASFolderRecord{
			{
				ID:           "nsf-visible",
				PoolID:       "nsp-main",
				PoolName:     "Main",
				Name:         "My files",
				Kind:         "private",
				OwnerUserID:  "usr-test",
				RelativePath: "users/usr-test/nsf-visible",
			},
			{
				ID:           "nsf-hidden",
				PoolID:       "nsp-main",
				PoolName:     "Main",
				Name:         "Other",
				Kind:         "private",
				OwnerUserID:  "usr-other",
				RelativePath: "users/usr-other/nsf-hidden",
			},
		},
	}, sec)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/folders", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Folders []fileFolderResponse `json:"folders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Folders) != 1 || body.Folders[0].ID != "nsf-visible" {
		t.Fatalf("unexpected visible folders: %#v", body.Folders)
	}
	if !body.Folders[0].CanWrite {
		t.Fatal("scoped files.write was not reflected in response")
	}
}

func TestStoragePurposeResponseFollowsFilesystemUUID(t *testing.T) {
	records := state.StoragePurposeRecord{
		DevicePath:     "/dev/sdb1",
		FilesystemUUID: "uuid-files",
		Purpose:        state.StoragePurposeFiles,
	}
	nodes := []systeminfo.BlockNode{
		{
			Path: "/dev/nvme1n1",
			Type: "disk",
			Children: []systeminfo.BlockNode{
				{
					Path:        "/dev/nvme1n1p1",
					Type:        "part",
					Filesystem:  "ext4",
					UUID:        "uuid-files",
					Label:       "DATA",
					Mountpoints: []string{"/mnt/home-ai-core/data"},
					SizeBytes:   1000,
					FreeBytes:   400,
					FreeKnown:   true,
				},
			},
		},
	}

	response := storagePurposeResponseFor(records, nodes)
	if !response.Present {
		t.Fatal("purpose assignment was not matched to present filesystem")
	}
	if response.DevicePath != "/dev/nvme1n1p1" {
		t.Fatalf("device = %q", response.DevicePath)
	}
	if response.Purpose != state.StoragePurposeFiles || response.Filesystem != "ext4" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if len(response.Mountpoints) != 1 || response.Mountpoints[0] != "/mnt/home-ai-core/data" {
		t.Fatalf("mountpoints = %#v", response.Mountpoints)
	}
}

func TestStoragePurposeResponseKeepsMissingAssignment(t *testing.T) {
	record := state.StoragePurposeRecord{
		DevicePath:     "/dev/sdc1",
		FilesystemUUID: "uuid-missing",
		Purpose:        state.StoragePurposeVideo,
	}
	response := storagePurposeResponseFor(record, nil)
	if response.Present {
		t.Fatal("missing storage was reported as present")
	}
	if response.DevicePath != "/dev/sdc1" || response.Purpose != state.StoragePurposeVideo {
		t.Fatalf("unexpected missing response: %#v", response)
	}
}

func TestFilePoolCreateRequiresManage(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	handler := testHandlerWithSecurity(fakeState{}, sec)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/pools",
		strings.NewReader(`{"name":"Main","root_path":"/srv/home-ai/main"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func stubNASProvisioning(t *testing.T) {
	t.Helper()
	originalPool := prepareFilePool
	originalFolder := prepareFileFolder
	prepareFilePool = func(context.Context, string) error { return nil }
	prepareFileFolder = func(context.Context, string, string) error { return nil }
	t.Cleanup(func() {
		prepareFilePool = originalPool
		prepareFileFolder = originalFolder
	})
}

func TestFilePoolCreate(t *testing.T) {
	stubNASProvisioning(t)
	handler := testHandler(fakeState{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/pools",
		strings.NewReader(`{"name":"Main","root_path":"/srv/home-ai/main"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestSharedFileFolderCreate(t *testing.T) {
	stubNASProvisioning(t)
	handler := testHandler(fakeState{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/folders",
		strings.NewReader(`{"pool_id":"nsp-main","name":"Family","kind":"shared"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestFileContentScopedReadWrite(t *testing.T) {
	poolRoot := t.TempDir()
	folderRoot := filepath.Join(poolRoot, ".home-ai", "shared", "nsf-visible")
	if err := os.MkdirAll(folderRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folderRoot, "existing.txt"), []byte("existing"), 0o640); err != nil {
		t.Fatal(err)
	}

	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{
		{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf-visible"},
		{Permission: "files.write", ResourceType: "file_folder", ResourceID: "nsf-visible"},
	}
	handler := testHandlerWithSecurity(fakeState{
		nasFolders: []state.NASFolderRecord{
			{
				ID:           "nsf-visible",
				PoolID:       "nsp-main",
				PoolName:     "Main",
				PoolRoot:     poolRoot,
				Name:         "Family",
				Kind:         "shared",
				RelativePath: "shared/nsf-visible",
			},
		},
	}, sec)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/folders/nsf-visible/entries", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/files/folders/nsf-visible/content?path=upload.txt",
		strings.NewReader("uploaded"),
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/files/folders/nsf-visible/content?path=upload.txt",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "uploaded" {
		t.Fatalf("download body = %q", rec.Body.String())
	}
}

func TestFileContentRejectsTraversalAndMissingWriteScope(t *testing.T) {
	poolRoot := t.TempDir()
	folderRoot := filepath.Join(poolRoot, ".home-ai", "shared", "nsf-visible")
	if err := os.MkdirAll(folderRoot, 0o750); err != nil {
		t.Fatal(err)
	}

	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{
		{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf-visible"},
	}
	handler := testHandlerWithSecurity(fakeState{
		nasFolders: []state.NASFolderRecord{
			{
				ID:           "nsf-visible",
				PoolID:       "nsp-main",
				PoolName:     "Main",
				PoolRoot:     poolRoot,
				Name:         "Family",
				Kind:         "shared",
				RelativePath: "shared/nsf-visible",
			},
		},
	}, sec)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/files/folders/nsf-visible/entries?path=../outside",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	req = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/files/folders/nsf-visible/content?path=nope.txt",
		strings.NewReader("nope"),
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("write without scope status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestFileMoveAndDelete(t *testing.T) {
	poolRoot := t.TempDir()
	folderRoot := filepath.Join(poolRoot, ".home-ai", "shared", "nsf-visible")
	if err := os.MkdirAll(filepath.Join(folderRoot, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folderRoot, "docs", "old.txt"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}

	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{
		{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf-visible"},
		{Permission: "files.write", ResourceType: "file_folder", ResourceID: "nsf-visible"},
	}
	handler := testHandlerWithSecurity(fakeState{
		nasFolders: []state.NASFolderRecord{
			{
				ID:           "nsf-visible",
				PoolID:       "nsp-main",
				PoolName:     "Main",
				PoolRoot:     poolRoot,
				Name:         "Family",
				Kind:         "shared",
				RelativePath: "shared/nsf-visible",
			},
		},
	}, sec)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/folders/nsf-visible/move",
		strings.NewReader(`{"from_path":"docs/old.txt","to_path":"docs/new.txt"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("move status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/files/folders/nsf-visible/entry?path=docs%2Fnew.txt",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(folderRoot, "docs", "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted file still exists: %v", err)
	}

	if err := os.WriteFile(filepath.Join(folderRoot, "docs", "child.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/files/folders/nsf-visible/entry?path=docs",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("non-empty directory trash status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if _, err := os.Stat(filepath.Join(folderRoot, "docs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("trashed directory still exists: %v", err)
	}
}

func TestFileTrashRestoreAndPurgeAPI(t *testing.T) {
	poolRoot := t.TempDir()
	folderRoot := filepath.Join(poolRoot, ".home-ai", "shared", "nsf-visible")
	if err := os.MkdirAll(folderRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folderRoot, "trash-me.txt"), []byte("trash"), 0o640); err != nil {
		t.Fatal(err)
	}

	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{
		{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf-visible"},
		{Permission: "files.write", ResourceType: "file_folder", ResourceID: "nsf-visible"},
	}
	handler := testHandlerWithSecurity(fakeState{
		nasFolders: []state.NASFolderRecord{{
			ID:           "nsf-visible",
			PoolID:       "nsp-main",
			PoolName:     "Main",
			PoolRoot:     poolRoot,
			Name:         "Family",
			Kind:         "shared",
			RelativePath: "shared/nsf-visible",
		}},
	}, sec)

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/files/folders/nsf-visible/entry?path=trash-me.txt",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("trash status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(folderRoot, "trash-me.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists after trash: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/files/folders/nsf-visible/trash", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("trash list status = %d: %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Trash []filedata.TrashEntry `json:"trash"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode trash list: %v", err)
	}
	if len(listed.Trash) != 1 || listed.Trash[0].OriginalPath != "trash-me.txt" {
		t.Fatalf("unexpected trash list: %#v", listed.Trash)
	}
	trashID := listed.Trash[0].ID

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/folders/nsf-visible/trash/"+trashID+"/restore",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d: %s", rec.Code, rec.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(folderRoot, "trash-me.txt")); err != nil || string(data) != "trash" {
		t.Fatalf("restored data = %q err=%v", data, err)
	}

	req = httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/files/folders/nsf-visible/entry?path=trash-me.txt",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("second trash status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/files/folders/nsf-visible/trash", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode second trash list: %v", err)
	}
	if len(listed.Trash) != 1 {
		t.Fatalf("trash count = %d, want 1", len(listed.Trash))
	}

	req = httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/files/folders/nsf-visible/trash/"+listed.Trash[0].ID,
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("purge status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFileTrashMutationsRequireWriteScope(t *testing.T) {
	poolRoot := t.TempDir()
	folderRoot := filepath.Join(poolRoot, ".home-ai", "shared", "nsf-visible")
	if err := os.MkdirAll(folderRoot, 0o750); err != nil {
		t.Fatal(err)
	}

	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{
		{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf-visible"},
	}
	handler := testHandlerWithSecurity(fakeState{
		nasFolders: []state.NASFolderRecord{{
			ID:           "nsf-visible",
			PoolID:       "nsp-main",
			PoolName:     "Main",
			PoolRoot:     poolRoot,
			Name:         "Family",
			Kind:         "shared",
			RelativePath: "shared/nsf-visible",
		}},
	}, sec)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/folders/nsf-visible/trash/0123456789abcdef0123456789abcdef/restore",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("restore without write scope status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestResumableFileUploadAPI(t *testing.T) {
	poolRoot := t.TempDir()
	folderRoot := filepath.Join(poolRoot, ".home-ai", "shared", "nsf-visible")
	if err := os.MkdirAll(folderRoot, 0o750); err != nil {
		t.Fatal(err)
	}

	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"security.self.read"}
	sec.actor.ResourcePermissions = []security.PermissionScope{
		{Permission: "files.read", ResourceType: "file_folder", ResourceID: "nsf-visible"},
		{Permission: "files.write", ResourceType: "file_folder", ResourceID: "nsf-visible"},
	}
	handler := testHandlerWithSecurity(fakeState{
		nasFolders: []state.NASFolderRecord{{
			ID:           "nsf-visible",
			PoolID:       "nsp-main",
			PoolName:     "Main",
			PoolRoot:     poolRoot,
			Name:         "Family",
			Kind:         "shared",
			RelativePath: "shared/nsf-visible",
		}},
	}, sec)

	payload := []byte("hello world")
	fullHash := sha256.Sum256(payload)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/folders/nsf-visible/uploads",
		strings.NewReader(`{"path":"large.bin","total_bytes":11,"sha256":"`+hex.EncodeToString(fullHash[:])+`","client_fingerprint":"large.bin:11:1"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create upload status = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Upload filedata.UploadSession `json:"upload"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Upload.ID == "" || created.Upload.ReceivedBytes != 0 {
		t.Fatalf("unexpected upload session: %#v", created.Upload)
	}

	first := []byte("hello ")
	firstHash := sha256.Sum256(first)
	req = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/files/folders/nsf-visible/uploads/"+created.Upload.ID+"/chunk",
		strings.NewReader(string(first)),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Upload-Offset", "0")
	req.Header.Set("X-Chunk-SHA256", hex.EncodeToString(firstHash[:]))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first chunk status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/files/folders/nsf-visible/uploads",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list uploads status = %d: %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Uploads []filedata.UploadSession `json:"uploads"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Uploads) != 1 || listed.Uploads[0].ReceivedBytes != int64(len(first)) {
		t.Fatalf("unexpected upload list: %#v", listed.Uploads)
	}

	second := []byte("world")
	secondHash := sha256.Sum256(second)
	req = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/files/folders/nsf-visible/uploads/"+created.Upload.ID+"/chunk",
		strings.NewReader(string(second)),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Upload-Offset", "6")
	req.Header.Set("X-Chunk-SHA256", hex.EncodeToString(secondHash[:]))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second chunk status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/folders/nsf-visible/uploads/"+created.Upload.ID+"/complete",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete upload status = %d: %s", rec.Code, rec.Body.String())
	}
	var completed struct {
		File filedata.UploadResult `json:"file"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &completed); err != nil {
		t.Fatal(err)
	}
	if completed.File.SHA256 != hex.EncodeToString(fullHash[:]) {
		t.Fatalf("completed SHA-256 = %q", completed.File.SHA256)
	}
	data, err := os.ReadFile(filepath.Join(folderRoot, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(payload) {
		t.Fatalf("uploaded data = %q", data)
	}
}
