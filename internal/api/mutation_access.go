package api

import (
	"errors"
	"net/http"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

// A request may wait behind another user's permission change. Refresh the
// actor after acquiring the mutation lock, before it can touch stored data.
func (s *server) refreshMutationActor(w http.ResponseWriter, r *http.Request, permission string) (security.Actor, bool) {
	token, _ := sessionToken(r)
	actor, err := s.security.Authenticate(r.Context(), token)
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, security.ErrUnauthorized) {
			status = http.StatusUnauthorized
		}
		writeAPIError(w, r, status, "authentication_required", "current authentication required", nil)
		return security.Actor{}, false
	}
	if permission != "" && !actor.Has(permission) {
		writeAPIError(w, r, http.StatusForbidden, "permission_denied", "permission denied", nil)
		return security.Actor{}, false
	}
	return actor, true
}
