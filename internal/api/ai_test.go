package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/security"
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
