package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
)

type testPrincipal struct {
	global map[string]bool
	scoped map[string]bool
}

func (p testPrincipal) Has(permission string) bool {
	return p.global[permission]
}

func (p testPrincipal) Allows(permission, resourceType, resourceID string) bool {
	return p.global[permission] || p.scoped[permission+"|"+resourceType+"|"+resourceID]
}

func TestModuleManifestIsValid(t *testing.T) {
	manifest := NewModule().Manifest()
	if err := modules.ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest() error = %v", err)
	}
	if manifest.ID != "ai.agent" {
		t.Fatalf("module id = %q", manifest.ID)
	}
}

func TestToolRegistryExecutesAuthorizedReadTool(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(ToolDescriptor{
		ID:                  "core.system.status",
		ModuleID:            "ai.agent",
		Name:                "System status",
		RequiredPermissions: []string{"system.read"},
		Sensitivity:         SensitivityRead,
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"status":"ok"}`), nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	output, err := registry.Execute(
		context.Background(),
		testPrincipal{global: map[string]bool{"system.read": true}},
		"core.system.status",
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(output) != `{"status":"ok"}` {
		t.Fatalf("output = %s", output)
	}
}

func TestToolRegistryDeniesMissingPermission(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(ToolDescriptor{
		ID:                  "core.system.status",
		ModuleID:            "ai.agent",
		Name:                "System status",
		RequiredPermissions: []string{"system.read"},
		Sensitivity:         SensitivityRead,
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	})

	_, err := registry.Execute(context.Background(), testPrincipal{}, "core.system.status", nil, false)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Execute() error = %v, want permission denied", err)
	}
}

func TestToolRegistryUsesScopedPermission(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(ToolDescriptor{
		ID:                  "files.folder.inspect",
		ModuleID:            "ai.agent",
		Name:                "Inspect folder",
		RequiredPermissions: []string{"files.read"},
		Scope:               Scope{ResourceType: "file_folder", ResourceID: "folder-1"},
		Sensitivity:         SensitivityRead,
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	})

	principal := testPrincipal{scoped: map[string]bool{"files.read|file_folder|folder-1": true}}
	if _, err := registry.Execute(context.Background(), principal, "files.folder.inspect", nil, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestChangeToolRequiresApproval(t *testing.T) {
	registry := NewToolRegistry()
	_ = registry.Register(ToolDescriptor{
		ID:          "smart-home.light.set",
		ModuleID:    "ai.agent",
		Name:        "Set light",
		Sensitivity: SensitivityChange,
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	})

	principal := testPrincipal{}
	if _, err := registry.Execute(context.Background(), principal, "smart-home.light.set", nil, false); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("Execute() error = %v, want approval required", err)
	}
	if _, err := registry.Execute(context.Background(), principal, "smart-home.light.set", nil, true); err != nil {
		t.Fatalf("approved Execute() error = %v", err)
	}
}

func TestDeterministicProviderHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := DeterministicProvider{
		Response: ModelResponse{Message: Message{Role: RoleAssistant, Content: "ok"}},
	}
	if _, err := provider.Generate(ctx, ModelRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate() error = %v, want context canceled", err)
	}
}
