//go:build !linux || !amd64 || !cgo

package cameras

import (
	"context"
	"runtime"
)

type unsupportedHCNetSDKRuntime struct {
	status SDKStatus
}

func newHCNetSDKRuntime() sdkRuntime {
	return &unsupportedHCNetSDKRuntime{
		status: SDKStatus{
			Architecture: runtime.GOARCH,
			Supported:    false,
			Available:    false,
			Initialized:  false,
			Error:        "the supplied HCNetSDK package supports Linux amd64 only",
		},
	}
}

func (r *unsupportedHCNetSDKRuntime) Status() SDKStatus {
	return r.status
}

func (r *unsupportedHCNetSDKRuntime) Refresh() SDKStatus {
	return r.status
}

func (r *unsupportedHCNetSDKRuntime) TestLogin(
	context.Context,
	LoginRequest,
) (LoginResult, error) {
	return LoginResult{}, ErrSDKUnsupported
}

func (r *unsupportedHCNetSDKRuntime) Close() error {
	return nil
}
