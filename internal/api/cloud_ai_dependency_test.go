package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/aiagent"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
)

func TestCloudAIEnableAutoEnablesAIAgentDependency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const nodeID = "00000000-0000-4000-8000-000000000000"

	moduleService := fakeModules{
		items: []modules.Registered{
			{
				Manifest: modules.Manifest{
					SchemaVersion: 1,
					ID:            "ai.agent",
					Name:          "AI Agent",
					Version:       "0.8.0",
					Core:          ">=0.1.0 <1.0.0",
					Lifecycle:     []string{"backup"},
				},
				Status: "disabled",
			},
			{
				Manifest: modules.Manifest{
					SchemaVersion: 1,
					ID:            "ai.cloud",
					Name:          "Cloud AI",
					Version:       "0.1.0",
					Core:          ">=0.1.0 <1.0.0",
					Lifecycle:     []string{"backup"},
				},
				Status: "disabled",
			},
		},
	}

	local := aiagent.DeterministicProvider{
		ProviderID: "local",
		Response:   aiagent.ModelResponse{Message: aiagent.Message{Role: aiagent.RoleAssistant, Content: "local"}},
	}
	cloud := aiagent.DeterministicProvider{
		ProviderID: "cloud",
		Response:   aiagent.ModelResponse{Message: aiagent.Message{Role: aiagent.RoleAssistant, Content: "cloud"}},
	}
	router := aiagent.NewRoutingProvider(local, cloud)

	handler := New(
		nodeID,
		logger,
		fakeState{schemaVersion: 19},
		defaultFakeSecurity(),
		nil,
		nil,
		moduleService,
		nil,
		realtime.New(nodeID, logger),
		router,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/modules/ai.cloud/control",
		strings.NewReader(`{"operation":"enable"}`),
	)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cloud enable status = %d: %s", rec.Code, rec.Body.String())
	}

	agent, err := moduleService.Get(context.Background(), "ai.agent")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != "enabled" {
		t.Fatalf("AI Agent dependency status = %q, want enabled", agent.Status)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/ai/status", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("AI status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"state":"ready"`) {
		t.Fatalf("AI Agent was not started: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"cloud_provider_enabled":true`) {
		t.Fatalf("Cloud AI was not enabled: %s", rec.Body.String())
	}
}
