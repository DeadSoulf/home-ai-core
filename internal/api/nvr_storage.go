package api

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

func (s *server) resolveNVRExternalRoot(ctx context.Context) (string, error) {
	resolve := s.nvrStorageResolver
	if resolve == nil { resolve = modules.ResolveNVRStorage }
	root, err := resolve(ctx)
	if err != nil { return "", err }
	if root == "" { return "", nil }
	// The root can only be a helper-inspected Docker bind mount from the
	// NVR container, never a URL or path supplied by a browser.
	clean := filepath.Clean(root)
	if !strings.HasPrefix(clean, "/mnt/home-ai-core/") ||
		!strings.HasSuffix(clean, "/nvr-recordings") {
		return "", &nvrUntrustedStorageError{}
	}
	return clean, nil
}

type nvrUntrustedStorageError struct{}
func (*nvrUntrustedStorageError) Error() string { return "untrusted NVR recording volume" }

func (s *server) nvrStorageStatus(w http.ResponseWriter, r *http.Request, actor security.Actor, _ authSource) {
	if !actor.Has("storage.manage") {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "storage manage permission required", nil)
		return
	}
	if !s.nvrReady(w, r) { return }
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()
	root, err := s.resolveNVRExternalRoot(ctx)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable", "NVR recording storage could not be inspected", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attached": root != "", "path": root})
}

func (s *server) nvrStorageAttach(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if !actor.Has("storage.manage") {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "storage manage permission required", nil)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) { return }
	var input struct { Device string `json:"device"` }
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_storage", err.Error(), nil)
		return
	}
	records, err := s.state.ListStoragePurposes(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "storage_purposes_unavailable", "Storage assignments unavailable", nil)
		return
	}
	nodes := systeminfo.Collect(s.nodeID).BlockTree
	node, ok := blockNodeByPath(nodes, strings.TrimSpace(input.Device))
	if !ok || node.System || (node.Type != "part" && node.Type != "lvm") ||
		(node.Filesystem != "ext4" && node.Filesystem != "xfs") {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_nvr_storage", "Select a mounted non-system ext4/xfs partition", nil)
		return
	}
	if assignedStoragePurposeForNode(node.Path, node.UUID, records) != state.StoragePurposeNVR {
		writeAPIError(w, r, http.StatusConflict, "nvr_storage_not_assigned", "Assign this partition to NVR before attaching", nil)
		return
	}
	mount := ""
	for _, candidate := range node.Mountpoints {
		clean := filepath.Clean(candidate)
		if strings.HasPrefix(clean, "/mnt/home-ai-core/") && clean != "/mnt/home-ai-core" {
			mount = clean
			break
		}
	}
	if mount == "" {
		writeAPIError(w, r, http.StatusConflict, "nvr_storage_not_mounted", "Mount the assigned partition under /mnt/home-ai-core first", nil)
		return
	}
	attach := s.nvrStorageAttacher
	if attach == nil { attach = modules.AttachNVRStorage }
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(3*time.Minute+20*time.Second))
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute+10*time.Second)
	defer cancel()
	if err := attach(ctx, node.Path, mount); err != nil {
		writeAPIError(w, r, http.StatusConflict, "nvr_storage_attach_failed", err.Error(), nil)
		return
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.storage.attach", "module", "nvr.module", "success",
		map[string]any{"device": node.Path, "mount": mount})
	writeJSON(w, http.StatusOK, map[string]any{"attached": true, "path": filepath.Join(mount, "nvr-recordings")})
}
