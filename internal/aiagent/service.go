package aiagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
	"github.com/DeadSoulf/home-ai-core/internal/version"
)

const (
	defaultToolTimeout      = 10 * time.Second
	defaultChatTimeout      = 90 * time.Second
	maxChatMessageRunes     = 8000
	maxChatResponseRunes    = 32000
	maxChatContextMessages  = 24
	maxChatContextRunes     = 32000
)

var (
	ErrChatUnavailable    = errors.New("AI chat is unavailable")
	ErrInvalidChatMessage = errors.New("invalid AI chat message")
)

type StateReader interface {
	SchemaVersion(context.Context) (int, error)
}

type ConversationStore interface {
	CreateAIConversation(context.Context, string, string, string, time.Time) (state.AIConversationRecord, error)
	ListAIConversations(context.Context, string, int) ([]state.AIConversationRecord, error)
	AIConversation(context.Context, string, string) (state.AIConversationRecord, error)
	UpdateAIConversationTitle(context.Context, string, string, string, time.Time) error
	AppendAIMessage(context.Context, string, string, string, string, string, time.Time) (state.AIMessageRecord, error)
	ListAIMessages(context.Context, string, string, int) ([]state.AIMessageRecord, error)
}

type JobReader interface {
	List(context.Context, string, int) ([]state.JobRecord, error)
}

type ModuleReader interface {
	List(context.Context) ([]modules.Registered, error)
	Capabilities(context.Context) ([]string, error)
}

type AuditRecorder interface {
	RecordAudit(context.Context, security.RequestContext, security.Actor, string, string, string, string, map[string]any)
}

type Service struct {
	nodeID        string
	registry      *ToolRegistry
	state         StateReader
	jobs          JobReader
	modules       ModuleReader
	audit         AuditRecorder
	conversations ConversationStore
	provider      Provider
	toolTimeout   time.Duration
	chatTimeout   time.Duration
	initErr       error
}

type Status struct {
	ModuleID               string `json:"module_id"`
	State                  string `json:"state"`
	Version                string `json:"version"`
	ToolCount              int    `json:"tool_count"`
	ProviderConfigured     bool   `json:"provider_configured"`
	ProviderID             string `json:"provider_id,omitempty"`
	ProviderModel          string `json:"provider_model,omitempty"`
	ConversationStoreReady bool   `json:"conversation_store_ready"`
}

func NewService(
	nodeID string,
	stateReader StateReader,
	jobReader JobReader,
	moduleReader ModuleReader,
	auditRecorder AuditRecorder,
	providers ...Provider,
) *Service {
	conversations, _ := stateReader.(ConversationStore)
	var provider Provider
	if len(providers) > 0 {
		provider = providers[0]
	}
	s := &Service{
		nodeID: nodeID, registry: NewToolRegistry(), state: stateReader, jobs: jobReader, modules: moduleReader,
		audit: auditRecorder, conversations: conversations, provider: provider,
		toolTimeout: defaultToolTimeout, chatTimeout: defaultChatTimeout,
	}
	s.initErr = s.registerCoreReadTools()
	return s
}

func (s *Service) Status() Status {
	stateName := "ready"
	if s.initErr != nil {
		stateName = "error"
	}
	status := Status{
		ModuleID:               "ai.agent",
		State:                  stateName,
		Version:                version.Version,
		ToolCount:              len(s.registry.List()),
		ProviderConfigured:     s.provider != nil,
		ConversationStoreReady: s.conversations != nil,
	}
	if s.provider != nil {
		status.ProviderID = s.provider.ID()
		if modelProvider, ok := s.provider.(interface{ Model() string }); ok {
			status.ProviderModel = modelProvider.Model()
		}
	}
	return status
}

func (s *Service) Conversations(ctx context.Context, actor security.Actor) ([]state.AIConversationRecord, error) {
	if s.conversations == nil || actor.ID == "" {
		return nil, ErrChatUnavailable
	}
	return s.conversations.ListAIConversations(ctx, actor.ID, 50)
}

func (s *Service) CreateConversation(
	ctx context.Context,
	actor security.Actor,
	meta security.RequestContext,
	title string,
) (state.AIConversationRecord, error) {
	if s.conversations == nil || actor.ID == "" {
		return state.AIConversationRecord{}, ErrChatUnavailable
	}
	title = cleanConversationTitle(title)
	id, err := newAgentID("aic_")
	if err != nil {
		return state.AIConversationRecord{}, err
	}
	now := time.Now().UTC()
	record, err := s.conversations.CreateAIConversation(ctx, id, actor.ID, title, now)
	if err == nil && s.audit != nil {
		s.audit.RecordAudit(
			context.WithoutCancel(ctx), meta, actor,
			"ai.conversation.create", "ai_conversation", record.ID, "success",
			map[string]any{"has_title": title != ""},
		)
	}
	return record, err
}

func (s *Service) Messages(
	ctx context.Context,
	actor security.Actor,
	conversationID string,
) ([]state.AIMessageRecord, error) {
	if s.conversations == nil || actor.ID == "" {
		return nil, ErrChatUnavailable
	}
	return s.conversations.ListAIMessages(ctx, conversationID, actor.ID, 100)
}

func (s *Service) Chat(
	ctx context.Context,
	actor security.Actor,
	meta security.RequestContext,
	conversationID, content string,
) (state.AIMessageRecord, state.AIMessageRecord, error) {
	if s.conversations == nil || s.provider == nil || actor.ID == "" {
		return state.AIMessageRecord{}, state.AIMessageRecord{}, ErrChatUnavailable
	}
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > maxChatMessageRunes {
		return state.AIMessageRecord{}, state.AIMessageRecord{}, ErrInvalidChatMessage
	}
	conversation, err := s.conversations.AIConversation(ctx, conversationID, actor.ID)
	if err != nil {
		return state.AIMessageRecord{}, state.AIMessageRecord{}, err
	}

	userMessageID, err := newAgentID("aim_")
	if err != nil {
		return state.AIMessageRecord{}, state.AIMessageRecord{}, err
	}
	now := time.Now().UTC()
	userMessage, err := s.conversations.AppendAIMessage(
		ctx, userMessageID, conversation.ID, actor.ID, string(RoleUser), content, now,
	)
	if err != nil {
		return state.AIMessageRecord{}, state.AIMessageRecord{}, err
	}
	if conversation.Title == "" {
		title := cleanConversationTitle(content)
		if title != "" {
			_ = s.conversations.UpdateAIConversationTitle(ctx, conversation.ID, actor.ID, title, now)
		}
	}

	history, err := s.conversations.ListAIMessages(ctx, conversation.ID, actor.ID, maxChatContextMessages)
	if err != nil {
		return userMessage, state.AIMessageRecord{}, err
	}
	request := ModelRequest{Messages: buildChatContext(history)}
	started := time.Now()
	chatCtx, cancel := context.WithTimeout(ctx, s.chatTimeout)
	response, generateErr := s.provider.Generate(chatCtx, request)
	cancel()
	if generateErr != nil {
		s.recordChatAudit(ctx, meta, actor, conversation.ID, content, "", time.Since(started), generateErr)
		return userMessage, state.AIMessageRecord{}, generateErr
	}

	answer := strings.TrimSpace(response.Message.Content)
	if response.Message.Role != RoleAssistant || answer == "" || utf8.RuneCountInString(answer) > maxChatResponseRunes {
		err := errors.New("AI provider returned an invalid assistant response")
		s.recordChatAudit(ctx, meta, actor, conversation.ID, content, answer, time.Since(started), err)
		return userMessage, state.AIMessageRecord{}, err
	}
	assistantMessageID, err := newAgentID("aim_")
	if err != nil {
		return userMessage, state.AIMessageRecord{}, err
	}
	assistantMessage, err := s.conversations.AppendAIMessage(
		ctx, assistantMessageID, conversation.ID, actor.ID, string(RoleAssistant), answer, time.Now().UTC(),
	)
	if err != nil {
		s.recordChatAudit(ctx, meta, actor, conversation.ID, content, answer, time.Since(started), err)
		return userMessage, state.AIMessageRecord{}, err
	}
	s.recordChatAudit(ctx, meta, actor, conversation.ID, content, answer, time.Since(started), nil)
	return userMessage, assistantMessage, nil
}

func buildChatContext(history []state.AIMessageRecord) []Message {
	selected := make([]state.AIMessageRecord, 0, len(history))
	runes := 0
	for i := len(history) - 1; i >= 0; i-- {
		count := utf8.RuneCountInString(history[i].Content)
		if len(selected) > 0 && runes+count > maxChatContextRunes {
			break
		}
		selected = append(selected, history[i])
		runes += count
	}
	messages := []Message{{
		Role: RoleSystem,
		Content: "You are the local Home-AI assistant. Answer clearly and conservatively. " +
			"This chat slice does not automatically execute Home-AI tools or system actions. " +
			"Never claim an action was executed unless a tool result is explicitly present in the conversation.",
	}}
	for i := len(selected) - 1; i >= 0; i-- {
		role := RoleUser
		if selected[i].Role == string(RoleAssistant) {
			role = RoleAssistant
		}
		messages = append(messages, Message{Role: role, Content: selected[i].Content})
	}
	return messages
}

func (s *Service) recordChatAudit(
	ctx context.Context,
	meta security.RequestContext,
	actor security.Actor,
	conversationID, input, output string,
	duration time.Duration,
	runErr error,
) {
	if s.audit == nil {
		return
	}
	outcome := "success"
	metadata := map[string]any{
		"provider":       s.provider.ID(),
		"duration_ms":    duration.Milliseconds(),
		"input_chars":    utf8.RuneCountInString(input),
		"output_chars":   utf8.RuneCountInString(output),
	}
	if modelProvider, ok := s.provider.(interface{ Model() string }); ok {
		metadata["model"] = modelProvider.Model()
	}
	if runErr != nil {
		outcome = "failed"
		switch {
		case errors.Is(runErr, context.DeadlineExceeded):
			metadata["error_code"] = "timeout"
		case errors.Is(runErr, context.Canceled):
			metadata["error_code"] = "cancelled"
		default:
			metadata["error_code"] = "provider_failed"
		}
	}
	s.audit.RecordAudit(
		context.WithoutCancel(ctx), meta, actor,
		"ai.chat.generate", "ai_conversation", conversationID, outcome, metadata,
	)
}

func cleanConversationTitle(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	runes := []rune(value)
	if len(runes) > 80 {
		value = string(runes[:80]) + "…"
	}
	return value
}

func newAgentID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate AI id: %w", err)
	}
	return prefix + hex.EncodeToString(raw), nil
}

func (s *Service) AvailableTools(principal Principal) []ToolDescriptor {
	all := s.registry.List()
	available := make([]ToolDescriptor, 0, len(all))
	for _, descriptor := range all {
		if toolAllowed(principal, descriptor) {
			available = append(available, descriptor)
		}
	}
	return available
}

func (s *Service) Execute(ctx context.Context, actor security.Actor, meta security.RequestContext, toolID string, input json.RawMessage, approved bool) (json.RawMessage, error) {
	if s.initErr != nil {
		return nil, s.initErr
	}
	descriptor, _ := s.registry.Descriptor(toolID)
	started := time.Now()
	toolCtx, cancel := context.WithTimeout(ctx, s.toolTimeout)
	defer cancel()
	result, err := s.registry.Execute(toolCtx, actor, toolID, input, approved)
	outcome := "success"
	if err != nil {
		outcome = "failed"
		if errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrApprovalRequired) {
			outcome = "denied"
		}
	}
	s.recordAudit(ctx, meta, actor, descriptor, toolID, approved, outcome, err, time.Since(started))
	return result, err
}

func (s *Service) recordAudit(ctx context.Context, meta security.RequestContext, actor security.Actor, descriptor ToolDescriptor, toolID string, approved bool, outcome string, runErr error, duration time.Duration) {
	if s.audit == nil {
		return
	}
	metadata := map[string]any{"tool_id": toolID, "approved": approved, "duration_ms": duration.Milliseconds()}
	if descriptor.Sensitivity != "" {
		metadata["sensitivity"] = descriptor.Sensitivity
	}
	if runErr != nil {
		metadata["error_code"] = toolErrorCode(runErr)
	}
	s.audit.RecordAudit(context.WithoutCancel(ctx), meta, actor, "ai.tool.execute", "ai_tool", toolID, outcome, metadata)
}

func toolErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrToolNotFound):
		return "tool_not_found"
	case errors.Is(err, ErrPermissionDenied):
		return "permission_denied"
	case errors.Is(err, ErrApprovalRequired):
		return "approval_required"
	case errors.Is(err, ErrInvalidToolInput):
		return "invalid_input"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "tool_failed"
	}
}

func (s *Service) registerCoreReadTools() error {
	tools := []struct {
		descriptor ToolDescriptor
		handler    ToolHandler
	}{
		{ToolDescriptor{ID: "core.system.status", ModuleID: "ai.agent", Name: "Home-AI system status", Description: "Read current Core version, schema and local hardware/system status.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), RequiredPermissions: []string{"system.read"}, Sensitivity: SensitivityRead}, s.systemStatus},
		{ToolDescriptor{ID: "core.jobs.list", ModuleID: "ai.agent", Name: "Home-AI jobs", Description: "List recent Core jobs without exposing job input/result payloads.", InputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","maxLength":32},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`), RequiredPermissions: []string{"jobs.read"}, Sensitivity: SensitivityRead}, s.jobsList},
		{ToolDescriptor{ID: "core.modules.list", ModuleID: "ai.agent", Name: "Home-AI modules", Description: "List registered modules and current platform capabilities.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), RequiredPermissions: []string{"modules.read"}, Sensitivity: SensitivityRead}, s.modulesList},
	}
	for _, tool := range tools {
		if err := s.registry.Register(tool.descriptor, tool.handler); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) systemStatus(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if err := decodeEmptyObject(input); err != nil {
		return nil, err
	}
	if s.state == nil {
		return nil, errors.New("Core state reader is unavailable")
	}
	schemaVersion, err := s.state.SchemaVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("read Core schema version: %w", err)
	}
	info := systeminfo.Collect(s.nodeID)
	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if inspection, inspectErr := storage.Inspect(inspectCtx); inspectErr == nil {
		systeminfo.ApplyFilesystemStats(&info, inspection.Filesystems)
		systeminfo.ApplyStorageDetails(&info, inspection.DiskHealth, inspection.LVM)
	}
	cancel()
	return json.Marshal(map[string]any{"version": version.Version, "schema_version": schemaVersion, "system": info})
}

type jobsListInput struct {
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type jobSummary struct {
	ID           string     `json:"id"`
	NodeID       string     `json:"node_id"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	Progress     int        `json:"progress"`
	Message      string     `json:"message,omitempty"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

func (s *Service) jobsList(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if s.jobs == nil {
		return nil, errors.New("Core job reader is unavailable")
	}
	var request jobsListInput
	if err := decodeToolInput(input, &request); err != nil {
		return nil, err
	}
	if request.Limit == 0 {
		request.Limit = 20
	}
	if request.Limit < 1 || request.Limit > 100 {
		return nil, ErrInvalidToolInput
	}
	jobs, err := s.jobs.List(ctx, request.Status, request.Limit)
	if err != nil {
		return nil, fmt.Errorf("list Core jobs: %w", err)
	}
	result := make([]jobSummary, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, jobSummary{ID: job.ID, NodeID: job.NodeID, Type: job.Type, Status: job.Status, Progress: job.Progress, Message: job.Message, ErrorCode: job.ErrorCode, ErrorMessage: job.ErrorMessage, CreatedAt: job.CreatedAt, StartedAt: job.StartedAt, CompletedAt: job.CompletedAt})
	}
	return json.Marshal(map[string]any{"jobs": result})
}

func (s *Service) modulesList(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if err := decodeEmptyObject(input); err != nil {
		return nil, err
	}
	if s.modules == nil {
		return nil, errors.New("Core module reader is unavailable")
	}
	items, err := s.modules.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Core modules: %w", err)
	}
	capabilities, err := s.modules.Capabilities(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Core capabilities: %w", err)
	}
	return json.Marshal(map[string]any{"modules": items, "capabilities": capabilities})
}

func decodeEmptyObject(input json.RawMessage) error {
	if len(input) == 0 {
		return nil
	}
	var value map[string]json.RawMessage
	if err := decodeToolInput(input, &value); err != nil {
		return err
	}
	if len(value) != 0 {
		return ErrInvalidToolInput
	}
	return nil
}

func decodeToolInput(input json.RawMessage, target any) error {
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	if !json.Valid(input) {
		return ErrInvalidToolInput
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrInvalidToolInput
	}
	return nil
}
