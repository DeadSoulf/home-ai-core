package api

import (
	"database/sql"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"strings"

	managedfiles "github.com/DeadSoulf/home-ai-core/internal/files"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func (s *server) fileEntries(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	folder, ok := s.authorizedFileFolder(w, r, actor, r.URL.Query().Get("folder_id"), "files.read")
	if !ok {
		return
	}
	entries, err := managedfiles.List(folder, r.URL.Query().Get("path"))
	if err != nil {
		writeFileOperationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"folder_id": folder.ID,
		"path":      strings.TrimSpace(r.URL.Query().Get("path")),
		"entries":   entries,
	})
}

func (s *server) fileDownload(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	folder, ok := s.authorizedFileFolder(w, r, actor, r.URL.Query().Get("folder_id"), "files.read")
	if !ok {
		return
	}
	file, info, closeFn, err := managedfiles.OpenDownloadWithClose(folder, r.URL.Query().Get("path"))
	if err != nil {
		writeFileOperationError(w, r, err)
		return
	}
	defer closeFn()

	if disposition := mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func (s *server) fileOperation(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}

	var input struct {
		FolderID    string `json:"folder_id"`
		Operation   string `json:"operation"`
		Path        string `json:"path"`
		Destination string `json:"destination,omitempty"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_file_operation", err.Error(), nil)
		return
	}
	input.FolderID = strings.TrimSpace(input.FolderID)
	input.Operation = strings.ToLower(strings.TrimSpace(input.Operation))

	folder, ok := s.authorizedFileFolder(w, r, actor, input.FolderID, "files.write")
	if !ok {
		return
	}

	var err error
	switch input.Operation {
	case "mkdir":
		err = managedfiles.CreateDirectory(folder, input.Path)
	case "delete":
		err = managedfiles.Delete(folder, input.Path)
	case "move":
		err = managedfiles.Move(folder, input.Path, input.Destination)
	default:
		writeAPIError(w, r, http.StatusBadRequest, "unsupported_file_operation", "supported operations are mkdir, delete and move", nil)
		return
	}
	if err != nil {
		writeFileOperationError(w, r, err)
		return
	}

	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"files."+input.Operation,
		"file_folder",
		folder.ID,
		"success",
		map[string]any{
			"path":        input.Path,
			"destination": input.Destination,
		},
	)
	s.realtime.Publish(
		"files.changed",
		map[string]any{"folder_id": folder.ID, "operation": input.Operation},
		requestIDFromContext(r.Context()),
	)
	writeJSON(w, http.StatusOK, map[string]any{"message": "file operation completed"})
}

func (s *server) authorizedFileFolder(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	folderID, permission string,
) (state.NASFolderRecord, bool) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		writeAPIError(w, r, http.StatusBadRequest, "file_folder_required", "folder_id is required", nil)
		return state.NASFolderRecord{}, false
	}
	folder, err := s.state.NASFolder(r.Context(), folderID)
	if errors.Is(err, sql.ErrNoRows) {
		writeAPIError(w, r, http.StatusNotFound, "file_folder_not_found", "file folder not found", nil)
		return state.NASFolderRecord{}, false
	}
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "file_folder_unavailable", "file folder is unavailable", nil)
		return state.NASFolderRecord{}, false
	}
	if !actor.Has("files.manage") && !actor.Allows(permission, "file_folder", folder.ID) {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "permission denied", nil)
		return state.NASFolderRecord{}, false
	}
	return folder, true
}

func writeFileOperationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, managedfiles.ErrInvalidPath),
		errors.Is(err, managedfiles.ErrRootMutation):
		writeAPIError(w, r, http.StatusBadRequest, "invalid_file_path", err.Error(), nil)
	case errors.Is(err, managedfiles.ErrNotDirectory),
		errors.Is(err, managedfiles.ErrNotRegular):
		writeAPIError(w, r, http.StatusBadRequest, "invalid_file_type", err.Error(), nil)
	case errors.Is(err, managedfiles.ErrDestinationExists):
		writeAPIError(w, r, http.StatusConflict, "file_destination_exists", err.Error(), nil)
	case errors.Is(err, fs.ErrNotExist):
		writeAPIError(w, r, http.StatusNotFound, "file_not_found", "file or directory not found", nil)
	default:
		writeAPIError(w, r, http.StatusInternalServerError, "file_operation_failed", err.Error(), nil)
	}
}
