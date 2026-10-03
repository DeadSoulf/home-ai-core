package cameras

import (
	"context"
	"errors"
	"testing"
)

type fakeSDKRuntime struct {
	status SDKStatus
	login  LoginResult
	err    error
	last   LoginRequest
}

func (f *fakeSDKRuntime) Status() SDKStatus {
	return f.status
}

func (f *fakeSDKRuntime) Refresh() SDKStatus {
	return f.status
}

func (f *fakeSDKRuntime) TestLogin(_ context.Context, request LoginRequest) (LoginResult, error) {
	f.last = request
	return f.login, f.err
}

func (f *fakeSDKRuntime) Close() error {
	return nil
}

func TestServiceDefaultsHCNetSDKPort(t *testing.T) {
	fake := &fakeSDKRuntime{
		status: SDKStatus{Supported: true, Available: true, Initialized: true},
		login:  LoginResult{OK: true, Backend: "HCNetSDK"},
	}
	service := NewServiceWithRuntime(fake)
	result, err := service.TestLogin(context.Background(), LoginRequest{
		Address:  "192.168.1.64",
		Username: "admin",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if fake.last.Port != 8000 {
		t.Fatalf("port = %d, want 8000", fake.last.Port)
	}
}

func TestServiceRejectsPublicCameraAddress(t *testing.T) {
	service := NewServiceWithRuntime(&fakeSDKRuntime{})
	_, err := service.TestLogin(context.Background(), LoginRequest{
		Address:  "8.8.8.8",
		Port:     8000,
		Username: "admin",
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("error = %v, want ErrInvalidTarget", err)
	}
}

func TestCameraModuleUsesFreshNamespace(t *testing.T) {
	manifest := NewModule().Manifest()
	if manifest.ID != "cameras" {
		t.Fatalf("module id = %q", manifest.ID)
	}
	if manifest.API.Namespace != "cameras" {
		t.Fatalf("API namespace = %q", manifest.API.Namespace)
	}
	if len(manifest.UI.Navigation) != 1 ||
		manifest.UI.Navigation[0].Route != "/modules/cameras" {
		t.Fatalf("navigation = %#v", manifest.UI.Navigation)
	}
	if len(manifest.Capabilities.Provides) == 0 {
		t.Fatal("camera module capability missing")
	}
}
