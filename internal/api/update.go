package api

import (
	"context"
	"net/http"

	"github.com/DeadSoulf/home-ai-core/internal/updater"
)

type UpdaterService interface {
	Check(context.Context) (updater.ReleaseStatus, error)
}

func (s *server) updateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	if s.updater == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "updater_unavailable", "updater is unavailable", nil)
		return
	}
	status, err := s.updater.Check(r.Context())
	if err != nil {
		s.logger.Error("update check failed", "error", err)
		writeAPIError(w, r, http.StatusBadGateway, "update_check_failed", "failed to check GitHub for updates", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"update": status})
}
