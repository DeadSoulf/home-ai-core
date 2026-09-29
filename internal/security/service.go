package security

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var (
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrInvalidBootstrapToken = errors.New("invalid bootstrap token")
	ErrAlreadyInitialized    = errors.New("security already initialized")
	ErrUnauthorized          = errors.New("authentication required")
)

const sessionLifetime = 24 * time.Hour

type RequestContext struct {
	RequestID     string
	CorrelationID string
	RemoteAddr    string
}

type PermissionScope struct {
	Permission   string `json:"permission"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

type Actor struct {
	Type                string            `json:"type"`
	ID                  string            `json:"id"`
	Username            string            `json:"username,omitempty"`
	DisplayName         string            `json:"display_name,omitempty"`
	SessionID           string            `json:"-"`
	Roles               []string          `json:"roles"`
	Permissions         []string          `json:"permissions"`
	ResourcePermissions []PermissionScope `json:"resource_permissions,omitempty"`

	csrfHash string
}

func (a Actor) Has(permission string) bool {
	for _, current := range a.Permissions {
		if current == permission {
			return true
		}
	}
	return false
}

// Allows returns true when the actor has the permission globally or has an
// exact scoped grant for the requested resource. Resource hierarchy (for
// example room -> device) is deliberately resolved by the owning domain.
func (a Actor) Allows(permission, resourceType, resourceID string) bool {
	if a.Has(permission) {
		return true
	}
	if permission == "" || resourceType == "" || resourceID == "" {
		return false
	}
	for _, scope := range a.ResourcePermissions {
		if scope.Permission == permission &&
			scope.ResourceType == resourceType &&
			scope.ResourceID == resourceID {
			return true
		}
	}
	return false
}

func (a Actor) ValidCSRF(token string) bool {
	if token == "" || a.csrfHash == "" {
		return false
	}
	actual := hashSecret(token)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(a.csrfHash)) == 1
}

type AuthResult struct {
	Actor     Actor
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

type AuditEntry struct {
	ID            string         `json:"id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	ActorType     string         `json:"actor_type"`
	ActorID       string         `json:"actor_id,omitempty"`
	Action        string         `json:"action"`
	TargetType    string         `json:"target_type,omitempty"`
	TargetID      string         `json:"target_id,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Outcome       string         `json:"outcome"`
	Metadata      map[string]any `json:"metadata"`
}

type Service struct {
	store     *state.Store
	stateDir  string
	dummyHash string
	now       func() time.Time
}

func New(ctx context.Context, store *state.Store, stateDir string) (*Service, error) {
	dummyHash, err := HashPassword("home-ai-dummy-password-not-a-credential")
	if err != nil {
		return nil, fmt.Errorf("initialize password verifier: %w", err)
	}

	service := &Service{
		store:     store,
		stateDir:  stateDir,
		dummyHash: dummyHash,
		now:       time.Now,
	}

	initialized, err := service.Initialized(ctx)
	if err != nil {
		return nil, err
	}
	if err := service.prepareBootstrap(initialized); err != nil {
		return nil, err
	}
	if err := store.CleanupExpiredSessions(ctx, service.now()); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) Initialized(ctx context.Context) (bool, error) {
	count, err := s.store.UserCount(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) Bootstrap(
	ctx context.Context,
	bootstrapToken, username, displayName, password string,
	meta RequestContext,
) (AuthResult, error) {
	initialized, err := s.Initialized(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	if initialized {
		return AuthResult{}, ErrAlreadyInitialized
	}
	if !s.verifyBootstrapToken(bootstrapToken) {
		return AuthResult{}, ErrInvalidBootstrapToken
	}

	username, err = NormalizeUsername(username)
	if err != nil {
		return AuthResult{}, err
	}
	displayName, err = NormalizeDisplayName(displayName, username)
	if err != nil {
		return AuthResult{}, err
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return AuthResult{}, err
	}
	userID, err := newID("usr_")
	if err != nil {
		return AuthResult{}, err
	}

	now := s.now().UTC()
	user, err := s.store.CreateOwner(ctx, userID, username, displayName, passwordHash, now)
	if errors.Is(err, state.ErrAlreadyInitialized) {
		return AuthResult{}, ErrAlreadyInitialized
	}
	if err != nil {
		return AuthResult{}, err
	}
	if err := s.removeBootstrapToken(); err != nil {
		return AuthResult{}, fmt.Errorf("remove bootstrap token: %w", err)
	}

	result, err := s.createSession(ctx, user, now)
	if err != nil {
		return AuthResult{}, err
	}
	s.audit(ctx, meta, Actor{Type: "user", ID: user.ID, Username: user.Username},
		"security.bootstrap", "user", user.ID, "success", nil)
	return result, nil
}

func (s *Service) Login(
	ctx context.Context,
	username, password string,
	meta RequestContext,
) (AuthResult, error) {
	normalized, normalizeErr := NormalizeUsername(username)
	if normalizeErr != nil {
		_, _ = VerifyPassword(s.dummyHash, password)
		s.audit(ctx, meta, Actor{Type: "anonymous"}, "auth.login", "", "", "denied",
			map[string]any{"reason": "invalid_credentials"})
		return AuthResult{}, ErrInvalidCredentials
	}

	user, err := s.store.UserByUsername(ctx, normalized)
	if errors.Is(err, sql.ErrNoRows) {
		_, _ = VerifyPassword(s.dummyHash, password)
		s.audit(ctx, meta, Actor{Type: "anonymous"}, "auth.login", "", "", "denied",
			map[string]any{"reason": "invalid_credentials"})
		return AuthResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthResult{}, err
	}

	valid, err := VerifyPassword(user.PasswordHash, password)
	if err != nil {
		return AuthResult{}, err
	}
	if !valid || user.Disabled {
		s.audit(ctx, meta, Actor{Type: "anonymous"}, "auth.login", "user", user.ID, "denied",
			map[string]any{"reason": "invalid_credentials"})
		return AuthResult{}, ErrInvalidCredentials
	}

	now := s.now().UTC()
	if err := s.store.UpdateLastLogin(ctx, user.ID, now); err != nil {
		return AuthResult{}, err
	}
	result, err := s.createSession(ctx, user, now)
	if err != nil {
		return AuthResult{}, err
	}

	s.audit(ctx, meta, result.Actor, "auth.login", "session", result.Actor.SessionID, "success", nil)
	return result, nil
}

func (s *Service) createSession(ctx context.Context, user state.UserRecord, now time.Time) (AuthResult, error) {
	sessionID, err := newID("ses_")
	if err != nil {
		return AuthResult{}, err
	}
	token, err := newSecret()
	if err != nil {
		return AuthResult{}, err
	}
	csrfToken, err := newSecret()
	if err != nil {
		return AuthResult{}, err
	}

	expiresAt := now.Add(sessionLifetime)
	if err := s.store.CreateSession(
		ctx,
		sessionID,
		user.ID,
		hashSecret(token),
		hashSecret(csrfToken),
		now,
		expiresAt,
	); err != nil {
		return AuthResult{}, err
	}

	record, err := s.store.SessionByTokenHash(ctx, hashSecret(token), now)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		Actor:     actorFromSession(record),
		Token:     token,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Actor, error) {
	if token == "" {
		return Actor{}, ErrUnauthorized
	}
	record, err := s.store.SessionByTokenHash(ctx, hashSecret(token), s.now())
	if errors.Is(err, sql.ErrNoRows) {
		return Actor{}, ErrUnauthorized
	}
	if err != nil {
		return Actor{}, err
	}
	return actorFromSession(record), nil
}

func (s *Service) Logout(ctx context.Context, actor Actor, meta RequestContext) error {
	if actor.SessionID == "" {
		return ErrUnauthorized
	}
	if err := s.store.RevokeSession(ctx, actor.SessionID, s.now()); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	s.audit(ctx, meta, actor, "auth.logout", "session", actor.SessionID, "success", nil)
	return nil
}

func (s *Service) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	records, err := s.store.ListAudit(ctx, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]AuditEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, AuditEntry{
			ID:            record.ID,
			OccurredAt:    record.OccurredAt,
			ActorType:     record.ActorType,
			ActorID:       record.ActorID,
			Action:        record.Action,
			TargetType:    record.TargetType,
			TargetID:      record.TargetID,
			RequestID:     record.RequestID,
			CorrelationID: record.CorrelationID,
			Outcome:       record.Outcome,
			Metadata:      record.Metadata,
		})
	}
	return entries, nil
}

func (s *Service) RecordAudit(
	ctx context.Context,
	meta RequestContext,
	actor Actor,
	action, targetType, targetID, outcome string,
	metadata map[string]any,
) {
	s.audit(ctx, meta, actor, action, targetType, targetID, outcome, metadata)
}

func (s *Service) audit(
	ctx context.Context,
	meta RequestContext,
	actor Actor,
	action, targetType, targetID, outcome string,
	metadata map[string]any,
) {
	id, err := newID("aud_")
	if err != nil {
		return
	}
	_ = s.store.WriteAudit(ctx, state.AuditRecord{
		ID:            id,
		OccurredAt:    s.now().UTC(),
		ActorType:     actor.Type,
		ActorID:       actor.ID,
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		RequestID:     meta.RequestID,
		CorrelationID: meta.CorrelationID,
		Outcome:       outcome,
		Metadata:      metadata,
	})
}

func actorFromSession(record state.SessionRecord) Actor {
	return Actor{
		Type:        "user",
		ID:          record.User.ID,
		Username:    record.User.Username,
		DisplayName: record.User.DisplayName,
		SessionID:   record.ID,
		Roles:       record.Roles,
		Permissions:         record.Permissions,
		ResourcePermissions: permissionScopes(record.ResourcePermissions),
		csrfHash:            record.CSRFHash,
	}
}

func permissionScopes(records []state.ResourcePermissionRecord) []PermissionScope {
	if len(records) == 0 {
		return nil
	}
	result := make([]PermissionScope, 0, len(records))
	for _, record := range records {
		result = append(result, PermissionScope{
			Permission:   record.Permission,
			ResourceType: record.ResourceType,
			ResourceID:   record.ResourceID,
		})
	}
	return result
}
