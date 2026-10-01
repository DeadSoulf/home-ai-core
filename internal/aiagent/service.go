package aiagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
	"github.com/DeadSoulf/home-ai-core/internal/version"
)

const defaultToolTimeout = 10 * time.Second

type StateReader interface {
	SchemaVersion(context.Context) (int, error)
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
	nodeID      string
	registry    *ToolRegistry
	state       StateReader
	jobs        JobReader
	modules     ModuleReader
	audit       AuditRecorder
	toolTimeout time.Duration
	initErr     error
}

type Status struct {
	ModuleID           string `json:"module_id"`
	State              string `json:"state"`
	Version            string `json:"version"`
	ToolCount          int    `json:"tool_count"`
	ProviderConfigured bool   `json:"provider_configured"`
}

func NewService(nodeID string, stateReader StateReader, jobReader JobReader, moduleReader ModuleReader, auditRecorder AuditRecorder) *Service {
	s := &Service{nodeID: nodeID, registry: NewToolRegistry(), state: stateReader, jobs: jobReader, modules: moduleReader, audit: auditRecorder, toolTimeout: defaultToolTimeout}
	s.initErr = s.registerCoreReadTools()
	return s
}

func (s *Service) Status() Status {
	stateName := "ready"
	if s.initErr != nil { stateName = "error" }
	return Status{ModuleID: "ai.agent", State: stateName, Version: version.Version, ToolCount: len(s.registry.List()), ProviderConfigured: false}
}

func (s *Service) AvailableTools(principal Principal) []ToolDescriptor {
	all := s.registry.List()
	available := make([]ToolDescriptor, 0, len(all))
	for _, descriptor := range all {
		if toolAllowed(principal, descriptor) { available = append(available, descriptor) }
	}
	return available
}

func (s *Service) Execute(ctx context.Context, actor security.Actor, meta security.RequestContext, toolID string, input json.RawMessage, approved bool) (json.RawMessage, error) {
	if s.initErr != nil { return nil, s.initErr }
	descriptor, _ := s.registry.Descriptor(toolID)
	started := time.Now()
	toolCtx, cancel := context.WithTimeout(ctx, s.toolTimeout)
	defer cancel()
	result, err := s.registry.Execute(toolCtx, actor, toolID, input, approved)
	outcome := "success"
	if err != nil {
		outcome = "failed"
		if errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrApprovalRequired) { outcome = "denied" }
	}
	s.recordAudit(ctx, meta, actor, descriptor, toolID, approved, outcome, err, time.Since(started))
	return result, err
}

func (s *Service) recordAudit(ctx context.Context, meta security.RequestContext, actor security.Actor, descriptor ToolDescriptor, toolID string, approved bool, outcome string, runErr error, duration time.Duration) {
	if s.audit == nil { return }
	metadata := map[string]any{"tool_id": toolID, "approved": approved, "duration_ms": duration.Milliseconds()}
	if descriptor.Sensitivity != "" { metadata["sensitivity"] = descriptor.Sensitivity }
	if runErr != nil { metadata["error_code"] = toolErrorCode(runErr) }
	s.audit.RecordAudit(context.WithoutCancel(ctx), meta, actor, "ai.tool.execute", "ai_tool", toolID, outcome, metadata)
}

func toolErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrToolNotFound): return "tool_not_found"
	case errors.Is(err, ErrPermissionDenied): return "permission_denied"
	case errors.Is(err, ErrApprovalRequired): return "approval_required"
	case errors.Is(err, ErrInvalidToolInput): return "invalid_input"
	case errors.Is(err, context.DeadlineExceeded): return "timeout"
	case errors.Is(err, context.Canceled): return "cancelled"
	default: return "tool_failed"
	}
}

func (s *Service) registerCoreReadTools() error {
	tools := []struct { descriptor ToolDescriptor; handler ToolHandler }{
		{ToolDescriptor{ID: "core.system.status", ModuleID: "ai.agent", Name: "Home-AI system status", Description: "Read current Core version, schema and local hardware/system status.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), RequiredPermissions: []string{"system.read"}, Sensitivity: SensitivityRead}, s.systemStatus},
		{ToolDescriptor{ID: "core.jobs.list", ModuleID: "ai.agent", Name: "Home-AI jobs", Description: "List recent Core jobs without exposing job input/result payloads.", InputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","maxLength":32},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`), RequiredPermissions: []string{"jobs.read"}, Sensitivity: SensitivityRead}, s.jobsList},
		{ToolDescriptor{ID: "core.modules.list", ModuleID: "ai.agent", Name: "Home-AI modules", Description: "List registered modules and current platform capabilities.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), RequiredPermissions: []string{"modules.read"}, Sensitivity: SensitivityRead}, s.modulesList},
	}
	for _, tool := range tools { if err := s.registry.Register(tool.descriptor, tool.handler); err != nil { return err } }
	return nil
}

func (s *Service) systemStatus(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if err := decodeEmptyObject(input); err != nil { return nil, err }
	if s.state == nil { return nil, errors.New("Core state reader is unavailable") }
	schemaVersion, err := s.state.SchemaVersion(ctx)
	if err != nil { return nil, fmt.Errorf("read Core schema version: %w", err) }
	info := systeminfo.Collect(s.nodeID)
	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if inspection, inspectErr := storage.Inspect(inspectCtx); inspectErr == nil {
		systeminfo.ApplyFilesystemStats(&info, inspection.Filesystems)
		systeminfo.ApplyStorageDetails(&info, inspection.DiskHealth, inspection.LVM)
	}
	cancel()
	return json.Marshal(map[string]any{"version": version.Version, "schema_version": schemaVersion, "system": info})
}

type jobsListInput struct { Status string `json:"status,omitempty"`; Limit int `json:"limit,omitempty"` }
type jobSummary struct {
	ID string `json:"id"`; NodeID string `json:"node_id"`; Type string `json:"type"`; Status string `json:"status"`; Progress int `json:"progress"`; Message string `json:"message,omitempty"`; ErrorCode string `json:"error_code,omitempty"`; ErrorMessage string `json:"error_message,omitempty"`; CreatedAt time.Time `json:"created_at"`; StartedAt *time.Time `json:"started_at,omitempty"`; CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (s *Service) jobsList(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if s.jobs == nil { return nil, errors.New("Core job reader is unavailable") }
	var request jobsListInput
	if err := decodeToolInput(input, &request); err != nil { return nil, err }
	if request.Limit == 0 { request.Limit = 20 }
	if request.Limit < 1 || request.Limit > 100 { return nil, ErrInvalidToolInput }
	jobs, err := s.jobs.List(ctx, request.Status, request.Limit)
	if err != nil { return nil, fmt.Errorf("list Core jobs: %w", err) }
	result := make([]jobSummary, 0, len(jobs))
	for _, job := range jobs { result = append(result, jobSummary{ID: job.ID, NodeID: job.NodeID, Type: job.Type, Status: job.Status, Progress: job.Progress, Message: job.Message, ErrorCode: job.ErrorCode, ErrorMessage: job.ErrorMessage, CreatedAt: job.CreatedAt, StartedAt: job.StartedAt, CompletedAt: job.CompletedAt}) }
	return json.Marshal(map[string]any{"jobs": result})
}

func (s *Service) modulesList(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	if err := decodeEmptyObject(input); err != nil { return nil, err }
	if s.modules == nil { return nil, errors.New("Core module reader is unavailable") }
	items, err := s.modules.List(ctx)
	if err != nil { return nil, fmt.Errorf("list Core modules: %w", err) }
	capabilities, err := s.modules.Capabilities(ctx)
	if err != nil { return nil, fmt.Errorf("list Core capabilities: %w", err) }
	return json.Marshal(map[string]any{"modules": items, "capabilities": capabilities})
}

func decodeEmptyObject(input json.RawMessage) error {
	if len(input) == 0 { return nil }
	var value map[string]json.RawMessage
	if err := decodeToolInput(input, &value); err != nil { return err }
	if len(value) != 0 { return ErrInvalidToolInput }
	return nil
}

func decodeToolInput(input json.RawMessage, target any) error {
	if len(input) == 0 { input = json.RawMessage(`{}`) }
	if !json.Valid(input) { return ErrInvalidToolInput }
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil { return ErrInvalidToolInput }
	return nil
}
