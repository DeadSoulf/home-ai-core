package security

import (
	"context"
	"errors"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func (s *Service) UpdateUserIdentity(ctx context.Context, actor Actor, id, username, displayName string, meta RequestContext) (User, error) {
	if !actorIsAdministrator(actor) {
		return User{}, ErrAdministratorRequired
	}
	username, err := NormalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	displayName, err = NormalizeDisplayName(displayName, username)
	if err != nil {
		return User{}, err
	}
	if err := s.store.UpdateUserIdentity(ctx, strings.TrimSpace(id), username, displayName, s.now()); err != nil {
		if errors.Is(err, state.ErrUserExists) {
			return User{}, ErrUserExists
		}
		return User{}, err
	}
	record, err := s.store.UserAccount(ctx, id)
	if err != nil {
		return User{}, err
	}
	user := userFromRecord(record)
	s.audit(ctx, meta, actor, "security.user.identity.update", "user", id, "success", map[string]any{"username": username, "display_name": displayName})
	return user, nil
}

func (s *Service) ResetUserPassword(ctx context.Context, actor Actor, id, password string, meta RequestContext) error {
	if !actorIsAdministrator(actor) {
		return ErrAdministratorRequired
	}
	if id == actor.ID {
		return errors.New("use account password change with the current password for your own account")
	}
	return s.setUserPassword(ctx, actor, id, "", password, true, meta)
}

func (s *Service) ChangePassword(ctx context.Context, actor Actor, current, password string, meta RequestContext) error {
	return s.setUserPassword(ctx, actor, actor.ID, current, password, false, meta)
}

func (s *Service) setUserPassword(ctx context.Context, actor Actor, id, current, password string, administratorReset bool, meta RequestContext) error {
	record, err := s.store.UserAccount(ctx, id)
	if err != nil {
		return err
	}
	if !administratorReset {
		valid, err := VerifyPassword(record.User.PasswordHash, current)
		if err != nil || !valid {
			return ErrInvalidCredentials
		}
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.store.ChangeUserPassword(ctx, id, record.User.PasswordHash, hash, s.now()); err != nil {
		return err
	}
	action := "security.user.password.change"
	if administratorReset {
		action = "security.user.password.reset"
	}
	s.audit(ctx, meta, actor, action, "user", id, "success", map[string]any{"sessions_revoked": true})
	return nil
}
