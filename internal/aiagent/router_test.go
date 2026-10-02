package aiagent

import (
	"context"
	"errors"
	"testing"
)

type routerTestProvider struct {
	id       string
	content  string
	err      error
	calls    int
}

func (p *routerTestProvider) ID() string { return p.id }

func (p *routerTestProvider) Generate(context.Context, ModelRequest) (ModelResponse, error) {
	p.calls++
	if p.err != nil {
		return ModelResponse{}, p.err
	}
	return ModelResponse{Message: Message{Role: RoleAssistant, Content: p.content}}, nil
}

func TestRoutingProviderKeepsCloudOptInDisabledByDefault(t *testing.T) {
	local := &routerTestProvider{id: "local", content: "local"}
	cloud := &routerTestProvider{id: "cloud", content: "cloud"}
	router := NewRoutingProvider(local, cloud)

	if router.CloudEnabled() {
		t.Fatal("cloud provider unexpectedly enabled")
	}
	if _, err := router.Generate(context.Background(), ModelRequest{ProviderMode: ProviderModeCloud}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("cloud mode error = %v", err)
	}
	response, err := router.Generate(context.Background(), ModelRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.Content != "local" || local.calls != 1 || cloud.calls != 0 {
		t.Fatalf("unexpected default routing: response=%#v local=%d cloud=%d", response, local.calls, cloud.calls)
	}
}

func TestRoutingProviderAutoPrefersCloudAndFallsBackLocal(t *testing.T) {
	local := &routerTestProvider{id: "local", content: "local"}
	cloud := &routerTestProvider{id: "cloud", content: "cloud"}
	router := NewRoutingProvider(local, cloud)
	router.SetCloudEnabled(true)

	response, err := router.Generate(context.Background(), ModelRequest{ProviderMode: ProviderModeAuto})
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.Content != "cloud" || cloud.calls != 1 || local.calls != 0 {
		t.Fatalf("auto did not prefer cloud: %#v", response)
	}

	cloud.err = errors.New("temporary cloud failure")
	response, err = router.Generate(context.Background(), ModelRequest{ProviderMode: ProviderModeAuto})
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.Content != "local" || local.calls != 1 {
		t.Fatalf("auto did not fall back local: %#v", response)
	}
}
