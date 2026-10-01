package api

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

var (
	prepareFilePool        = storage.PrepareNASPool
	prepareFileFolder      = storage.PrepareNASFolder
	resolveFilePoolStorage = func(nodeID, rootPath string, assignments []state.StoragePurposeRecord) (systeminfo.BlockNode, error) {
		return filePoolStorageNode(rootPath, assignments, systeminfo.Collect(nodeID).BlockTree)
	}
	readFilePoolCapacity         = filedata.ReadCapacity
	applyFilePoolCapacityPolicy = storage.ApplyNASCapacityPolicy
)

type filePoolResponse struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	RootPath              string `json:"root_path"`
	StorageDevicePath     string `json:"storage_device,omitempty"`
	StorageFilesystemUUID string `json:"storage_filesystem_uuid,omitempty"`
	ReservePercent        int    `json:"reserve_percent"`
	WarningPercent        int    `json:"warning_percent"`
	CapacityKnown         bool   `json:"capacity_known"`
	SizeBytes             uint64 `json:"size_bytes,omitempty"`
	FreeBytes             uint64 `json:"free_bytes,omitempty"`
	ReserveBytes          uint64 `json:"reserve_bytes,omitempty"`
	WarningBytes          uint64 `json:"warning_bytes,omitempty"`
	CapacityState         string `json:"capacity_state"`
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
			pools = append(pools, filePoolResponseFor(record))
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
		input.RootPath = filepath.Clean(strings.TrimSpace(input.RootPath))
		assignments, err := s.state.ListStoragePurposes(r.Context())
		if err != nil {
			writeAPIError(w, r, http.StatusInternalServerError, "storage_purposes_unavailable", "storage purposes are unavailable", nil)
			return
		}
		backingStorage, err := resolveFilePoolStorage(s.nodeID, input.RootPath, assignments)
		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "file_pool_storage_not_assigned", err.Error(), nil)
			return
		}
		if err := prepareFilePool(r.Context(), input.RootPath); err != nil {
			writeAPIError(w, r, http.StatusBadGateway, "file_pool_prepare_failed", err.Error(), nil)
			return
		}
		record, err := s.state.CreateNASPool(
			r.Context(),
			input.Name,
			input.RootPath,
			backingStorage.Path,
			backingStorage.UUID,
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
				map[string]any{
					"name":                    record.Name,
					"root_path":               record.RootPath,
					"storage_device":          record.StorageDevicePath,
					"storage_filesystem_uuid": record.StorageFilesystemUUID,
				},
			)
			s.realtime.Publish("files.pool.created", map[string]any{"pool_id": record.ID}, requestIDFromContext(r.Context()))
			writeJSON(w, http.StatusCreated, map[string]any{"pool": filePoolResponseFor(record)})
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
		if err == nil {
			if prepareErr := prepareFileFolder(r.Context(), record.PoolRoot, record.RelativePath); prepareErr != nil {
				if cleanupErr := s.state.DeleteNASFolder(r.Context(), record.ID); cleanupErr != nil {
					s.logger.Error("failed to clean NAS folder metadata after provisioning failure", "folder_id", record.ID, "error", cleanupErr)
				}
				writeAPIError(w, r, http.StatusBadGateway, "file_folder_prepare_failed", prepareErr.Error(), nil)
				return
			}
		}
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

const maxFileUploadBytes int64 = 512 << 20

func (s *server) fileFolderEntries(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	folder, root, ok := s.authorizedFileFolder(w, r, actor, false)
	if !ok {
		return
	}
	relative := strings.TrimSpace(r.URL.Query().Get("path"))
	entries, err := filedata.List(root, relative)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "file_path_invalid", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"folder_id": folder.ID,
		"path":      relative,
		"entries":   entries,
	})
}

func (s *server) fileFolderDirectory(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}
	var input struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_file_directory", err.Error(), nil)
		return
	}
	input.Path = strings.TrimSpace(input.Path)
	if err := filedata.CreateDirectory(root, input.Path); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "file_directory_create_failed", err.Error(), nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"files.directory.create",
		"file_folder",
		folder.ID,
		"success",
		map[string]any{"path": input.Path},
	)
	s.realtime.Publish(
		"files.directory.created",
		map[string]any{"folder_id": folder.ID, "path": input.Path},
		requestIDFromContext(r.Context()),
	)
	writeJSON(w, http.StatusCreated, map[string]any{"path": input.Path})
}

func (s *server) fileFolderContent(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	write := r.Method == http.MethodPut
	if write && source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	folder, root, ok := s.authorizedFileFolder(w, r, actor, write)
	if !ok {
		return
	}
	relative := strings.TrimSpace(r.URL.Query().Get("path"))
	if relative == "" {
		writeAPIError(w, r, http.StatusBadRequest, "file_path_required", "file path is required", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		file, info, err := filedata.OpenFile(root, relative)
		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "file_open_failed", err.Error(), nil)
			return
		}
		defer file.Close()
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	case http.MethodPut:
		allowance, ok := filePoolWriteAllowance(w, r, folder)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxFileUploadBytes)
		written, err := filedata.UploadLimited(root, relative, r.Body, allowance)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeAPIError(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "file exceeds upload limit", nil)
				return
			}
			if errors.Is(err, filedata.ErrCapacityLimit) {
				writeFilePoolReserveError(w, r, folder)
				return
			}
			writeAPIError(w, r, http.StatusBadRequest, "file_upload_failed", err.Error(), nil)
			return
		}
		s.security.RecordAudit(
			r.Context(),
			s.securityRequestContext(r),
			actor,
			"files.file.upload",
			"file_folder",
			folder.ID,
			"success",
			map[string]any{"path": relative, "size_bytes": written},
		)
		s.realtime.Publish(
			"files.file.uploaded",
			map[string]any{"folder_id": folder.ID, "path": relative, "size_bytes": written},
			requestIDFromContext(r.Context()),
		)
		writeJSON(w, http.StatusCreated, map[string]any{
			"path":       relative,
			"size_bytes": written,
		})
	default:
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *server) fileFolderUploads(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		uploads, err := filedata.ListUploads(root)
		if err != nil {
			writeAPIError(w, r, http.StatusBadGateway, "file_uploads_unavailable", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"uploads": uploads})
	case http.MethodPost:
		if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		var input struct {
			Path              string `json:"path"`
			TotalBytes        int64  `json:"total_bytes"`
			SHA256            string `json:"sha256,omitempty"`
			ClientFingerprint string `json:"client_fingerprint,omitempty"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_file_upload", err.Error(), nil)
			return
		}
		allowance, ok := filePoolWriteAllowance(w, r, folder)
		if !ok {
			return
		}
		if input.TotalBytes > allowance {
			writeFilePoolReserveError(w, r, folder)
			return
		}
		session, err := filedata.CreateUpload(
			root,
			strings.TrimSpace(input.Path),
			input.TotalBytes,
			input.SHA256,
			input.ClientFingerprint,
			time.Now().UTC(),
		)
		switch {
		case errors.Is(err, filedata.ErrUploadTargetExists), errors.Is(err, filedata.ErrUploadAlreadyActive):
			writeAPIError(w, r, http.StatusConflict, "file_upload_conflict", err.Error(), nil)
		case err != nil:
			writeAPIError(w, r, http.StatusBadRequest, "file_upload_create_failed", err.Error(), nil)
		default:
			s.security.RecordAudit(
				r.Context(),
				s.securityRequestContext(r),
				actor,
				"files.upload.create",
				"file_folder",
				folder.ID,
				"success",
				map[string]any{
					"upload_id":   session.ID,
					"path":        session.Path,
					"total_bytes": session.TotalBytes,
				},
			)
			writeJSON(w, http.StatusCreated, map[string]any{"upload": session})
		}
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *server) fileFolderUpload(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}
	uploadID := strings.TrimSpace(r.PathValue("uploadID"))

	switch r.Method {
	case http.MethodGet:
		session, err := filedata.GetUpload(root, uploadID)
		if errors.Is(err, filedata.ErrUploadNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "file_upload_not_found", "upload session not found", nil)
			return
		}
		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "file_upload_unavailable", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"upload": session})
	case http.MethodDelete:
		if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
			writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		if err := filedata.CancelUpload(root, uploadID); err != nil {
			if errors.Is(err, filedata.ErrUploadNotFound) {
				writeAPIError(w, r, http.StatusNotFound, "file_upload_not_found", "upload session not found", nil)
				return
			}
			writeAPIError(w, r, http.StatusBadRequest, "file_upload_cancel_failed", err.Error(), nil)
			return
		}
		s.security.RecordAudit(
			r.Context(),
			s.securityRequestContext(r),
			actor,
			"files.upload.cancel",
			"file_folder",
			folder.ID,
			"success",
			map[string]any{"upload_id": uploadID},
		)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodDelete)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *server) fileFolderUploadChunk(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}
	allowance, ok := filePoolWriteAllowance(w, r, folder)
	if !ok {
		return
	}
	if allowance <= 0 {
		writeFilePoolReserveError(w, r, folder)
		return
	}
	chunkLimit := filedata.MaxUploadChunkBytes
	if allowance < chunkLimit {
		chunkLimit = allowance
	}

	offset, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get("Upload-Offset")), 10, 64)
	if err != nil || offset < 0 {
		writeAPIError(w, r, http.StatusBadRequest, "file_upload_offset_invalid", "valid Upload-Offset header is required", nil)
		return
	}
	session, err := filedata.AppendUploadChunk(
		root,
		strings.TrimSpace(r.PathValue("uploadID")),
		offset,
		r.Header.Get("X-Chunk-SHA256"),
		r.Body,
		chunkLimit,
		time.Now().UTC(),
	)
	switch {
	case errors.Is(err, filedata.ErrUploadNotFound):
		writeAPIError(w, r, http.StatusNotFound, "file_upload_not_found", "upload session not found", nil)
	case errors.Is(err, filedata.ErrUploadOffsetMismatch):
		writeAPIError(w, r, http.StatusConflict, "file_upload_offset_mismatch", err.Error(), nil)
	case errors.Is(err, filedata.ErrUploadChecksumMismatch):
		writeAPIError(w, r, http.StatusUnprocessableEntity, "file_upload_checksum_mismatch", err.Error(), nil)
	case errors.Is(err, filedata.ErrUploadChunkTooLarge) && chunkLimit < filedata.MaxUploadChunkBytes:
		writeFilePoolReserveError(w, r, folder)
	case errors.Is(err, filedata.ErrUploadChunkTooLarge):
		writeAPIError(w, r, http.StatusRequestEntityTooLarge, "file_upload_chunk_too_large", err.Error(), nil)
	case err != nil:
		writeAPIError(w, r, http.StatusBadRequest, "file_upload_chunk_failed", err.Error(), nil)
	default:
		w.Header().Set("Upload-Offset", strconv.FormatInt(session.ReceivedBytes, 10))
		writeJSON(w, http.StatusOK, map[string]any{"upload": session})
	}
}

func (s *server) fileFolderUploadComplete(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}
	uploadID := strings.TrimSpace(r.PathValue("uploadID"))
	result, err := filedata.CompleteUpload(root, uploadID)
	switch {
	case errors.Is(err, filedata.ErrUploadNotFound):
		writeAPIError(w, r, http.StatusNotFound, "file_upload_not_found", "upload session not found", nil)
	case errors.Is(err, filedata.ErrUploadChecksumMismatch):
		writeAPIError(w, r, http.StatusUnprocessableEntity, "file_upload_checksum_mismatch", err.Error(), nil)
	case errors.Is(err, filedata.ErrUploadTargetExists):
		writeAPIError(w, r, http.StatusConflict, "file_upload_target_exists", err.Error(), nil)
	case err != nil:
		writeAPIError(w, r, http.StatusBadRequest, "file_upload_complete_failed", err.Error(), nil)
	default:
		s.security.RecordAudit(
			r.Context(),
			s.securityRequestContext(r),
			actor,
			"files.file.upload.complete",
			"file_folder",
			folder.ID,
			"success",
			map[string]any{
				"upload_id":  uploadID,
				"path":       result.Path,
				"size_bytes": result.SizeBytes,
				"sha256":     result.SHA256,
			},
		)
		s.realtime.Publish(
			"files.file.uploaded",
			map[string]any{
				"folder_id":  folder.ID,
				"path":       result.Path,
				"size_bytes": result.SizeBytes,
				"sha256":     result.SHA256,
			},
			requestIDFromContext(r.Context()),
		)
		writeJSON(w, http.StatusOK, map[string]any{"file": result})
	}
}

func (s *server) fileFolderEntry(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}
	relative := strings.TrimSpace(r.URL.Query().Get("path"))
	if relative == "" {
		writeAPIError(w, r, http.StatusBadRequest, "file_path_required", "file path is required", nil)
		return
	}
	entry, err := filedata.Trash(root, relative, time.Now().UTC())
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "file_trash_failed", err.Error(), nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"files.entry.trash",
		"file_folder",
		folder.ID,
		"success",
		map[string]any{"path": relative, "trash_id": entry.ID},
	)
	s.realtime.Publish(
		"files.entry.trashed",
		map[string]any{"folder_id": folder.ID, "path": relative, "trash_id": entry.ID},
		requestIDFromContext(r.Context()),
	)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) fileFolderMove(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}
	var input struct {
		FromPath string `json:"from_path"`
		ToPath   string `json:"to_path"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_file_move", err.Error(), nil)
		return
	}
	input.FromPath = strings.TrimSpace(input.FromPath)
	input.ToPath = strings.TrimSpace(input.ToPath)
	if err := filedata.Move(root, input.FromPath, input.ToPath); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "file_move_failed", err.Error(), nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"files.entry.move",
		"file_folder",
		folder.ID,
		"success",
		map[string]any{
			"from_path": input.FromPath,
			"to_path":   input.ToPath,
		},
	)
	s.realtime.Publish(
		"files.entry.moved",
		map[string]any{
			"folder_id": folder.ID,
			"from_path": input.FromPath,
			"to_path":   input.ToPath,
		},
		requestIDFromContext(r.Context()),
	)
	writeJSON(w, http.StatusOK, map[string]any{"path": input.ToPath})
}

func (s *server) fileFolderTrash(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	_, root, ok := s.authorizedFileFolder(w, r, actor, false)
	if !ok {
		return
	}
	entries, err := filedata.ListTrash(root)
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "file_trash_unavailable", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"trash": entries})
}

func (s *server) fileFolderTrashRestore(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}
	trashID := strings.TrimSpace(r.PathValue("trashID"))
	entry, err := filedata.RestoreTrash(root, trashID)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "file_restore_failed", err.Error(), nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"files.trash.restore",
		"file_folder",
		folder.ID,
		"success",
		map[string]any{"trash_id": trashID, "path": entry.OriginalPath},
	)
	s.realtime.Publish(
		"files.trash.restored",
		map[string]any{"folder_id": folder.ID, "trash_id": trashID, "path": entry.OriginalPath},
		requestIDFromContext(r.Context()),
	)
	writeJSON(w, http.StatusOK, map[string]any{"path": entry.OriginalPath})
}

func (s *server) fileFolderTrashPurge(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	folder, root, ok := s.authorizedFileFolder(w, r, actor, true)
	if !ok {
		return
	}
	trashID := strings.TrimSpace(r.PathValue("trashID"))
	entry, err := filedata.PurgeTrash(root, trashID)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "file_trash_purge_failed", err.Error(), nil)
		return
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"files.trash.purge",
		"file_folder",
		folder.ID,
		"success",
		map[string]any{"trash_id": trashID, "path": entry.OriginalPath},
	)
	s.realtime.Publish(
		"files.trash.purged",
		map[string]any{"folder_id": folder.ID, "trash_id": trashID},
		requestIDFromContext(r.Context()),
	)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) authorizedFileFolder(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	write bool,
) (state.NASFolderRecord, string, bool) {
	folderID := strings.TrimSpace(r.PathValue("folderID"))
	if folderID == "" {
		writeAPIError(w, r, http.StatusBadRequest, "file_folder_required", "file folder is required", nil)
		return state.NASFolderRecord{}, "", false
	}
	folder, err := s.state.NASFolder(r.Context(), folderID)
	if errors.Is(err, state.ErrNASFolderNotFound) {
		writeAPIError(w, r, http.StatusNotFound, "file_folder_not_found", "file folder not found", nil)
		return state.NASFolderRecord{}, "", false
	}
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "file_folder_unavailable", "file folder is unavailable", nil)
		return state.NASFolderRecord{}, "", false
	}

	permission := "files.read"
	if write {
		permission = "files.write"
	}
	if !actor.Has("files.manage") && !actor.Allows(permission, "file_folder", folder.ID) {
		writeAPIError(
			w,
			r,
			http.StatusForbidden,
			"permission_denied",
			fmt.Sprintf("%s permission is required for this folder", permission),
			nil,
		)
		return state.NASFolderRecord{}, "", false
	}

	root, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "file_folder_storage_unavailable", err.Error(), nil)
		return state.NASFolderRecord{}, "", false
	}
	return folder, root, true
}

func filePoolResponseFor(record state.NASPoolRecord) filePoolResponse {
	response := filePoolResponse{
		ID:                    record.ID,
		Name:                  record.Name,
		RootPath:              record.RootPath,
		StorageDevicePath:     record.StorageDevicePath,
		StorageFilesystemUUID: record.StorageFilesystemUUID,
		ReservePercent:        record.ReservePercent,
		WarningPercent:        record.WarningPercent,
		CapacityState:         "unknown",
	}
	capacity, err := readFilePoolCapacity(record.RootPath)
	if err != nil {
		return response
	}
	response.CapacityKnown = true
	response.SizeBytes = capacity.TotalBytes
	response.FreeBytes = capacity.FreeBytes
	response.ReserveBytes = capacityPercentBytes(capacity.TotalBytes, record.ReservePercent)
	response.WarningBytes = capacityPercentBytes(capacity.TotalBytes, record.WarningPercent)
	switch {
	case record.ReservePercent > 0 && capacity.FreeBytes <= response.ReserveBytes:
		response.CapacityState = "reserve"
	case record.WarningPercent > 0 && capacity.FreeBytes <= response.WarningBytes:
		response.CapacityState = "warning"
	default:
		response.CapacityState = "ok"
	}
	return response
}

func capacityPercentBytes(total uint64, percent int) uint64 {
	if total == 0 || percent <= 0 {
		return 0
	}
	value := uint64(percent)
	return (total/100)*value + ((total%100)*value)/100
}

func filePoolWriteAllowance(
	w http.ResponseWriter,
	r *http.Request,
	folder state.NASFolderRecord,
) (int64, bool) {
	capacity, err := readFilePoolCapacity(folder.PoolRoot)
	if err != nil {
		writeAPIError(
			w,
			r,
			http.StatusServiceUnavailable,
			"file_pool_capacity_unavailable",
			"file pool capacity is unavailable",
			map[string]any{"pool_id": folder.PoolID},
		)
		return 0, false
	}
	reserveBytes := capacityPercentBytes(capacity.TotalBytes, folder.PoolReservePercent)
	if capacity.FreeBytes <= reserveBytes {
		writeFilePoolReserveErrorWithCapacity(w, r, folder, capacity)
		return 0, false
	}
	available := capacity.FreeBytes - reserveBytes
	if available > uint64(math.MaxInt64) {
		return math.MaxInt64, true
	}
	return int64(available), true
}

func writeFilePoolReserveError(
	w http.ResponseWriter,
	r *http.Request,
	folder state.NASFolderRecord,
) {
	capacity, err := readFilePoolCapacity(folder.PoolRoot)
	if err != nil {
		writeAPIError(
			w,
			r,
			http.StatusInsufficientStorage,
			"file_pool_reserve_reached",
			"file pool free-space reserve would be violated",
			map[string]any{"pool_id": folder.PoolID, "reserve_percent": folder.PoolReservePercent},
		)
		return
	}
	writeFilePoolReserveErrorWithCapacity(w, r, folder, capacity)
}

func writeFilePoolReserveErrorWithCapacity(
	w http.ResponseWriter,
	r *http.Request,
	folder state.NASFolderRecord,
	capacity filedata.Capacity,
) {
	writeAPIError(
		w,
		r,
		http.StatusInsufficientStorage,
		"file_pool_reserve_reached",
		"file pool free-space reserve would be violated",
		map[string]any{
			"pool_id":         folder.PoolID,
			"pool_name":       folder.PoolName,
			"free_bytes":      capacity.FreeBytes,
			"reserve_bytes":   capacityPercentBytes(capacity.TotalBytes, folder.PoolReservePercent),
			"reserve_percent": folder.PoolReservePercent,
		},
	)
}

func (s *server) filePoolCapacityPolicy(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", http.MethodPatch)
		writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	var input struct {
		ReservePercent int `json:"reserve_percent"`
		WarningPercent int `json:"warning_percent"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_file_pool_policy", err.Error(), nil)
		return
	}
	record, err := s.state.UpdateNASPoolCapacityPolicy(
		r.Context(),
		strings.TrimSpace(r.PathValue("poolID")),
		input.ReservePercent,
		input.WarningPercent,
		time.Now().UTC(),
	)
	switch {
	case errors.Is(err, state.ErrNASPoolNotFound):
		writeAPIError(w, r, http.StatusNotFound, "file_pool_not_found", "file pool not found", nil)
	case err != nil:
		writeAPIError(w, r, http.StatusBadRequest, "file_pool_policy_update_failed", err.Error(), nil)
	default:
		hardQuotaError := ""
		if syncErr := applyFilePoolCapacityPolicy(r.Context(), record.RootPath, record.ReservePercent); syncErr != nil {
			hardQuotaError = syncErr.Error()
			s.logger.Warn(
				"NAS hard quota policy is not synchronized",
				"pool_id", record.ID,
				"root_path", record.RootPath,
				"reserve_percent", record.ReservePercent,
				"error", syncErr,
			)
		}
		s.security.RecordAudit(
			r.Context(),
			s.securityRequestContext(r),
			actor,
			"files.pool.capacity_policy.update",
			"file_pool",
			record.ID,
			"success",
			map[string]any{
				"reserve_percent": record.ReservePercent,
				"warning_percent":  record.WarningPercent,
				"hard_quota_error": hardQuotaError,
			},
		)
		s.realtime.Publish(
			"files.pool.capacity_policy.changed",
			map[string]any{"pool_id": record.ID},
			requestIDFromContext(r.Context()),
		)
		writeJSON(w, http.StatusOK, map[string]any{"pool": filePoolResponseFor(record)})
	}
}

func filePoolStorageNode(
	rootPath string,
	assignments []state.StoragePurposeRecord,
	nodes []systeminfo.BlockNode,
) (systeminfo.BlockNode, error) {
	rootPath = filepath.Clean(strings.TrimSpace(rootPath))
	if rootPath == "." || !filepath.IsAbs(rootPath) {
		return systeminfo.BlockNode{}, errors.New("file pool root path must be an absolute mounted path")
	}
	for _, assignment := range assignments {
		if assignment.Purpose != state.StoragePurposeFiles {
			continue
		}
		node, ok := blockNodeForStoragePurpose(nodes, assignment)
		if !ok || node.System || node.Filesystem == "" {
			continue
		}
		for _, mountpoint := range node.Mountpoints {
			if filepath.Clean(strings.TrimSpace(mountpoint)) == rootPath {
				return node, nil
			}
		}
	}
	return systeminfo.BlockNode{}, errors.New("file pool root must be a mounted storage device explicitly assigned to Files")
}
