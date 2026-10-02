package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/aiagent"
	"github.com/DeadSoulf/home-ai-core/internal/nvr"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
	"github.com/DeadSoulf/home-ai-core/internal/version"
)

type server struct {
	nodeID              string
	logger              *slog.Logger
	state               State
	security            SecurityService
	jobs                JobService
	eventHistoryService EventHistoryService
	modules             ModuleService
	ai                  *aiagent.Service
	nvr                 *nvr.Service
	updater             UpdaterService
	realtime            *realtime.Hub
	mux                 *http.ServeMux
}

func New(
	nodeID string,
	logger *slog.Logger,
	state State,
	securityService SecurityService,
	jobService JobService,
	eventHistoryService EventHistoryService,
	moduleService ModuleService,
	updaterService UpdaterService,
	realtimeHub *realtime.Hub,
	aiProviders ...aiagent.Provider,
) http.Handler {
	return newServer(
		nodeID,
		logger,
		state,
		securityService,
		jobService,
		eventHistoryService,
		moduleService,
		updaterService,
		realtimeHub,
		nil,
		aiProviders...,
	)
}

func NewWithNVR(
	nodeID string,
	logger *slog.Logger,
	state State,
	securityService SecurityService,
	jobService JobService,
	eventHistoryService EventHistoryService,
	moduleService ModuleService,
	updaterService UpdaterService,
	realtimeHub *realtime.Hub,
	nvrService *nvr.Service,
	aiProviders ...aiagent.Provider,
) http.Handler {
	return newServer(
		nodeID,
		logger,
		state,
		securityService,
		jobService,
		eventHistoryService,
		moduleService,
		updaterService,
		realtimeHub,
		nvrService,
		aiProviders...,
	)
}

func newServer(
	nodeID string,
	logger *slog.Logger,
	state State,
	securityService SecurityService,
	jobService JobService,
	eventHistoryService EventHistoryService,
	moduleService ModuleService,
	updaterService UpdaterService,
	realtimeHub *realtime.Hub,
	nvrService *nvr.Service,
	aiProviders ...aiagent.Provider,
) http.Handler {
	aiService := aiagent.NewService(nodeID, state, jobService, moduleService, securityService, aiProviders...)
	if moduleService != nil {
		if item, err := moduleService.Get(context.Background(), "ai.agent"); err == nil {
			switch item.Status {
			case "disabled", "error":
				aiService.SetEnabled(false)
			case "registered":
				_ = moduleService.SetStatus(context.Background(), "ai.agent", "enabled", "")
			}
		}
	}
	s := &server{
		nodeID:              nodeID,
		logger:              logger,
		state:               state,
		security:            securityService,
		jobs:                jobService,
		eventHistoryService: eventHistoryService,
		modules:             moduleService,
		ai:                  aiService,
		nvr:                 nvrService,
		updater:             updaterService,
		realtime:            realtimeHub,
		mux:                 http.NewServeMux(),
	}

	s.mux.HandleFunc("GET /health", s.health)
	s.mux.HandleFunc("/api/v1/security/setup-status", s.setupStatus)
	s.mux.HandleFunc("/api/v1/security/bootstrap", s.bootstrap)
	s.mux.HandleFunc("/api/v1/auth/login", s.login)
	s.mux.HandleFunc("/api/v1/auth/me", s.requireAuth(
		"security.self.read",
		func(w http.ResponseWriter, r *http.Request, actor security.Actor, _ authSource) {
			s.me(w, r, actor)
		},
	))
	s.mux.HandleFunc("/api/v1/auth/logout", s.requireAuth("", s.logout))
	s.mux.HandleFunc("PUT /api/v1/auth/password", s.requireAuth("security.self.read", s.accountPassword))
	s.mux.HandleFunc("GET /api/v1/security/users", s.requireAuth("security.users.read", s.usersCollection))
	s.mux.HandleFunc("POST /api/v1/security/users", s.requireAuth("security.users.manage", s.usersCollection))
	s.mux.HandleFunc("GET /api/v1/security/access-catalog", s.requireAuth("security.users.manage", s.accessCatalog))
	s.mux.HandleFunc("PUT /api/v1/security/users/{userID}/access", s.requireAuth("security.users.manage", s.userAccess))
	s.mux.HandleFunc("PUT /api/v1/security/users/{userID}/identity", s.requireAuth("security.users.manage", s.userIdentity))
	s.mux.HandleFunc("PUT /api/v1/security/users/{userID}/password", s.requireAuth("security.users.manage", s.userPassword))
	s.mux.HandleFunc("/api/v1/system", s.requireAuth(
		"system.read",
		func(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
			s.systemRoute(w, r)
		},
	))
	s.mux.HandleFunc("/api/v1/events/history", s.requireAuth("events.read", s.eventHistory))
	s.mux.HandleFunc("/api/v1/events", s.requireAuth(
		"events.read",
		func(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
			s.eventsRoute(w, r)
		},
	))
	s.mux.HandleFunc("/api/v1/jobs", s.requireAuth("jobs.read", s.jobsCollection))
	s.mux.HandleFunc("/api/v1/jobs/", s.requireAuth("jobs.read", s.jobResource))
	s.mux.HandleFunc("/api/v1/modules", s.requireAuth("modules.read", s.modulesCollection))
	s.mux.HandleFunc("GET /api/v1/modules/navigation", s.requireAuth("", s.moduleNavigation))
	s.mux.HandleFunc("/api/v1/modules/capabilities", s.requireAuth("modules.read", s.moduleCapabilities))
	s.mux.HandleFunc("POST /api/v1/modules/ai.cloud/test", s.requireAuth("modules.manage", s.cloudAIModuleTest))
	s.mux.HandleFunc("POST /api/v1/modules/{moduleID}/control", s.requireAuth("modules.manage", s.moduleControl))
	s.mux.HandleFunc("/api/v1/modules/", s.requireAuth("modules.read", s.moduleResource))
	s.mux.HandleFunc("GET /api/v1/nvr/status", s.requireAuth("security.self.read", s.nvrStatus))
	s.mux.HandleFunc("GET /api/v1/nvr/cameras", s.requireAuth("security.self.read", s.nvrCameras))
	s.mux.HandleFunc("POST /api/v1/nvr/cameras", s.requireAuth("security.self.read", s.nvrCameraCreate))
	s.mux.HandleFunc("POST /api/v1/nvr/cameras/test", s.requireAuth("security.self.read", s.nvrCameraTest))
	s.mux.HandleFunc("GET /api/v1/nvr/cameras/{cameraID}", s.requireAuth("security.self.read", s.nvrCameraResource))
	s.mux.HandleFunc("PUT /api/v1/nvr/cameras/{cameraID}", s.requireAuth("security.self.read", s.nvrCameraResource))
	s.mux.HandleFunc("DELETE /api/v1/nvr/cameras/{cameraID}", s.requireAuth("security.self.read", s.nvrCameraResource))
	s.mux.HandleFunc("POST /api/v1/nvr/cameras/{cameraID}/test", s.requireAuth("security.self.read", s.nvrCameraExistingTest))
	s.mux.HandleFunc("GET /api/v1/nvr/cameras/{cameraID}/live.mjpeg", s.requireAuth("security.self.read", s.nvrCameraLiveMJPEG))
	s.mux.HandleFunc("/api/v1/ai/status", s.requireAuth("", s.aiStatus))
	s.mux.HandleFunc("/api/v1/ai/tools", s.requireAuth("", s.aiTools))
	s.mux.HandleFunc("/api/v1/ai/tools/", s.requireAuth("", s.aiToolResource))
	s.mux.HandleFunc("/api/v1/ai/conversations", s.requireAuth("", s.aiConversations))
	s.mux.HandleFunc("DELETE /api/v1/ai/conversations/closed", s.requireAuth("", s.aiClosedConversationsDelete))
	s.mux.HandleFunc("DELETE /api/v1/ai/conversations/{conversationID}", s.requireAuth("", s.aiConversationDelete))
	s.mux.HandleFunc("POST /api/v1/ai/conversations/{conversationID}/close", s.requireAuth("", s.aiConversationResource))
	s.mux.HandleFunc("POST /api/v1/ai/conversations/{conversationID}/messages/stream", s.requireAuth("", s.aiConversationMessageStream))
	s.mux.HandleFunc("GET /api/v1/ai/conversations/{conversationID}/actions", s.requireAuth("", s.aiConversationActions))
	s.mux.HandleFunc("POST /api/v1/ai/conversations/{conversationID}/actions/{actionID}/approve", s.requireAuth("", s.aiConversationActionApprove))
	s.mux.HandleFunc("POST /api/v1/ai/conversations/{conversationID}/actions/{actionID}/reject", s.requireAuth("", s.aiConversationActionReject))
	s.mux.HandleFunc("/api/v1/ai/conversations/", s.requireAuth("", s.aiConversationResource))
	s.mux.HandleFunc("/api/v1/update", s.requireAuth("updates.read", func(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
		s.updateStatus(w, r)
	}))
	s.mux.HandleFunc("/api/v1/update/state", s.requireAuth("updates.read", func(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
		s.updateState(w, r)
	}))
	s.mux.HandleFunc("/api/v1/update/download", s.requireAuth("updates.manage", s.updateDownload))
	s.mux.HandleFunc("/api/v1/update/install", s.requireAuth("updates.manage", s.updateInstall))
	s.mux.HandleFunc("/api/v1/update/rollback", s.requireAuth("updates.manage", s.updateRollback))
	s.mux.HandleFunc("/api/v1/storage/operation", s.requireAuth("storage.manage", s.storageOperation))
	s.mux.HandleFunc("/api/v1/storage/name", s.requireAuth("storage.manage", s.storageName))
	s.mux.HandleFunc("GET /api/v1/storage/purposes", s.requireAuth("system.read", s.storagePurposes))
	s.mux.HandleFunc("POST /api/v1/storage/purposes", s.requireAuth("storage.manage", s.storagePurposes))
	s.mux.HandleFunc("GET /api/v1/files/owners", s.requireAuth("files.manage", s.fileOwners))
	s.mux.HandleFunc("GET /api/v1/files/quotas", s.requireAuth("files.manage", s.fileUserQuotas))
	s.mux.HandleFunc("PUT /api/v1/files/users/{userID}/quota", s.requireAuth("files.manage", s.fileUserQuotas))
	s.mux.HandleFunc("GET /api/v1/files/folders/{folderID}/settings", s.requireAuth("files.manage", s.fileFolderManagement))
	s.mux.HandleFunc("PUT /api/v1/files/folders/{folderID}/settings", s.requireAuth("files.manage", s.fileFolderManagement))
	s.mux.HandleFunc("GET /api/v1/files/pools", s.requireAuth("files.manage", s.filePools))
	s.mux.HandleFunc("POST /api/v1/files/pools", s.requireAuth("files.manage", s.filePools))
	s.mux.HandleFunc("PATCH /api/v1/files/pools/{poolID}/capacity-policy", s.requireAuth("files.manage", s.filePoolCapacityPolicy))
	s.mux.HandleFunc("GET /api/v1/files/smb", s.requireAuth("files.manage", s.smbStatus))
	s.mux.HandleFunc("POST /api/v1/files/smb/operation", s.requireAuth("files.manage", s.smbOperation))
	s.mux.HandleFunc("GET /api/v1/files/folders", s.requireAuth("security.self.read", s.fileFolders))
	s.mux.HandleFunc("POST /api/v1/files/folders", s.requireAuth("files.manage", s.fileFolders))
	s.mux.HandleFunc("GET /api/v1/files/folders/{folderID}/entries", s.requireAuth("security.self.read", s.fileFolderEntries))
	s.mux.HandleFunc("POST /api/v1/files/folders/{folderID}/directories", s.requireAuth("security.self.read", s.fileFolderDirectory))
	s.mux.HandleFunc("GET /api/v1/files/folders/{folderID}/uploads", s.requireAuth("security.self.read", s.fileFolderUploads))
	s.mux.HandleFunc("POST /api/v1/files/folders/{folderID}/uploads", s.requireAuth("security.self.read", s.fileFolderUploads))
	s.mux.HandleFunc("GET /api/v1/files/folders/{folderID}/uploads/{uploadID}", s.requireAuth("security.self.read", s.fileFolderUpload))
	s.mux.HandleFunc("DELETE /api/v1/files/folders/{folderID}/uploads/{uploadID}", s.requireAuth("security.self.read", s.fileFolderUpload))
	s.mux.HandleFunc("PUT /api/v1/files/folders/{folderID}/uploads/{uploadID}/chunk", s.requireAuth("security.self.read", s.fileFolderUploadChunk))
	s.mux.HandleFunc("POST /api/v1/files/folders/{folderID}/uploads/{uploadID}/complete", s.requireAuth("security.self.read", s.fileFolderUploadComplete))
	s.mux.HandleFunc("GET /api/v1/files/folders/{folderID}/content", s.requireAuth("security.self.read", s.fileFolderContent))
	s.mux.HandleFunc("PUT /api/v1/files/folders/{folderID}/content", s.requireAuth("security.self.read", s.fileFolderContent))
	s.mux.HandleFunc("DELETE /api/v1/files/folders/{folderID}/entry", s.requireAuth("security.self.read", s.fileFolderEntry))
	s.mux.HandleFunc("POST /api/v1/files/folders/{folderID}/move", s.requireAuth("security.self.read", s.fileFolderMove))
	s.mux.HandleFunc("GET /api/v1/files/folders/{folderID}/trash", s.requireAuth("security.self.read", s.fileFolderTrash))
	s.mux.HandleFunc("POST /api/v1/files/folders/{folderID}/trash/{trashID}/restore", s.requireAuth("security.self.read", s.fileFolderTrashRestore))
	s.mux.HandleFunc("DELETE /api/v1/files/folders/{folderID}/trash/{trashID}", s.requireAuth("security.self.read", s.fileFolderTrashPurge))
	s.mux.HandleFunc("GET /api/v1/network/profiles", s.requireAuth("network.read", s.networkProfiles))
	s.mux.HandleFunc("GET /api/v1/network/wireguard", s.requireAuth("network.read", s.wireGuardStatus))
	s.mux.HandleFunc("POST /api/v1/network/operation", s.requireAuth("network.manage", s.networkOperation))
	s.mux.HandleFunc("/api/v1/audit", s.requireAuth(
		"audit.read",
		func(w http.ResponseWriter, r *http.Request, actor security.Actor, _ authSource) {
			s.audit(w, r, actor)
		},
	))
	s.mux.HandleFunc("/", s.notFound)

	return s.requestContext(s.mux)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.state.Ping(ctx); err != nil {
		s.logger.Error("health check failed", "component", "state", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "degraded",
			"version": version.Version,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": version.Version,
	})
}

func (s *server) systemRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	s.system(w, r)
}

func (s *server) system(w http.ResponseWriter, r *http.Request) {
	schemaVersion, err := s.state.SchemaVersion(r.Context())
	if err != nil {
		s.logger.Error("failed to read schema version", "error", err)
		writeAPIError(
			w,
			r,
			http.StatusInternalServerError,
			"state_unavailable",
			"core state is unavailable",
			nil,
		)
		return
	}

	info := systeminfo.Collect(s.nodeID)
	inspectCtx, inspectCancel := contextWithTimeout(r.Context(), 8*time.Second)
	if inspection, err := storage.Inspect(inspectCtx); err == nil {
		systeminfo.ApplyFilesystemStats(&info, inspection.Filesystems)
		systeminfo.ApplyStorageDetails(&info, inspection.DiskHealth, inspection.LVM)
	} else {
		s.logger.Debug("storage inspection unavailable", "error", err)
	}
	inspectCancel()
	if names, err := s.state.DiskNames(r.Context()); err != nil {
		s.logger.Error("failed to read disk names", "error", err)
	} else {
		systeminfo.ApplyDiskNames(&info, names)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":        version.Version,
		"schema_version": schemaVersion,
		"system":         info,
	})
}

func (s *server) eventsRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	s.events(w, r)
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	token, _ := sessionToken(r)
	var current security.Actor
	s.realtime.ServeHTTPAuthorized(w, r, requestIDFromContext(r.Context()), realtime.AccessPolicy{
		Authenticate: func(ctx context.Context) error {
			actor, err := s.security.Authenticate(ctx, token)
			if err != nil {
				return err
			}
			if !actor.Has("events.read") {
				return security.ErrUnauthorized
			}
			current = actor
			return nil
		},
		Allows: func(eventType string, data any) bool { return actorAllowsEvent(current, eventType, data) },
	})
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	writeAPIError(
		w,
		r,
		http.StatusNotFound,
		"not_found",
		"resource not found",
		nil,
	)
}

func (s *server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, meta := withRequestMetadata(r)
		w.Header().Set("X-Request-ID", meta.RequestID)
		w.Header().Set("X-Correlation-ID", meta.CorrelationID)

		started := time.Now()
		next.ServeHTTP(w, r)

		s.logger.Info("http request",
			"request_id", meta.RequestID,
			"correlation_id", meta.CorrelationID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

var contextWithTimeout = context.WithTimeout
