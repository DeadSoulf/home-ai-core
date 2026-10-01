package aiagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

var (
	ErrConversationUnavailable = errors.New("AI conversation storage is unavailable")
	ErrProviderUnavailable     = errors.New("AI model provider is unavailable")
	ErrModelNotFound           = errors.New("AI model not found")
	ErrMessageTooLarge         = errors.New("AI message is too large")
)

const (
	maxConversationMessages = 100
	maxUserMessageBytes      = 16 << 10
	chatTimeout              = 5 * time.Minute
)

type ConversationStore interface {
	CreateAISession(context.Context, state.AISessionRecord) error
	AISession(context.Context, string, string) (state.AISessionRecord, error)
	ListAISessions(context.Context, string, int) ([]state.AISessionRecord, error)
	AppendAIMessage(context.Context, string, state.AIMessageRecord) error
	ListAIMessages(context.Context, string, string, int) ([]state.AIMessageRecord, error)
	UpdateAISessionTitle(context.Context, string, string, string, time.Time) error
}

type Conversation struct {
	ID        string                  `json:"id"`
	Provider  string                  `json:"provider"`
	Model     string                  `json:"model"`
	Title     string                  `json:"title"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
	Messages  []state.AIMessageRecord `json:"messages,omitempty"`
}

func (s *Service) Models(ctx context.Context) ([]ModelInfo, error) {
	if s.provider == nil {
		return nil, ErrProviderUnavailable
	}
	models, err := s.provider.Models(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	return models, nil
}

func (s *Service) Conversations(ctx context.Context, actor security.Actor) ([]Conversation, error) {
	if s.sessions == nil {
		return nil, ErrConversationUnavailable
	}
	records, err := s.sessions.ListAISessions(ctx, actor.ID, 50)
	if err != nil {
		return nil, err
	}
	result := make([]Conversation, 0, len(records))
	for _, record := range records {
		result = append(result, conversationFromRecord(record))
	}
	return result, nil
}

func (s *Service) CreateConversation(ctx context.Context, actor security.Actor, model string) (Conversation, error) {
	if s.sessions == nil {
		return Conversation{}, ErrConversationUnavailable
	}
	if s.provider == nil {
		return Conversation{}, ErrProviderUnavailable
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return Conversation{}, ErrModelNotFound
	}
	models, err := s.Models(ctx)
	if err != nil {
		return Conversation{}, err
	}
	found := false
	for _, available := range models {
		if available.ID == model {
			found = true
			break
		}
	}
	if !found {
		return Conversation{}, ErrModelNotFound
	}
	id, err := newConversationID("ais_")
	if err != nil {
		return Conversation{}, err
	}
	now := time.Now().UTC()
	record := state.AISessionRecord{
		ID:        id,
		UserID:    actor.ID,
		Provider:  s.provider.ID(),
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.sessions.CreateAISession(ctx, record); err != nil {
		return Conversation{}, err
	}
	return conversationFromRecord(record), nil
}

func (s *Service) Conversation(ctx context.Context, actor security.Actor, id string) (Conversation, error) {
	if s.sessions == nil {
		return Conversation{}, ErrConversationUnavailable
	}
	record, err := s.sessions.AISession(ctx, strings.TrimSpace(id), actor.ID)
	if err != nil {
		return Conversation{}, err
	}
	messages, err := s.sessions.ListAIMessages(ctx, record.ID, actor.ID, maxConversationMessages)
	if err != nil {
		return Conversation{}, err
	}
	result := conversationFromRecord(record)
	result.Messages = messages
	return result, nil
}

func (s *Service) StreamConversation(
	ctx context.Context,
	actor security.Actor,
	meta security.RequestContext,
	sessionID string,
	content string,
	onDelta func(string) error,
) (Conversation, error) {
	if s.sessions == nil {
		return Conversation{}, ErrConversationUnavailable
	}
	if s.provider == nil {
		return Conversation{}, ErrProviderUnavailable
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return Conversation{}, ErrInvalidToolInput
	}
	if len(content) > maxUserMessageBytes || !utf8.ValidString(content) {
		return Conversation{}, ErrMessageTooLarge
	}

	session, err := s.sessions.AISession(ctx, strings.TrimSpace(sessionID), actor.ID)
	if err != nil {
		return Conversation{}, err
	}
	if session.Provider != s.provider.ID() {
		return Conversation{}, ErrProviderUnavailable
	}

	now := time.Now().UTC()
	userMessageID, err := newConversationID("aim_")
	if err != nil {
		return Conversation{}, err
	}
	if err := s.sessions.AppendAIMessage(ctx, actor.ID, state.AIMessageRecord{
		ID:        userMessageID,
		SessionID: session.ID,
		Role:      string(RoleUser),
		Content:   content,
		CreatedAt: now,
	}); err != nil {
		return Conversation{}, err
	}
	if strings.TrimSpace(session.Title) == "" {
		title := conversationTitle(content)
		if err := s.sessions.UpdateAISessionTitle(ctx, session.ID, actor.ID, title, now); err == nil {
			session.Title = title
		}
	}

	history, err := s.sessions.ListAIMessages(ctx, session.ID, actor.ID, maxConversationMessages)
	if err != nil {
		return Conversation{}, err
	}
	messages := make([]Message, 0, len(history)+1)
	messages = append(messages, Message{
		Role: RoleSystem,
		Content: "You are the local Home-AI assistant. Reply in the user's language unless asked otherwise. Answer clearly and do not claim that you changed the home or server unless a Home-AI tool actually performed that action.",
	})
	for _, item := range history {
		messages = append(messages, Message{Role: MessageRole(item.Role), Content: item.Content})
	}

	started := time.Now()
	chatCtx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()
	response, runErr := s.provider.Stream(chatCtx, ModelRequest{
		Model:    session.Model,
		Messages: messages,
	}, onDelta)
	outcome := "success"
	if runErr != nil {
		outcome = "failed"
		s.recordConversationAudit(ctx, meta, actor, session, outcome, runErr, time.Since(started))
		return Conversation{}, runErr
	}
	assistantContent := strings.TrimSpace(response.Message.Content)
	if assistantContent == "" {
		runErr = errors.New("AI provider returned an empty response")
		s.recordConversationAudit(ctx, meta, actor, session, "failed", runErr, time.Since(started))
		return Conversation{}, runErr
	}

	assistantMessageID, err := newConversationID("aim_")
	if err != nil {
		return Conversation{}, err
	}
	completedAt := time.Now().UTC()
	if err := s.sessions.AppendAIMessage(ctx, actor.ID, state.AIMessageRecord{
		ID:        assistantMessageID,
		SessionID: session.ID,
		Role:      string(RoleAssistant),
		Content:   assistantContent,
		CreatedAt: completedAt,
	}); err != nil {
		return Conversation{}, err
	}
	s.recordConversationAudit(ctx, meta, actor, session, outcome, nil, time.Since(started))
	return s.Conversation(ctx, actor, session.ID)
}

func (s *Service) recordConversationAudit(
	ctx context.Context,
	meta security.RequestContext,
	actor security.Actor,
	session state.AISessionRecord,
	outcome string,
	runErr error,
	duration time.Duration,
) {
	if s.audit == nil {
		return
	}
	metadata := map[string]any{
		"session_id":  session.ID,
		"provider":    session.Provider,
		"model":       session.Model,
		"duration_ms": duration.Milliseconds(),
	}
	if runErr != nil {
		metadata["error_code"] = toolErrorCode(runErr)
	}
	s.audit.RecordAudit(
		context.WithoutCancel(ctx),
		meta,
		actor,
		"ai.chat.message",
		"ai_session",
		session.ID,
		outcome,
		metadata,
	)
}

func conversationFromRecord(record state.AISessionRecord) Conversation {
	return Conversation{
		ID:        record.ID,
		Provider:  record.Provider,
		Model:     record.Model,
		Title:     record.Title,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

func conversationTitle(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) > 60 {
		runes = runes[:60]
	}
	return string(runes)
}

func newConversationID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate AI id: %w", err)
	}
	return prefix + hex.EncodeToString(raw[:]), nil
}
