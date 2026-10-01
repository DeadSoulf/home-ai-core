package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type EventHistoryService interface {
	History(context.Context, int64, int) ([]state.EventRecord, error)
	LatestCursor(context.Context) (int64, error)
}

func (s *server) eventHistory(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}

	var after int64
	if value := strings.TrimSpace(r.URL.Query().Get("after")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_cursor", "after must be a non-negative integer", nil)
			return
		}
		after = parsed
	}

	limit, ok := queryLimit(r, 100)
	if !ok {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500", nil)
		return
	}

	events, err := s.eventHistoryService.History(r.Context(), after, limit)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "events_unavailable", "event history is unavailable", nil)
		return
	}

	latest, err := s.eventHistoryService.LatestCursor(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "events_unavailable", "event history is unavailable", nil)
		return
	}

	visible := make([]state.EventRecord, 0, len(events))
	for _, event := range events {
		if actorAllowsEvent(actor, event.Type, event.Data) {
			visible = append(visible, event)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":        visible,
		"latest_cursor": latest,
	})
}
