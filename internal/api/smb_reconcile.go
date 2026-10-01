package api

import (
	"context"
	"fmt"
	"github.com/DeadSoulf/home-ai-core/internal/smb"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"log/slog"
	"net/http"
	"time"
)

var suspendSMBAccess = smb.Suspend
var applySMBAccess = smb.Apply
var inspectFolderHardQuota = storage.InspectNASQuota
var applyFolderHardQuota = func(ctx context.Context, folder state.NASFolderRecord, quota int64) error {
	return storage.ApplyNASQuota(ctx, folder.PoolRoot, folder.RelativePath, folder.ProjectID+1000000, quota)
}

func (s *server) beginSMBAccessChange(r *http.Request) (bool, error) {
	// Older embedders have no NAS-management persistence or managed Samba.
	if _, ok := s.state.(nasManagementState); !ok {
		return false, nil
	}
	return suspendSMBAccess(r.Context())
}

func (s *server) finishSMBAccessChange(r *http.Request, active bool) string {
	if !active {
		return ""
	}
	model, err := s.buildSMBModel(r)
	if err == nil {
		group := defaultSMBWorkgroup
		if store, ok := s.state.(nasManagementState); ok {
			if value, readErr := store.NASSetting(r.Context(), "smb_workgroup"); readErr != nil {
				err = readErr
			} else if value != "" {
				group = value
			}
		}
		if err == nil {
			_, err = applySMBAccess(r.Context(), group, model.Shares)
		}
	}
	if err != nil {
		s.logger.Error("SMB reconciliation failed; shares remain suspended", "error", err)
		return fmt.Sprintf("SMB access remains suspended; apply Windows shares after resolving: %v", err)
	}
	return ""
}

// ReconcileFileAccess restores authoritative grants after Core/helper restart.
// The helper suspends previously applied shares before this refresh.
func ReconcileFileAccess(ctx context.Context, logger *slog.Logger, store State, securityService SecurityService) {
	server := &server{state: store, security: securityService, logger: logger}
	for {
		fileMutationMu.Lock()
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/", nil)
		active, err := server.beginSMBAccessChange(request)
		warning := ""
		if err == nil {
			warning = server.finishSMBAccessChange(request, active)
		}
		fileMutationMu.Unlock()
		if err == nil && warning == "" {
			return
		}
		logger.Warn("file access reconciliation pending", "error", err, "warning", warning)
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
