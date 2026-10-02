package aiagent

import (
	"context"
	"errors"
	"strings"
	"sync"
)

const (
	ProviderModeLocal = "local"
	ProviderModeCloud = "cloud"
	ProviderModeAuto  = "auto"
)

type RoutingProvider struct {
	mu           sync.RWMutex
	local        Provider
	cloud        Provider
	cloudEnabled bool
}

func NewRoutingProvider(local, cloud Provider) *RoutingProvider {
	return &RoutingProvider{local: local, cloud: cloud}
}

func (p *RoutingProvider) ID() string {
	return "routing"
}

func (p *RoutingProvider) Model() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if modelProvider, ok := p.local.(interface{ Model() string }); ok && p.local != nil {
		return modelProvider.Model()
	}
	if modelProvider, ok := p.cloud.(interface{ Model() string }); ok && p.cloud != nil {
		return modelProvider.Model()
	}
	return ""
}

func (p *RoutingProvider) SetCloudEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cloudEnabled = enabled && p.cloud != nil
}

func (p *RoutingProvider) CloudConfigured() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cloud != nil
}

func (p *RoutingProvider) CloudEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cloudEnabled && p.cloud != nil
}

func (p *RoutingProvider) LocalConfigured() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.local != nil
}

func (p *RoutingProvider) AvailableModes() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	modes := make([]string, 0, 3)
	if p.local != nil {
		modes = append(modes, ProviderModeLocal)
	}
	if p.cloudEnabled && p.cloud != nil {
		modes = append(modes, ProviderModeCloud)
		if p.local != nil {
			modes = append(modes, ProviderModeAuto)
		}
	}
	return modes
}

func (p *RoutingProvider) CloudModel() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if modelProvider, ok := p.cloud.(interface{ Model() string }); ok && p.cloud != nil {
		return modelProvider.Model()
	}
	return ""
}

func (p *RoutingProvider) LocalModel() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if modelProvider, ok := p.local.(interface{ Model() string }); ok && p.local != nil {
		return modelProvider.Model()
	}
	return ""
}

func (p *RoutingProvider) TestCloud(ctx context.Context) error {
	p.mu.RLock()
	cloud := p.cloud
	p.mu.RUnlock()
	if cloud == nil {
		return ErrProviderUnavailable
	}
	response, err := cloud.Generate(ctx, ModelRequest{
		Messages: []Message{{Role: RoleUser, Content: "Reply with OK."}},
		ProviderMode: ProviderModeCloud,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(response.Message.Content) == "" {
		return &ProviderRequestError{
			Provider: "cloud",
			Kind:     "invalid_response",
			Message:  "Cloud AI returned an empty test response",
		}
	}
	return nil
}

func (p *RoutingProvider) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	local, cloud, cloudEnabled := p.providers()
	mode := strings.ToLower(strings.TrimSpace(request.ProviderMode))
	switch mode {
	case "", ProviderModeLocal:
		if local != nil {
			return local.Generate(ctx, request)
		}
		if mode == "" && cloudEnabled && cloud != nil {
			return cloud.Generate(ctx, request)
		}
		return ModelResponse{}, ErrProviderUnavailable
	case ProviderModeCloud:
		if !cloudEnabled || cloud == nil {
			return ModelResponse{}, ErrProviderUnavailable
		}
		return cloud.Generate(ctx, request)
	case ProviderModeAuto:
		if cloudEnabled && cloud != nil {
			response, err := cloud.Generate(ctx, request)
			if err == nil {
				return response, nil
			}
			if ctx.Err() != nil || local == nil {
				return ModelResponse{}, err
			}
			return local.Generate(ctx, request)
		}
		if local != nil {
			return local.Generate(ctx, request)
		}
		return ModelResponse{}, ErrProviderUnavailable
	default:
		return ModelResponse{}, errors.New("unsupported AI provider mode")
	}
}

func (p *RoutingProvider) providers() (Provider, Provider, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.local, p.cloud, p.cloudEnabled
}
