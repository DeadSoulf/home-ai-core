package aiagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type serviceState struct{ schema int }

func (s serviceState) SchemaVersion(context.Context) (int, error) { return s.schema, nil }

type serviceJobs struct{ jobs []state.JobRecord }

func (s serviceJobs) List(context.Context, string, int) ([]state.JobRecord, error) {
	return s.jobs, nil
}

type serviceModules struct {
	items        []modules.Registered
	capabilities []string
}

func (s serviceModules) List(context.Context) ([]modules.Registered, error) { return s.items, nil }
func (s serviceModules) Capabilities(context.Context) ([]string, error)     { return s.capabilities, nil }

type auditCall struct {
	action, targetType, targetID, outcome string
	metadata                              map[string]any
}
type serviceAudit struct{ calls []auditCall }

func (a *serviceAudit) RecordAudit(_ context.Context, _ security.RequestContext, _ security.Actor, action, targetType, targetID, outcome string, metadata map[string]any) {
	a.calls = append(a.calls, auditCall{action: action, targetType: targetType, targetID: targetID, outcome: outcome, metadata: metadata})
}

func TestServiceAvailableToolsRespectPermissions(t *testing.T) {
	s := NewService("node-1", serviceState{schema: 1}, serviceJobs{}, serviceModules{}, nil)
	actor := security.Actor{Permissions: []string{"system.read", "modules.read"}}
	tools := s.AvailableTools(actor)
	if len(tools) != 2 {
		t.Fatalf("tool count = %d, want 2: %#v", len(tools), tools)
	}
	if tools[0].ID != "core.modules.list" || tools[1].ID != "core.system.status" {
		t.Fatalf("tools = %#v", tools)
	}
}

func TestServiceExecuteJobsAuditsAndRedactsPayloads(t *testing.T) {
	audit := &serviceAudit{}
	jobs := serviceJobs{jobs: []state.JobRecord{{
		ID: "job-1", NodeID: "node-1", Type: "update.download", Status: "failed", Progress: 4200,
		Input: map[string]any{"secret": "must-not-leak"}, Result: map[string]any{"token": "must-not-leak"},
		ErrorCode: "download_failed", ErrorMessage: "network unavailable", CreatedAt: time.Unix(1, 0).UTC(),
	}}}
	s := NewService("node-1", serviceState{schema: 1}, jobs, serviceModules{}, audit)
	actor := security.Actor{Type: "user", ID: "usr-1", Permissions: []string{"jobs.read"}}
	result, err := s.Execute(context.Background(), actor, security.RequestContext{RequestID: "req-1"}, "core.jobs.list", json.RawMessage(`{"limit":5}`), false)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(result) == "" {
		t.Fatal("empty tool result")
	}
	var body map[string]any
	if err := json.Unmarshal(result, &body); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result, []byte("must-not-leak")) {
		t.Fatalf("sensitive job payload leaked: %s", result)
	}
	if len(audit.calls) != 1 {
		t.Fatalf("audit calls = %d, want 1", len(audit.calls))
	}
	call := audit.calls[0]
	if call.action != "ai.tool.execute" || call.targetID != "core.jobs.list" || call.outcome != "success" {
		t.Fatalf("audit = %#v", call)
	}
	if _, ok := call.metadata["input"]; ok {
		t.Fatalf("raw input present in audit metadata: %#v", call.metadata)
	}
}

func TestServiceDeniedToolIsAudited(t *testing.T) {
	audit := &serviceAudit{}
	s := NewService("node-1", serviceState{schema: 1}, serviceJobs{}, serviceModules{}, audit)
	actor := security.Actor{Type: "user", ID: "usr-1"}
	_, err := s.Execute(context.Background(), actor, security.RequestContext{}, "core.modules.list", nil, false)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Execute() error = %v, want permission denied", err)
	}
	if len(audit.calls) != 1 || audit.calls[0].outcome != "denied" {
		t.Fatalf("audit calls = %#v", audit.calls)
	}
	if audit.calls[0].metadata["error_code"] != "permission_denied" {
		t.Fatalf("metadata = %#v", audit.calls[0].metadata)
	}
}

type chatMemoryStore struct {
	schema        int
	conversations map[string]state.AIConversationRecord
	messages      map[string][]state.AIMessageRecord
	actions       map[string]state.AIToolActionRecord
}

func newChatMemoryStore() *chatMemoryStore {
	return &chatMemoryStore{
		schema:        21,
		conversations: map[string]state.AIConversationRecord{},
		messages:      map[string][]state.AIMessageRecord{},
		actions:       map[string]state.AIToolActionRecord{},
	}
}

func (s *chatMemoryStore) SchemaVersion(context.Context) (int, error) { return s.schema, nil }

func (s *chatMemoryStore) CreateAIConversation(_ context.Context, id, userID, title string, now time.Time) (state.AIConversationRecord, error) {
	record := state.AIConversationRecord{ID: id, UserID: userID, Title: title, CreatedAt: now, UpdatedAt: now}
	s.conversations[id] = record
	return record, nil
}

func (s *chatMemoryStore) ListAIConversations(_ context.Context, userID string, _ int) ([]state.AIConversationRecord, error) {
	result := []state.AIConversationRecord{}
	for _, item := range s.conversations {
		if item.UserID == userID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *chatMemoryStore) AIConversation(_ context.Context, id, userID string) (state.AIConversationRecord, error) {
	item, ok := s.conversations[id]
	if !ok || item.UserID != userID {
		return state.AIConversationRecord{}, state.ErrAIConversationNotFound
	}
	return item, nil
}

func (s *chatMemoryStore) UpdateAIConversationTitle(_ context.Context, id, userID, title string, now time.Time) error {
	item, ok := s.conversations[id]
	if !ok || item.UserID != userID {
		return state.ErrAIConversationNotFound
	}
	item.Title = title
	item.UpdatedAt = now
	s.conversations[id] = item
	return nil
}

func (s *chatMemoryStore) CloseAIConversation(_ context.Context, id, userID string, now time.Time) (state.AIConversationRecord, error) {
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

func (s *chatMemoryStore) AppendAIMessage(_ context.Context, id, conversationID, userID, role, content string, now time.Time) (state.AIMessageRecord, error) {
	conversation, err := s.AIConversation(context.Background(), conversationID, userID)
	if err != nil {
		return state.AIMessageRecord{}, err
	}
	if conversation.ClosedAt != nil {
		return state.AIMessageRecord{}, state.ErrAIConversationClosed
	}
	item := state.AIMessageRecord{
		ID: id, ConversationID: conversationID, Role: role, Content: content, CreatedAt: now,
	}
	s.messages[conversationID] = append(s.messages[conversationID], item)
	return item, nil
}

func (s *chatMemoryStore) ListAIMessages(_ context.Context, conversationID, userID string, limit int) ([]state.AIMessageRecord, error) {
	if _, err := s.AIConversation(context.Background(), conversationID, userID); err != nil {
		return nil, err
	}
	items := s.messages[conversationID]
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return append([]state.AIMessageRecord(nil), items...), nil
}

func (s *chatMemoryStore) CreateAIToolAction(
	_ context.Context,
	id, conversationID, userID, toolID, toolName, sensitivity string,
	input json.RawMessage,
	now time.Time,
) (state.AIToolActionRecord, error) {
	conversation, err := s.AIConversation(context.Background(), conversationID, userID)
	if err != nil {
		return state.AIToolActionRecord{}, err
	}
	if conversation.ClosedAt != nil {
		return state.AIToolActionRecord{}, state.ErrAIConversationNotFound
	}
	item := state.AIToolActionRecord{
		ID: id, ConversationID: conversationID, UserID: userID, ToolID: toolID,
		ToolName: toolName, Sensitivity: sensitivity, Input: append(json.RawMessage(nil), input...),
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	s.actions[id] = item
	return item, nil
}

func (s *chatMemoryStore) ListAIToolActions(_ context.Context, conversationID, userID string, _ int) ([]state.AIToolActionRecord, error) {
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

func (s *chatMemoryStore) ClaimAIToolAction(_ context.Context, id, conversationID, userID string, now time.Time) (state.AIToolActionRecord, error) {
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

func (s *chatMemoryStore) RejectAIToolAction(_ context.Context, id, conversationID, userID string, now time.Time) (state.AIToolActionRecord, error) {
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

func (s *chatMemoryStore) FinishAIToolAction(
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

type scriptedProvider struct {
	responses []ModelResponse
	requests  []ModelRequest
}

func (p *scriptedProvider) ID() string { return "scripted" }

func (p *scriptedProvider) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	select {
	case <-ctx.Done():
		return ModelResponse{}, ctx.Err()
	default:
	}
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		return ModelResponse{}, errors.New("scripted provider has no response")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func TestServiceModelAutomaticallyUsesReadTool(t *testing.T) {
	store := newChatMemoryStore()
	provider := &scriptedProvider{responses: []ModelResponse{
		{
			Message: Message{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{{
					ID: "call-1", ToolID: "core.modules.list", Input: json.RawMessage(`{}`),
				}},
			},
			ToolCalls: []ToolCall{{
				ID: "call-1", ToolID: "core.modules.list", Input: json.RawMessage(`{}`),
			}},
		},
		{Message: Message{Role: RoleAssistant, Content: "The module list was inspected."}},
	}}
	service := NewService("node-1", store, serviceJobs{}, serviceModules{
		items:        []modules.Registered{{Manifest: modules.Manifest{ID: "ai.agent", Name: "AI Agent", Version: "0.4.0"}}},
		capabilities: []string{"ai.agent"},
	}, nil, provider)
	actor := security.Actor{Type: "user", ID: "usr-1", Permissions: []string{"modules.read"}}
	conversation, err := service.CreateConversation(context.Background(), actor, security.RequestContext{}, "Inspect")
	if err != nil {
		t.Fatal(err)
	}
	_, assistant, err := service.Chat(context.Background(), actor, security.RequestContext{}, conversation.ID, "Which modules are active?")
	if err != nil {
		t.Fatal(err)
	}
	if assistant.Content != "The module list was inspected." {
		t.Fatalf("assistant = %#v", assistant)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(provider.requests))
	}
	foundToolResult := false
	for _, message := range provider.requests[1].Messages {
		if message.Role == RoleTool && strings.Contains(message.Content, "core.modules.list") {
			foundToolResult = true
		}
	}
	if !foundToolResult {
		t.Fatalf("second provider request did not contain tool result: %#v", provider.requests[1].Messages)
	}
}

func TestServiceChangeToolWaitsForApprovalThenExecutesOnce(t *testing.T) {
	store := newChatMemoryStore()
	provider := &scriptedProvider{responses: []ModelResponse{{
		Message: Message{
			Role:    RoleAssistant,
			Content: "I prepared a server change for approval.",
			ToolCalls: []ToolCall{{
				ID: "call-change", ToolID: "test.server.change", Input: json.RawMessage(`{"value":"on"}`),
			}},
		},
		ToolCalls: []ToolCall{{
			ID: "call-change", ToolID: "test.server.change", Input: json.RawMessage(`{"value":"on"}`),
		}},
	}}}
	service := NewService("node-1", store, serviceJobs{}, serviceModules{}, nil, provider)
	executions := 0
	if err := service.registry.Register(ToolDescriptor{
		ID: "test.server.change", ModuleID: "ai.agent", Name: "Test server change",
		Description:         "Controlled test change",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`),
		RequiredPermissions: []string{"modules.manage"},
		Sensitivity:         SensitivityChange,
	}, func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
		executions++
		return json.RawMessage(`{"ok":true}`), nil
	}); err != nil {
		t.Fatal(err)
	}

	actor := security.Actor{Type: "user", ID: "usr-1", Permissions: []string{"modules.manage"}}
	conversation, err := service.CreateConversation(context.Background(), actor, security.RequestContext{}, "Configure")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Chat(context.Background(), actor, security.RequestContext{}, conversation.ID, "Enable the setting"); err != nil {
		t.Fatal(err)
	}
	if executions != 0 {
		t.Fatalf("change executed before approval: %d", executions)
	}
	actions, err := service.Actions(context.Background(), actor, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Status != "pending" || actions[0].ToolID != "test.server.change" {
		t.Fatalf("actions = %#v", actions)
	}

	approved, err := service.ApproveAction(
		context.Background(), actor, security.RequestContext{RequestID: "approve-1"}, conversation.ID, actions[0].ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "executed" || executions != 1 {
		t.Fatalf("approved = %#v executions=%d", approved, executions)
	}
	if _, err := service.ApproveAction(context.Background(), actor, security.RequestContext{}, conversation.ID, actions[0].ID); !errors.Is(err, state.ErrAIToolActionNotPending) {
		t.Fatalf("second approval error = %v", err)
	}
	if executions != 1 {
		t.Fatalf("change executed more than once: %d", executions)
	}
}

func TestServiceApprovalRechecksCurrentPermission(t *testing.T) {
	store := newChatMemoryStore()
	provider := &scriptedProvider{responses: []ModelResponse{{
		Message: Message{Role: RoleAssistant, ToolCalls: []ToolCall{{
			ID: "call-change", ToolID: "test.permission.change", Input: json.RawMessage(`{}`),
		}}},
		ToolCalls: []ToolCall{{ID: "call-change", ToolID: "test.permission.change", Input: json.RawMessage(`{}`)}},
	}}}
	service := NewService("node-1", store, serviceJobs{}, serviceModules{}, nil, provider)
	executions := 0
	if err := service.registry.Register(ToolDescriptor{
		ID: "test.permission.change", ModuleID: "ai.agent", Name: "Permission change test",
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false}`),
		RequiredPermissions: []string{"modules.manage"}, Sensitivity: SensitivityChange,
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		executions++
		return json.RawMessage(`{"ok":true}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	creator := security.Actor{Type: "user", ID: "usr-1", Permissions: []string{"modules.manage"}}
	conversation, err := service.CreateConversation(context.Background(), creator, security.RequestContext{}, "Permission")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Chat(context.Background(), creator, security.RequestContext{}, conversation.ID, "Prepare it"); err != nil {
		t.Fatal(err)
	}
	actions, err := service.Actions(context.Background(), creator, conversation.ID)
	if err != nil || len(actions) != 1 {
		t.Fatalf("actions = %#v err=%v", actions, err)
	}

	revoked := security.Actor{Type: "user", ID: "usr-1"}
	failed, err := service.ApproveAction(context.Background(), revoked, security.RequestContext{}, conversation.ID, actions[0].ID)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("approval error = %v, want permission denied", err)
	}
	if failed.Status != "failed" || failed.ErrorCode != "permission_denied" || executions != 0 {
		t.Fatalf("failed action = %#v executions=%d", failed, executions)
	}
}

func TestServicePersistentChatUsesProviderAndRedactsAuditText(t *testing.T) {
	store := newChatMemoryStore()
	audit := &serviceAudit{}
	provider := DeterministicProvider{
		ProviderID: "test-local",
		Response:   ModelResponse{Message: Message{Role: RoleAssistant, Content: "Local assistant reply"}},
	}
	service := NewService("node-1", store, serviceJobs{}, serviceModules{}, audit, provider)
	actor := security.Actor{Type: "user", ID: "usr-1"}

	conversation, err := service.CreateConversation(
		context.Background(), actor, security.RequestContext{RequestID: "req-create"}, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	userMessage, assistantMessage, err := service.Chat(
		context.Background(),
		actor,
		security.RequestContext{RequestID: "req-chat"},
		conversation.ID,
		"How is Home AI?",
	)
	if err != nil {
		t.Fatal(err)
	}
	if userMessage.Content != "How is Home AI?" || assistantMessage.Content != "Local assistant reply" {
		t.Fatalf("chat messages = %#v %#v", userMessage, assistantMessage)
	}
	updated, err := store.AIConversation(context.Background(), conversation.ID, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title == "" {
		t.Fatal("first message did not set conversation title")
	}

	messages, err := service.Messages(context.Background(), actor, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	if _, err := service.Messages(context.Background(), security.Actor{ID: "usr-2"}, conversation.ID); !errors.Is(err, state.ErrAIConversationNotFound) {
		t.Fatalf("foreign Messages() error = %v", err)
	}

	var chatAudit *auditCall
	for i := range audit.calls {
		if audit.calls[i].action == "ai.chat.generate" {
			chatAudit = &audit.calls[i]
			break
		}
	}
	if chatAudit == nil {
		t.Fatalf("chat audit missing: %#v", audit.calls)
	}
	if chatAudit.outcome != "success" {
		t.Fatalf("chat audit = %#v", chatAudit)
	}
	raw, err := json.Marshal(chatAudit.metadata)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("How is Home AI?")) || bytes.Contains(raw, []byte("Local assistant reply")) {
		t.Fatalf("chat text leaked into audit metadata: %s", raw)
	}
}

func TestServiceCloseConversationMakesHistoryReadOnly(t *testing.T) {
	store := newChatMemoryStore()
	audit := &serviceAudit{}
	provider := DeterministicProvider{
		ProviderID: "test-local",
		Response:   ModelResponse{Message: Message{Role: RoleAssistant, Content: "reply"}},
	}
	service := NewService("node-1", store, serviceJobs{}, serviceModules{}, audit, provider)
	actor := security.Actor{Type: "user", ID: "usr-1"}

	conversation, err := service.CreateConversation(context.Background(), actor, security.RequestContext{}, "Close me")
	if err != nil {
		t.Fatal(err)
	}
	closed, err := service.CloseConversation(
		context.Background(),
		actor,
		security.RequestContext{RequestID: "req-close"},
		conversation.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if closed.ClosedAt == nil {
		t.Fatal("closed_at was not set")
	}
	if _, _, err := service.Chat(context.Background(), actor, security.RequestContext{}, conversation.ID, "after close"); !errors.Is(err, state.ErrAIConversationClosed) {
		t.Fatalf("Chat() error = %v, want conversation closed", err)
	}
	messages, err := service.Messages(context.Background(), actor, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %#v", messages)
	}
	foundAudit := false
	for _, call := range audit.calls {
		if call.action == "ai.conversation.close" && call.targetID == conversation.ID && call.outcome == "success" {
			foundAudit = true
		}
	}
	if !foundAudit {
		t.Fatalf("close audit missing: %#v", audit.calls)
	}
}

type blockingProvider struct {
	started chan struct{}
}

func (p blockingProvider) ID() string { return "blocking" }

func (p blockingProvider) Generate(ctx context.Context, _ ModelRequest) (ModelResponse, error) {
	select {
	case p.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ModelResponse{}, ctx.Err()
}

func TestServiceDisableBlocksToolsAndChat(t *testing.T) {
	store := newChatMemoryStore()
	service := NewService("node-1", store, serviceJobs{}, serviceModules{}, nil)
	actor := security.Actor{Type: "user", ID: "usr-1", Permissions: []string{"system.read"}}

	service.SetEnabled(false)
	if service.Enabled() {
		t.Fatal("service remained enabled")
	}
	if service.Status().State != "disabled" {
		t.Fatalf("status = %#v", service.Status())
	}
	if tools := service.AvailableTools(actor); len(tools) != 0 {
		t.Fatalf("disabled tools = %#v", tools)
	}
	if _, err := service.Conversations(context.Background(), actor); !errors.Is(err, ErrAgentDisabled) {
		t.Fatalf("Conversations() error = %v, want agent disabled", err)
	}
	if _, err := service.Execute(context.Background(), actor, security.RequestContext{}, "core.system.status", nil, false); !errors.Is(err, ErrAgentDisabled) {
		t.Fatalf("Execute() error = %v, want agent disabled", err)
	}

	service.SetEnabled(true)
	if !service.Enabled() || service.Status().State == "disabled" {
		t.Fatalf("service did not re-enable: %#v", service.Status())
	}
}

func TestServiceRestartCancelsActiveGeneration(t *testing.T) {
	store := newChatMemoryStore()
	started := make(chan struct{}, 1)
	service := NewService("node-1", store, serviceJobs{}, serviceModules{}, nil, blockingProvider{started: started})
	actor := security.Actor{Type: "user", ID: "usr-1"}

	conversation, err := service.CreateConversation(context.Background(), actor, security.RequestContext{}, "restart test")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := service.Chat(context.Background(), actor, security.RequestContext{}, conversation.ID, "wait")
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider did not start")
	}

	service.Restart()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Chat() error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restart did not cancel active generation")
	}
	if !service.Enabled() {
		t.Fatal("service disabled after restart")
	}
}

func TestServiceChatUnavailableWithoutProvider(t *testing.T) {
	store := newChatMemoryStore()
	service := NewService("node-1", store, serviceJobs{}, serviceModules{}, nil)
	actor := security.Actor{Type: "user", ID: "usr-1"}
	conversation, err := service.CreateConversation(context.Background(), actor, security.RequestContext{}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.Chat(context.Background(), actor, security.RequestContext{}, conversation.ID, "hello")
	if !errors.Is(err, ErrChatUnavailable) {
		t.Fatalf("Chat() error = %v, want chat unavailable", err)
	}
}
