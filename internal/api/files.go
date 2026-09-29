package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type filePoolResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RootPath string `json:"root_path"`
}

type fileFolderResponse struct {
	ID           string `json:"id"`
	PoolID       string `json:"pool_id"`
	PoolName     string `json:"pool_name"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	OwnerUserID  string `json:"owner_user_id,omitempty"`
	RelativePath string `json:"relative_path"`
	CanRead      bool   `json:"can_read"`
	CanWrite     bool   `json:"can_write"`
}

func (s *server) filePools(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	switch r.Method {
	case http.MethodGet:
		records, err := s.state.ListNASPools(r.Context())
		if err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "file_pools_unavailable", "file pools are unavailable", nil)
			return
		}
		pools := make([]filePoolResponse, 0, len(records))
		for _, record := range records {
			pools = append(pools, filePoolResponse{
				ID:       record.ID,
				Name:     record.Name,
				RootPath: record.RootPath,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"pools": pools})
	case http.MethodPost:
		if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		var input struct {
			Name     string `json:"name"`
			RootPath string `json:"root_path"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_file_pool", err.Error(), nil)
			return
		}
		record, err := s.state.CreateNASPool(
			r.Context(),
			input.Name,
			input.RootPath,
			actor.ID,
			time.Now().UTC(),
		)
		switch {
		case errors.Is(err, state.ErrNASPoolExists):
			writeAPIError(w, r, http.StatusConflict, "file_pool_exists", "file pool already exists", nil)
		case err != nil:
			writeAPIError(w, r, http.StatusBadRequest, "file_pool_create_failed", err.Error(), nil)
		default:
			s.security.RecordAudit(
				r.Context(),
				s.securityRequestContext(r),
				actor,
				"files.pool.create",
				"file_pool",
				record.ID,
				"success",
				map[string]any{"name": record.Name, "root_path": record.RootPath},
			)
			s.realtime.Publish("files.pool.created", map[string]any{"pool_id": record.ID}, requestIDFromContext(r.Context()))
			writeJSON(w, http.StatusCreated, map[string]any{"pool": filePoolResponse{
				ID:       record.ID,
				Name:     record.Name,
				RootPath: record.RootPath,
			}})
		}
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *server) fileFolders(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	switch r.Method {
	case http.MethodGet:
		records, err := s.state.ListNASFolders(r.Context())
		if err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "file_folders_unavailable", "file folders are unavailable", nil)
			return
		}
		folders := make([]fileFolderResponse, 0, len(records))
		for _, record := range records {
			canRead := actor.Has("files.manage") || actor.Allows("files.read", "file_folder", record.ID)
			if !canRead {
				continue
			}
			folders = append(folders, fileFolderResponse{
				ID:           record.ID,
				PoolID:       record.PoolID,
				PoolName:     record.PoolName,
				Name:         record.Name,
				Kind:         record.Kind,
				OwnerUserID:  record.OwnerUserID,
				RelativePath: record.RelativePath,
				CanRead:      true,
				CanWrite:     actor.Has("files.manage") || actor.Allows("files.write", "file_folder", record.ID),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"folders": folders})
	case http.MethodPost:
		if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		var input struct {
			PoolID      string `json:"pool_id"`
			Name        string `json:"name"`
			Kind        string `json:"kind"`
			OwnerUserID string `json:"owner_user_id,omitempty"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_file_folder", err.Error(), nil)
			return
		}
		record, err := s.state.CreateNASFolder(
			r.Context(),
			input.PoolID,
			input.Name,
			input.Kind,
			input.OwnerUserID,
			actor.ID,
			time.Now().UTC(),
		)
		switch {
		case errors.Is(err, state.ErrNASPoolNotFound):
			writeAPIError(w, r, http.StatusNotFound, "file_pool_not_found", "file pool not found", nil)
		case errors.Is(err, state.ErrNASFolderExists):
			writeAPIError(w, r, http.StatusConflict, "file_folder_exists", "file folder already exists", nil)
		case err != nil:
			writeAPIError(w, r, http.StatusBadRequest, "file_folder_create_failed", err.Error(), nil)
		default:
			s.security.RecordAudit(
				r.Context(),
				s.securityRequestContext(r),
				actor,
				"files.folder.create",
				"file_folder",
				record.ID,
				"success",
				map[string]any{
					"pool_id":       record.PoolID,
					"kind":          record.Kind,
					"owner_user_id": record.OwnerUserID,
				},
			)
			s.realtime.Publish("files.folder.created", map[string]any{"folder_id": record.ID}, requestIDFromContext(r.Context()))
			writeJSON(w, http.StatusCreated, map[string]any{"folder": fileFolderResponse{
				ID:           record.ID,
				PoolID:       record.PoolID,
				PoolName:     record.PoolName,
				Name:         record.Name,
				Kind:         record.Kind,
				OwnerUserID:  record.OwnerUserID,
				RelativePath: record.RelativePath,
				CanRead:      true,
				CanWrite:     true,
			}})
		}
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}
