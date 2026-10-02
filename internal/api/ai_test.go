package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/aiagent"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type capturedAIAudit struct {
	action, targetID, outcome string
	metadata                  map[string]any
}
type aiCaptureSecurity struct {
	fakeSecurity
	audits []capturedAIAudit
}

func (s *aiCaptureSecurity) RecordAudit(_ context.Context, _ security.RequestContext, _ security.Actor, action, _ string, targetID, outcome string, metadata map[string]any) {
	s.audits = append(s.audits, capturedAIAudit{action: action, targetID: targetID, outcome: outcome, metadata: metadata})
}

func TestAIStatusAndPermissionFilteredTools(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"system.read"}
	handler := testHandlerWithSecurity(fakeState{schemaVersion: 7}, sec)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/status", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status endpoint = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"module_id":"ai.agent"`) || !strings.Contains(rec.Body.String(), `"tool_count":3`) {
		t.Fatalf("status body = %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/ai/tools", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tools endpoint = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Tools []struct {
			ID string `json:"id"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tools) != 1 || body.Tools[0].ID != "core.system.status" {
		t.Fatalf("tools = %#v", body.Tools)
	}
}

func TestAIReadToolExecutionIsAudited(t *testing.T) {
	base := defaultFakeSecurity()
	base.actor.Permissions = []string{"system.read"}
	sec := &aiCaptureSecurity{fakeSecurity: base}
	handler := testHandlerWithSecurity(fakeState{schemaVersion: 7}, sec)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/tools/core.system.status/execute", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("execute status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"schema_version":7`) {
		t.Fatalf("execute body = %s", rec.Body.String())
	}
	if len(sec.audits) != 1 {
		t.Fatalf("audit count = %d, want 1", len(sec.audits))
	}
	if sec.audits[0].action != "ai.tool.execute" || sec.audits[0].targetID != "core.system.status" || sec.audits[0].outcome != "success" {
		t.Fatalf("audit = %#v", sec.audits[0])
	}
	if _, ok := sec.audits[0].metadata["input"]; ok {
		t.Fatalf("raw AI input leaked into audit: %#v", sec.audits[0].metadata)
	}
}

func TestAIToolExecutionFailsClosedWithoutPermission(t *testing.T) {
	base := defaultFakeSecurity()
	base.actor.Permissions = nil
	sec := &aiCaptureSecurity{fakeSecurity: base}
	handler := testHandlerWithSecurity(fakeState{schemaVersion: 7}, sec)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/tools/core.system.status/execute", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if len(sec.audits) != 1 || sec.audits[0].outcome != "denied" {
		t.Fatalf("audits = %#v", sec.audits)
	}
}

type apiChatState struct {
	fakeState
	conversations map[string]state.AIConversationRecord
	messages      map[string][]state.AIMessageRecord
	actions       map[string]state.AIToolActionRecord
}

func newAPIChatState() *apiChatState {
	return &apiChatState{
		fakeState:     fakeState{schemaVersion: 21},
		conversations: map[string]state.AIConversationRecord{},
		messages:      map[string][]state.AIMessageRecord{},
		actions:       map[string]state.AIToolActionRecord{},
	}
}

func (s *apiChatState) CreateAIConversation(_ context.Context, id, userID, title string, now time.Time) (state.AIConversationRecord, error) {
	item := state.AIConversationRecord{ID: id, UserID: userID, Title: title, CreatedAt: now, UpdatedAt: now}
	s.conversations[id] = item
	return item, nil
}

func (s *apiChatState) ListAIConversations(_ context.Context, userID string, _ int) ([]state.AIConversationRecord, error) {
	result := []state.AIConversationRecord{}
	for _, item := range s.conversations {
		if item.UserID == userID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *apiChatState) AIConversation(_ context.Context, id, userID string) (state.AIConversationRecord, error) {
	item, ok := s.conversations[id]
	if !ok || item.UserID != userID {
		return state.AIConversationRecord{}, state.ErrAIConversationNotFound
	}
	return item, nil
}

func (s *apiChatState) UpdateAIConversationTitle(_ context.Context, id, userID, title string, now time.Time) error {
	item, ok := s.conversations[id]
	if !ok || item.UserID != userID {
		return state.ErrAIConversationNotFound
	}
	item.Title = title
	item.UpdatedAt = now
	s.conversations[id] = item
	return nil
}

func (s *apiChatState) CloseAIConversation(_ context.Context, id, userID string, now time.Time) (state.AIConversationRecord, error) {
	item, ok := s.conversations[id]
	if !ok || item.UserID != userID {
		return state.AIConversationRecord{}, state.ErrAIConversationNotFound
	}
	if item.ClosedAt == nil {
		value := now
		item.ClosedAt = &value
		item.UpdatedAt = now
		s.conversations[id] = item
	}
	return item, nil
}

func (s *apiChatState) AppendAIMessage(_ context.Context, id, conversationID, userID, role, content string, now time.Time) (state.AIMessageRecord, error) {
	conversation, err := s.AIConversation(context.Background(), conversationID, userID)
	if err != nil {
		return state.AIMessageRecord{}, err
	}
	if conversation.ClosedAt != nil {
		return state.AIMessageRecord{}, state.ErrAIConversationClosed
	}
	item := state.AIMessageRecord{ID: id, ConversationID: conversationID, Role: role, Content: content, CreatedAt: now}
	s.messages[conversationID] = append(s.messages[conversationID], item)
	return item, nil
}

func (s *apiChatState) ListAIMessages(_ context.Context, conversationID, userID string, limit int) ([]state.AIMessageRecord, error) {
	if _, err := s.AIConversation(context.Background(), conversationID, userID); err != nil {
		return nil, err
	}
	items := s.messages[conversationID]
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return append([]state.AIMessageRecord(nil), items...), nil
}

func (s *apiChatState) CreateAIToolAction(
	_ context.Context,
	id, conversationID, userID, toolID, toolName, sensitivity string,
	input json.RawMessage,
	now time.Time,
) (state.AIToolActionRecord, error) {
	if _, err := s.AIConversation(context.Background(), conversationID, userID); err != nil {
		return state.AIToolActionRecord{}, err
	}
	item := state.AIToolActionRecord{
		ID: id, ConversationID: conversationID, UserID: userID, ToolID: toolID, ToolName: toolName,
		Sensitivity: sensitivity, Input: append(json.RawMessage(nil), input...),
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	s.actions[id] = item
	return item, nil
}

func (s *apiChatState) ListAIToolActions(_ context.Context, conversationID, userID string, _ int) ([]state.AIToolActionRecord, error) {
	if _, err := s.AIConversation(context.Background(), conversationID, userID); err != nil {
		return nil, err
	}
	result := []state.AIToolActionRecord{}
	for _, item := range s.actions {
		if item.ConversationID == conversationID && item.UserID == userID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *apiChatState) ClaimAIToolAction(_ context.Context, id, conversationID, userID string, now time.Time) (state.AIToolActionRecord, error) {
	item, ok := s.actions[id]
	if !ok || item.ConversationID != conversationID || item.UserID != userID {
		return state.AIToolActionRecord{}, state.ErrAIToolActionNotFound
	}
	if item.Status != "pending" {
		return item, state.ErrAIToolActionNotPending
	}
	item.Status = "executing"
	item.UpdatedAt = now
	s.actions[id] = item
	return item, nil
}

func (s *apiChatState) RejectAIToolAction(_ context.Context, id, conversationID, userID string, now time.Time) (state.AIToolActionRecord, error) {
	item, ok := s.actions[id]
	if !ok || item.ConversationID != conversationID || item.UserID != userID {
		return state.AIToolActionRecord{}, state.ErrAIToolActionNotFound
	}
	if item.Status != "pending" {
		return item, state.ErrAIToolActionNotPending
	}
	item.Status = "rejected"
	item.UpdatedAt = now
	s.actions[id] = item
	return item, nil
}

func (s *apiChatState) FinishAIToolAction(
	_ context.Context,
	id, conversationID, userID, status string,
	result json.RawMessage,
	errorCode string,
	now time.Time,
) (state.AIToolActionRecord, error) {
	item, ok := s.actions[id]
	if !ok || item.ConversationID != conversationID || item.UserID != userID {
		return state.AIToolActionRecord{}, state.ErrAIToolActionNotFound
	}
	if item.Status != "executing" {
		return item, state.ErrAIToolActionNotPending
	}
	item.Status = status
	item.Result = append(json.RawMessage(nil), result...)
	item.ErrorCode = errorCode
	item.UpdatedAt = now
	s.actions[id] = item
	return item, nil
}

func newAIChatHandler(chatState *apiChatState, sec SecurityService, provider aiagent.Provider) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"
	return New(
		nodeID,
		logger,
		chatState,
		sec,
		nil,
		nil,
		nil,
		nil,
		realtime.New(nodeID, logger),
		provider,
	)
}

func TestAIConversationAPIWithLocalProvider(t *testing.T) {
	chatState := newAPIChatState()
	sec := defaultFakeSecurity()
	provider := aiagent.DeterministicProvider{
		ProviderID: "test-local",
		Response: aiagent.ModelResponse{
			Message: aiagent.Message{Role: aiagent.RoleAssistant, Content: "Local API reply"},
		},
	}
	handler := newAIChatHandler(chatState, sec, provider)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Conversation state.AIConversationRecord `json:"conversation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Conversation.ID == "" {
		t.Fatal("missing conversation id")
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ai/conversations/"+created.Conversation.ID+"/messages",
		strings.NewReader(`{"content":"hello local model"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Local API reply") {
		t.Fatalf("chat body = %s", rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/ai/conversations/"+created.Conversation.ID+"/messages",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("messages status = %d: %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Messages []state.AIMessageRecord `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Messages) != 2 {
		t.Fatalf("messages = %#v", listed.Messages)
	}

	other := defaultFakeSecurity()
	other.actor.ID = "usr-other"
	foreign := newAIChatHandler(chatState, other, provider)
	req = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/ai/conversations/"+created.Conversation.ID+"/messages",
		nil,
	)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	foreign.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign messages status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestAIConversationCloseAPIKeepsHistoryReadOnly(t *testing.T) {
	chatState := newAPIChatState()
	sec := defaultFakeSecurity()
	provider := aiagent.DeterministicProvider{
		ProviderID: "test-local",
		Response: aiagent.ModelResponse{
			Message: aiagent.Message{Role: aiagent.RoleAssistant, Content: "reply"},
		},
	}
	handler := newAIChatHandler(chatState, sec, provider)

	conv, err := chatState.CreateAIConversation(context.Background(), "aic-close", sec.actor.ID, "Close me", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/"+conv.ID+"/close", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("close status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "closed_at") {
		t.Fatalf("close body = %s", rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ai/conversations/"+conv.ID+"/messages",
		strings.NewReader(`{"content":"must fail"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "ai_conversation_closed") {
		t.Fatalf("closed chat send status = %d: %s", rec.Code, rec.Body.String())
	}

	other := defaultFakeSecurity()
	other.actor.ID = "usr-other"
	foreign := newAIChatHandler(chatState, other, provider)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/"+conv.ID+"/close", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	foreign.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign close status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestAIConversationActionListAndRejectAPI(t *testing.T) {
	chatState := newAPIChatState()
	sec := defaultFakeSecurity()
	handler := newAIChatHandler(chatState, sec, nil)

	conversation, err := chatState.CreateAIConversation(context.Background(), "aic-actions", sec.actor.ID, "Actions", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	action, err := chatState.CreateAIToolAction(
		context.Background(),
		"aia-actions",
		conversation.ID,
		sec.actor.ID,
		"core.storage.mount",
		"Mount storage",
		"change",
		json.RawMessage(`{"device":"/dev/sdb1"}`),
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/conversations/"+conversation.ID+"/actions", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), action.ID) {
		t.Fatalf("actions status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ai/conversations/"+conversation.ID+"/actions/"+action.ID+"/reject",
		strings.NewReader(`{}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"rejected"`) {
		t.Fatalf("reject status = %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ai/conversations/"+conversation.ID+"/actions/"+action.ID+"/reject",
		strings.NewReader(`{}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second reject status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestAIConversationMessageFailsWhenProviderMissing(t *testing.T) {
	chatState := newAPIChatState()
	sec := defaultFakeSecurity()
	handler := newAIChatHandler(chatState, sec, nil)

	conv, err := chatState.CreateAIConversation(context.Background(), "aic-test", sec.actor.ID, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ai/conversations/"+conv.ID+"/messages",
		strings.NewReader(`{"content":"hello"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
}
