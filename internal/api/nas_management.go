package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
)

// Serialize quota reservations and writes across pools: one user's private
// quota can span several physical filesystems.
var fileMutationMu sync.Mutex

type nasManagementState interface {
	NASSetting(context.Context, string) (string, error)
	SetNASSetting(context.Context, string, string) error
	NASUserQuotas(context.Context) (map[string]int64, error)
	SetNASUserQuota(context.Context, string, int64, time.Time) error
	NASFolderAccess(context.Context, string) ([]state.NASFolderAccess, error)
	UpdateNASFolderManagement(context.Context, string, string, int64, int64, []state.NASFolderAccess, time.Time) error
}

func (s *server) fileOwners(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	users, err := s.security.ListUsers(r.Context())
	if err != nil {
		writeAPIError(w, r, 500, "file_owners_unavailable", "file owners unavailable", nil)
		return
	}
	owners := []map[string]any{}
	for _, user := range users {
		if !user.Disabled {
			owners = append(owners, map[string]any{"id": user.ID, "username": user.Username, "display_name": user.DisplayName})
		}
	}
	writeJSON(w, 200, map[string]any{"users": owners})
}

func (s *server) fileFolderManagement(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	store, ok := s.state.(nasManagementState)
	if !ok {
		writeAPIError(w, r, 503, "file_management_unavailable", "folder management unavailable", nil)
		return
	}
	fileMutationMu.Lock()
	defer fileMutationMu.Unlock()
	currentActor, authorized := s.refreshMutationActor(w, r, "files.manage")
	if !authorized {
		return
	}
	actor = currentActor
	folder, err := s.state.NASFolder(r.Context(), r.PathValue("folderID"))
	if err != nil {
		writeAPIError(w, r, 404, "file_folder_not_found", "file folder not found", nil)
		return
	}
	if r.Method == http.MethodGet {
		grants, err := store.NASFolderAccess(r.Context(), folder.ID)
		if err != nil {
			writeAPIError(w, r, 500, "file_access_unavailable", err.Error(), nil)
			return
		}
		writeJSON(w, 200, map[string]any{"folder": s.folderView(r.Context(), folder, actor), "access": grants})
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, 403, "csrf_required", "valid CSRF token required", nil)
		return
	}
	var input struct {
		Name       string                  `json:"name"`
		QuotaBytes int64                   `json:"quota_bytes"`
		EnforceSMB bool                    `json:"enforce_smb"`
		Access     []state.NASFolderAccess `json:"access"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, 400, "invalid_file_folder", err.Error(), nil)
		return
	}
	if input.QuotaBytes < 0 || input.QuotaBytes > 1<<53-1 {
		writeAPIError(w, r, 400, "invalid_quota", "quota must be between 0 and 9007199254740991 bytes", nil)
		return
	}
	if input.EnforceSMB && input.QuotaBytes < 1<<20 {
		writeAPIError(w, r, 400, "hard_quota_required", "SMB enforcement requires a folder quota of at least 1 MiB", nil)
		return
	}
	root, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
	if err != nil {
		writeAPIError(w, r, 503, "file_folder_unavailable", err.Error(), nil)
		return
	}
	usage, err := s.folderUsage(r.Context(), folder)
	if err != nil {
		writeAPIError(w, r, 503, "file_usage_unavailable", err.Error(), nil)
		return
	}
	if input.QuotaBytes > 0 && usage.UsedBytes+usage.ReservedBytes > input.QuotaBytes {
		writeAPIError(w, r, 409, "quota_below_usage", "quota is below used and reserved space", nil)
		return
	}
	hardQuota := int64(0)
	active, err := s.beginSMBAccessChange(r)
	if err != nil {
		writeAPIError(w, r, 502, "smb_access_suspend_failed", err.Error(), nil)
		return
	}
	// Fail closed: a failed mutation leaves shares suspended until a successful
	// explicit Apply or subsequent edit rebuilds authoritative grants.
	if input.EnforceSMB {
		if err := applyFolderHardQuota(r.Context(), folder, input.QuotaBytes); err != nil {
			writeAPIError(w, r, 502, "file_hard_quota_unavailable", err.Error(), nil)
			return
		}
		hardQuota = input.QuotaBytes
	} else if folder.HardQuotaBytes > 0 {
		if err := applyFolderHardQuota(r.Context(), folder, 0); err != nil {
			writeAPIError(w, r, 502, "file_hard_quota_unavailable", err.Error(), nil)
			return
		}
	}
	if err := store.UpdateNASFolderManagement(r.Context(), folder.ID, input.Name, input.QuotaBytes, hardQuota, input.Access, time.Now().UTC()); err != nil {
		writeAPIError(w, r, 400, "file_folder_update_failed", err.Error(), nil)
		return
	}
	warning := s.finishSMBAccessChange(r, active)
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor, "files.folder.update", "file_folder", folder.ID, "success", map[string]any{"quota_bytes": input.QuotaBytes, "hard_quota_bytes": hardQuota})
	s.realtime.Publish("files.folder.changed", map[string]any{"folder_id": folder.ID}, requestIDFromContext(r.Context()))
	writeJSON(w, 200, map[string]any{"warning": warning})
}

func (s *server) fileUserQuotas(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	store, ok := s.state.(nasManagementState)
	if !ok {
		writeAPIError(w, r, 503, "file_quotas_unavailable", "quotas unavailable", nil)
		return
	}
	fileMutationMu.Lock()
	defer fileMutationMu.Unlock()
	currentActor, authorized := s.refreshMutationActor(w, r, "files.manage")
	if !authorized {
		return
	}
	actor = currentActor
	quotas, err := store.NASUserQuotas(r.Context())
	if err != nil {
		writeAPIError(w, r, 500, "file_quotas_unavailable", err.Error(), nil)
		return
	}
	folders, err := s.state.ListNASFolders(r.Context())
	if err != nil {
		writeAPIError(w, r, 500, "file_folders_unavailable", err.Error(), nil)
		return
	}
	if r.Method == http.MethodPut {
		if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
			writeAPIError(w, r, 403, "csrf_required", "valid CSRF token required", nil)
			return
		}
		var input struct {
			QuotaBytes int64 `json:"quota_bytes"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, r, 400, "invalid_quota", err.Error(), nil)
			return
		}
		if input.QuotaBytes < 0 || input.QuotaBytes > 1<<53-1 {
			writeAPIError(w, r, 400, "invalid_quota", "invalid quota", nil)
			return
		}
		id := r.PathValue("userID")
		usage, err := s.privateUserUsage(r.Context(), folders, id)
		if err != nil {
			writeAPIError(w, r, 503, "file_usage_unavailable", err.Error(), nil)
			return
		}
		if input.QuotaBytes > 0 && usage.UsedBytes+usage.ReservedBytes > input.QuotaBytes {
			writeAPIError(w, r, 409, "quota_below_usage", "quota is below used and reserved space", nil)
			return
		}
		active, err := s.beginSMBAccessChange(r)
		if err != nil {
			writeAPIError(w, r, 502, "smb_access_suspend_failed", err.Error(), nil)
			return
		}
		if err := store.SetNASUserQuota(r.Context(), id, input.QuotaBytes, time.Now().UTC()); err != nil {
			writeAPIError(w, r, 400, "user_quota_update_failed", err.Error(), nil)
			return
		}
		warning := s.finishSMBAccessChange(r, active)
		s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor, "files.user.quota.update", "user", id, "success", map[string]any{"quota_bytes": input.QuotaBytes})
		writeJSON(w, 200, map[string]any{"warning": warning})
		return
	}
	users, err := s.security.ListUsers(r.Context())
	if err != nil {
		writeAPIError(w, r, 500, "file_owners_unavailable", err.Error(), nil)
		return
	}
	result := []map[string]any{}
	for _, user := range users {
		usage, usageErr := s.privateUserUsage(r.Context(), folders, user.ID)
		result = append(result, map[string]any{"user_id": user.ID, "username": user.Username, "display_name": user.DisplayName, "quota_bytes": quotas[user.ID], "used_bytes": usage.UsedBytes, "reserved_bytes": usage.ReservedBytes, "usage_known": usageErr == nil})
	}
	writeJSON(w, 200, map[string]any{"quotas": result})
}

func (s *server) folderUsage(ctx context.Context, folder state.NASFolderRecord) (filedata.Usage, error) {
	root, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
	if err != nil {
		return filedata.Usage{}, err
	}
	if usage, err := filedata.UsageOf(root); err == nil {
		return usage, nil
	}
	usage, err := inspectNASFolderUsage(ctx, folder.PoolRoot, folder.RelativePath)
	if err != nil {
		return filedata.Usage{}, err
	}
	return filedata.Usage{UsedBytes: usage.UsedBytes, ReservedBytes: usage.ReservedBytes}, nil
}

func (s *server) privateUserUsage(ctx context.Context, folders []state.NASFolderRecord, userID string) (filedata.Usage, error) {
	var total filedata.Usage
	for _, folder := range folders {
		if folder.Kind != "private" || folder.OwnerUserID != userID {
			continue
		}
		usage, err := s.folderUsage(ctx, folder)
		if err != nil {
			return total, err
		}
		if usage.UsedBytes > math.MaxInt64-total.UsedBytes || usage.ReservedBytes > math.MaxInt64-total.ReservedBytes {
			return total, errors.New("user usage overflows")
		}
		total.UsedBytes += usage.UsedBytes
		total.ReservedBytes += usage.ReservedBytes
	}
	return total, nil
}

func (s *server) folderView(ctx context.Context, folder state.NASFolderRecord, actor security.Actor) fileFolderResponse {
	response := fileFolderResponse{ID: folder.ID, PoolID: folder.PoolID, PoolName: folder.PoolName, Name: folder.Name, Kind: folder.Kind, OwnerUserID: folder.OwnerUserID, RelativePath: folder.RelativePath, CanRead: true, CanWrite: actor.Has("files.manage") || actor.Allows("files.write", "file_folder", folder.ID), QuotaBytes: folder.QuotaBytes, HardQuotaBytes: folder.HardQuotaBytes}
	if usage, err := s.folderUsage(ctx, folder); err == nil {
		response.UsageKnown = true
		response.UsedBytes = usage.UsedBytes
		response.ReservedBytes = usage.ReservedBytes
	}
	return response
}

func (s *server) fileWriteAllowance(w http.ResponseWriter, r *http.Request, folder state.NASFolderRecord, uploadID string) (int64, bool) {
	allowance, ok := s.filePoolWriteAllowance(w, r, folder)
	if !ok {
		return 0, false
	}
	store, supported := s.state.(nasManagementState)
	if !supported {
		return allowance, true
	}
	folders, err := s.state.ListNASFolders(r.Context())
	if err != nil {
		writeAPIError(w, r, 503, "file_usage_unavailable", err.Error(), nil)
		return 0, false
	}
	allowance, err = reservedPoolAllowance(r.Context(), folders, folder, uploadID, allowance)
	if err != nil {
		writeAPIError(w, r, 503, "file_capacity_reservations_unavailable", err.Error(), nil)
		return 0, false
	}
	quotas, err := store.NASUserQuotas(r.Context())
	if err != nil {
		writeAPIError(w, r, 503, "file_quotas_unavailable", err.Error(), nil)
		return 0, false
	}
	userQuota := quotas[folder.OwnerUserID]
	if folder.Kind != "private" {
		userQuota = 0
	}
	if folder.QuotaBytes == 0 && userQuota == 0 {
		return allowance, true
	}
	root, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
	if err != nil {
		writeAPIError(w, r, 503, "file_usage_unavailable", err.Error(), nil)
		return 0, false
	}
	usage, err := s.folderUsage(r.Context(), folder)
	if err != nil {
		writeAPIError(w, r, 503, "file_usage_unavailable", err.Error(), nil)
		return 0, false
	}
	credit := int64(0)
	if uploadID != "" {
		upload, err := filedata.GetUpload(root, uploadID)
		if err != nil {
			writeAPIError(w, r, 400, "file_upload_unavailable", err.Error(), nil)
			return 0, false
		}
		credit = upload.TotalBytes - upload.ReceivedBytes
	}
	if folder.QuotaBytes > 0 {
		allowance = min(allowance, max(int64(0), folder.QuotaBytes-usage.UsedBytes-usage.ReservedBytes+credit))
	}
	if userQuota > 0 {
		total, err := s.privateUserUsage(r.Context(), folders, folder.OwnerUserID)
		if err != nil {
			writeAPIError(w, r, 503, "file_usage_unavailable", err.Error(), nil)
			return 0, false
		}
		allowance = min(allowance, max(int64(0), userQuota-total.UsedBytes-total.ReservedBytes+credit))
	}
	return allowance, true
}

func (s *server) writeFileLimitError(w http.ResponseWriter, r *http.Request, folder state.NASFolderRecord) {
	limited := folder.QuotaBytes > 0
	if store, ok := s.state.(nasManagementState); ok && folder.Kind == "private" {
		if quotas, err := store.NASUserQuotas(r.Context()); err == nil {
			limited = limited || quotas[folder.OwnerUserID] > 0
		}
	}
	if !limited {
		s.writeFilePoolReserveError(w, r, folder)
		return
	}
	writeAPIError(w, r, http.StatusInsufficientStorage, "file_quota_or_reserve_reached", "folder/user quota or pool reserve reached", map[string]any{"folder_id": folder.ID, "pool_id": folder.PoolID})
}
