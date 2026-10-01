package aiagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type serviceState struct { schema int }
func (s serviceState) SchemaVersion(context.Context) (int, error) { return s.schema, nil }

type serviceJobs struct { jobs []state.JobRecord }
func (s serviceJobs) List(context.Context, string, int) ([]state.JobRecord, error) { return s.jobs, nil }

type serviceModules struct { items []modules.Registered; capabilities []string }
func (s serviceModules) List(context.Context) ([]modules.Registered, error) { return s.items, nil }
func (s serviceModules) Capabilities(context.Context) ([]string, error) { return s.capabilities, nil }

type auditCall struct { action, targetType, targetID, outcome string; metadata map[string]any }
type serviceAudit struct { calls []auditCall }
func (a *serviceAudit) RecordAudit(_ context.Context, _ security.RequestContext, _ security.Actor, action, targetType, targetID, outcome string, metadata map[string]any) {
	a.calls = append(a.calls, auditCall{action: action, targetType: targetType, targetID: targetID, outcome: outcome, metadata: metadata})
}

func TestServiceAvailableToolsRespectPermissions(t *testing.T) {
	s := NewService("node-1", serviceState{schema: 1}, serviceJobs{}, serviceModules{}, nil)
	actor := security.Actor{Permissions: []string{"system.read", "modules.read"}}
	tools := s.AvailableTools(actor)
	if len(tools) != 2 { t.Fatalf("tool count = %d, want 2: %#v", len(tools), tools) }
	if tools[0].ID != "core.modules.list" || tools[1].ID != "core.system.status" { t.Fatalf("tools = %#v", tools) }
}

func TestServiceExecuteJobsAuditsAndRedactsPayloads(t *testing.T) {
	audit := &serviceAudit{}
	jobs := serviceJobs{jobs: []state.JobRecord{{
		ID: "job-1", NodeID: "node-1", Type: "update.download", Status: "failed", Progress: 4200,
		Input: map[string]any{"secret": "must-not-leak"}, Result: map[string]any{"token": "must-not-leak"},
		ErrorCode: "download_failed", ErrorMessage: "network unavailable", CreatedAt: time.Unix(1, 0).UTC(),
	}}
	s := NewService("node-1", serviceState{schema: 1}, jobs, serviceModules{}, audit)
	actor := security.Actor{Type: "user", ID: "usr-1", Permissions: []string{"jobs.read"}}
	result, err := s.Execute(context.Background(), actor, security.RequestContext{RequestID: "req-1"}, "core.jobs.list", json.RawMessage(`{"limit":5}`), false)
	if err != nil { t.Fatalf("Execute() error = %v", err) }
	if string(result) == "" { t.Fatal("empty tool result") }
	var body map[string]any
	if err := json.Unmarshal(result, &body); err != nil { t.Fatal(err) }
	if bytes.Contains(result, []byte("must-not-leak")) { t.Fatalf("sensitive job payload leaked: %s", result) }
	if len(audit.calls) != 1 { t.Fatalf("audit calls = %d, want 1", len(audit.calls)) }
	call := audit.calls[0]
	if call.action != "ai.tool.execute" || call.targetID != "core.jobs.list" || call.outcome != "success" { t.Fatalf("audit = %#v", call) }
	if _, ok := call.metadata["input"]; ok { t.Fatalf("raw input present in audit metadata: %#v", call.metadata) }
}

func TestServiceDeniedToolIsAudited(t *testing.T) {
	audit := &serviceAudit{}
	s := NewService("node-1", serviceState{schema: 1}, serviceJobs{}, serviceModules{}, audit)
	actor := security.Actor{Type: "user", ID: "usr-1"}
	_, err := s.Execute(context.Background(), actor, security.RequestContext{}, "core.modules.list", nil, false)
	if !errors.Is(err, ErrPermissionDenied) { t.Fatalf("Execute() error = %v, want permission denied", err) }
	if len(audit.calls) != 1 || audit.calls[0].outcome != "denied" { t.Fatalf("audit calls = %#v", audit.calls) }
	if audit.calls[0].metadata["error_code"] != "permission_denied" { t.Fatalf("metadata = %#v", audit.calls[0].metadata) }
}

