package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var (
	ErrToolNotFound     = errors.New("AI tool not found")
	ErrPermissionDenied = errors.New("AI tool permission denied")
	ErrApprovalRequired = errors.New("AI tool approval required")
	ErrInvalidToolInput = errors.New("invalid AI tool input")
)

type Sensitivity string

const (
	SensitivityRead      Sensitivity = "read"
	SensitivityChange    Sensitivity = "change"
	SensitivitySensitive Sensitivity = "sensitive"
)

type Scope struct {
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
}

type ToolDescriptor struct {
	ID                  string          `json:"id"`
	ModuleID            string          `json:"module_id"`
	Name                string          `json:"name"`
	Description         string          `json:"description,omitempty"`
	InputSchema         json.RawMessage `json:"input_schema,omitempty"`
	RequiredPermissions []string        `json:"required_permissions,omitempty"`
	Scope               Scope           `json:"scope,omitempty"`
	Sensitivity         Sensitivity     `json:"sensitivity"`
}

type Principal interface {
	Has(permission string) bool
	Allows(permission, resourceType, resourceID string) bool
}

type ToolHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

type registeredTool struct {
	descriptor ToolDescriptor
	handler    ToolHandler
}

type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]registeredTool
}

var toolIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]registeredTool)}
}

func (r *ToolRegistry) Register(descriptor ToolDescriptor, handler ToolHandler) error {
	if err := validateToolDescriptor(descriptor); err != nil {
		return err
	}
	if handler == nil {
		return errors.New("AI tool handler is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[descriptor.ID]; exists {
		return fmt.Errorf("AI tool %q is already registered", descriptor.ID)
	}
	r.tools[descriptor.ID] = registeredTool{descriptor: descriptor, handler: handler}
	return nil
}

func (r *ToolRegistry) Descriptor(id string) (ToolDescriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[id]
	if !ok {
		return ToolDescriptor{}, false
	}
	return tool.descriptor, true
}

func (r *ToolRegistry) List() []ToolDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ToolDescriptor, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool.descriptor)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (r *ToolRegistry) Execute(
	ctx context.Context,
	principal Principal,
	toolID string,
	input json.RawMessage,
	approved bool,
) (json.RawMessage, error) {
	r.mu.RLock()
	tool, ok := r.tools[toolID]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrToolNotFound
	}
	if principal == nil {
		return nil, ErrPermissionDenied
	}

	if !toolAllowed(principal, tool.descriptor) {
		return nil, ErrPermissionDenied
	}

	if tool.descriptor.Sensitivity != SensitivityRead && !approved {
		return nil, ErrApprovalRequired
	}
	if len(input) > 0 && !json.Valid(input) {
		return nil, ErrInvalidToolInput
	}

	return tool.handler(ctx, input)
}

func toolAllowed(principal Principal, descriptor ToolDescriptor) bool {
	if principal == nil {
		return false
	}
	for _, permission := range descriptor.RequiredPermissions {
		if descriptor.Scope.ResourceType != "" {
			if !principal.Allows(permission, descriptor.Scope.ResourceType, descriptor.Scope.ResourceID) {
				return false
			}
			continue
		}
		if !principal.Has(permission) {
			return false
		}
	}
	return true
}

func validateToolDescriptor(descriptor ToolDescriptor) error {
	if !toolIDPattern.MatchString(descriptor.ID) {
		return errors.New("invalid AI tool id")
	}
	if !toolIDPattern.MatchString(descriptor.ModuleID) {
		return errors.New("invalid AI tool module id")
	}
	if strings.TrimSpace(descriptor.Name) == "" {
		return errors.New("AI tool name is required")
	}
	switch descriptor.Sensitivity {
	case SensitivityRead, SensitivityChange, SensitivitySensitive:
	default:
		return errors.New("invalid AI tool sensitivity")
	}
	if (descriptor.Scope.ResourceType == "") != (descriptor.Scope.ResourceID == "") {
		return errors.New("AI tool scope requires both resource type and resource id")
	}
	if len(descriptor.InputSchema) > 0 && !json.Valid(descriptor.InputSchema) {
		return errors.New("invalid AI tool input schema")
	}
	for _, permission := range descriptor.RequiredPermissions {
		if !toolIDPattern.MatchString(permission) {
			return fmt.Errorf("invalid AI tool permission %q", permission)
		}
	}
	return nil
}
