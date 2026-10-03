//go:build linux && amd64 && cgo

package cameras

/*
#cgo LDFLAGS: -ldl

#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>

typedef int (*hai_NET_DVR_Init_t)(void);
typedef int (*hai_NET_DVR_Cleanup_t)(void);
typedef unsigned int (*hai_NET_DVR_GetLastError_t)(void);
typedef int (*hai_NET_DVR_Login_V30_t)(char*, unsigned short, char*, char*, void*);
typedef int (*hai_NET_DVR_Logout_t)(int);
typedef int (*hai_NET_DVR_SetConnectTime_t)(unsigned int, unsigned int);
typedef int (*hai_NET_DVR_SetSDKInitCfg_t)(int, void*);

static void *hai_hcnetsdk_handle = NULL;
static hai_NET_DVR_Init_t hai_NET_DVR_Init = NULL;
static hai_NET_DVR_Cleanup_t hai_NET_DVR_Cleanup = NULL;
static hai_NET_DVR_GetLastError_t hai_NET_DVR_GetLastError = NULL;
static hai_NET_DVR_Login_V30_t hai_NET_DVR_Login_V30 = NULL;
static hai_NET_DVR_Logout_t hai_NET_DVR_Logout = NULL;
static hai_NET_DVR_SetConnectTime_t hai_NET_DVR_SetConnectTime = NULL;
static hai_NET_DVR_SetSDKInitCfg_t hai_NET_DVR_SetSDKInitCfg = NULL;

static const char* hai_hcnetsdk_dlerror(void) {
	const char *err = dlerror();
	return err == NULL ? "" : err;
}

static int hai_hcnetsdk_load(const char *path) {
	if (hai_hcnetsdk_handle != NULL) {
		return 1;
	}

	dlerror();
	hai_hcnetsdk_handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
	if (hai_hcnetsdk_handle == NULL) {
		return 0;
	}

	hai_NET_DVR_Init = (hai_NET_DVR_Init_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_Init");
	hai_NET_DVR_Cleanup = (hai_NET_DVR_Cleanup_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_Cleanup");
	hai_NET_DVR_GetLastError = (hai_NET_DVR_GetLastError_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_GetLastError");
	hai_NET_DVR_Login_V30 = (hai_NET_DVR_Login_V30_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_Login_V30");
	hai_NET_DVR_Logout = (hai_NET_DVR_Logout_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_Logout");
	hai_NET_DVR_SetConnectTime = (hai_NET_DVR_SetConnectTime_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_SetConnectTime");
	hai_NET_DVR_SetSDKInitCfg = (hai_NET_DVR_SetSDKInitCfg_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_SetSDKInitCfg");

	if (hai_NET_DVR_Init == NULL ||
		hai_NET_DVR_Cleanup == NULL ||
		hai_NET_DVR_GetLastError == NULL ||
		hai_NET_DVR_Login_V30 == NULL ||
		hai_NET_DVR_Logout == NULL) {
		dlclose(hai_hcnetsdk_handle);
		hai_hcnetsdk_handle = NULL;
		hai_NET_DVR_Init = NULL;
		hai_NET_DVR_Cleanup = NULL;
		hai_NET_DVR_GetLastError = NULL;
		hai_NET_DVR_Login_V30 = NULL;
		hai_NET_DVR_Logout = NULL;
		hai_NET_DVR_SetConnectTime = NULL;
		hai_NET_DVR_SetSDKInitCfg = NULL;
		return -1;
	}
	return 1;
}

static int hai_hcnetsdk_set_sdk_path(const char *path) {
	if (hai_NET_DVR_SetSDKInitCfg == NULL || path == NULL) return 1;
	struct { char sPath[256]; unsigned char byRes[128]; } cfg;
	memset(&cfg, 0, sizeof(cfg));
	strncpy(cfg.sPath, path, sizeof(cfg.sPath)-1);
	return hai_NET_DVR_SetSDKInitCfg(2, &cfg);
}

static int hai_hcnetsdk_init(void) {
	if (hai_NET_DVR_Init == NULL) {
		return 0;
	}
	if (hai_NET_DVR_SetConnectTime != NULL) {
		hai_NET_DVR_SetConnectTime(3000, 1);
	}
	return hai_NET_DVR_Init();
}

static unsigned int hai_hcnetsdk_last_error(void) {
	if (hai_NET_DVR_GetLastError == NULL) {
		return 0;
	}
	return hai_NET_DVR_GetLastError();
}

static int hai_hcnetsdk_login_v30(
	const char *address,
	unsigned short port,
	const char *username,
	const char *password
) {
	if (hai_NET_DVR_Login_V30 == NULL) {
		return -1;
	}
	unsigned char device_info[4096];
	memset(device_info, 0, sizeof(device_info));
	return hai_NET_DVR_Login_V30(
		(char*)address,
		port,
		(char*)username,
		(char*)password,
		(void*)device_info
	);
}

static int hai_hcnetsdk_logout(int user_id) {
	if (hai_NET_DVR_Logout == NULL) {
		return 0;
	}
	return hai_NET_DVR_Logout(user_id);
}

static void hai_hcnetsdk_close(void) {
	if (hai_hcnetsdk_handle == NULL) {
		return;
	}
	if (hai_NET_DVR_Cleanup != NULL) {
		hai_NET_DVR_Cleanup();
	}
	dlclose(hai_hcnetsdk_handle);
	hai_hcnetsdk_handle = NULL;
	hai_NET_DVR_Init = NULL;
	hai_NET_DVR_Cleanup = NULL;
	hai_NET_DVR_GetLastError = NULL;
	hai_NET_DVR_Login_V30 = NULL;
	hai_NET_DVR_Logout = NULL;
	hai_NET_DVR_SetConnectTime = NULL;
	hai_NET_DVR_SetSDKInitCfg = NULL;
}
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

type hcNetSDKRuntime struct {
	mu          sync.Mutex
	status      SDKStatus
	loaded      bool
	initialized bool
}

func newHCNetSDKRuntime() sdkRuntime {
	runtime := &hcNetSDKRuntime{}
	runtime.status = runtime.detectAndLoad()
	return runtime
}

func (r *hcNetSDKRuntime) Status() SDKStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *hcNetSDKRuntime) Refresh() SDKStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.loaded {
		r.status.Available = true
		r.status.Initialized = r.initialized
		return r.status
	}
	r.status = r.detectAndLoadLocked()
	return r.status
}

func (r *hcNetSDKRuntime) detectAndLoad() SDKStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.detectAndLoadLocked()
}

func (r *hcNetSDKRuntime) detectAndLoadLocked() SDKStatus {
	status := SDKStatus{
		Architecture: runtime.GOARCH,
		Supported:    true,
	}
	path := findHCNetSDKLibrary()
	if path == "" {
		status.Error = "libhcnetsdk.so was not found"
		return status
	}
	status.LibraryPath = path

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	result := int(C.hai_hcnetsdk_load(cPath))
	switch result {
	case 1:
		r.loaded = true
		status.Available = true
	case -1:
		status.Error = "HCNetSDK is missing one or more required symbols"
		return status
	default:
		detail := strings.TrimSpace(C.GoString(C.hai_hcnetsdk_dlerror()))
		if detail == "" {
			detail = "dlopen failed"
		}
		status.Error = detail
		return status
	}

	sdkDir := filepath.Dir(path)
	cSDKDir := C.CString(sdkDir)
	C.hai_hcnetsdk_set_sdk_path(cSDKDir)
	C.free(unsafe.Pointer(cSDKDir))

	if int(C.hai_hcnetsdk_init()) == 0 {
		code := uint32(C.hai_hcnetsdk_last_error())
		status.Error = fmt.Sprintf("NET_DVR_Init failed (HCNetSDK error %d)", code)
		return status
	}
	r.initialized = true
	status.Initialized = true
	return status
}

func (r *hcNetSDKRuntime) TestLogin(
	ctx context.Context,
	request LoginRequest,
) (LoginResult, error) {
	if err := ctx.Err(); err != nil {
		return LoginResult{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.loaded || !r.initialized {
		status := r.detectAndLoadLocked()
		if !status.Supported {
			return LoginResult{}, ErrSDKUnsupported
		}
		if !status.Available || !status.Initialized {
			return LoginResult{}, fmt.Errorf("%w: %s", ErrSDKUnavailable, status.Error)
		}
	}

	cAddress := C.CString(request.Address)
	cUsername := C.CString(request.Username)
	cPassword := C.CString(request.Password)
	defer C.free(unsafe.Pointer(cAddress))
	defer C.free(unsafe.Pointer(cUsername))
	defer C.free(unsafe.Pointer(cPassword))

	userID := int(C.hai_hcnetsdk_login_v30(
		cAddress,
		C.ushort(request.Port),
		cUsername,
		cPassword,
	))
	if userID < 0 {
		code := uint32(C.hai_hcnetsdk_last_error())
		return LoginResult{
			OK:           false,
			Address:      request.Address,
			Port:         request.Port,
			Backend:      "HCNetSDK",
			SDKErrorCode: code,
		}, SDKError{Code: code}
	}
	C.hai_hcnetsdk_logout(C.int(userID))
	return LoginResult{
		OK:      true,
		Address: request.Address,
		Port:    request.Port,
		Backend: "HCNetSDK",
	}, nil
}

func (r *hcNetSDKRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded {
		C.hai_hcnetsdk_close()
	}
	r.loaded = false
	r.initialized = false
	r.status.Initialized = false
	r.status.Available = false
	return nil
}

func findHCNetSDKLibrary() string {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("HOME_AI_HCNETSDK_LIB")); configured != "" {
		candidates = append(candidates, configured)
	}
	if root := strings.TrimSpace(os.Getenv("HOME_AI_HCNETSDK_DIR")); root != "" {
		candidates = append(candidates,
			filepath.Join(root, "lib", "libhcnetsdk.so"),
			filepath.Join(root, "libhcnetsdk.so"),
		)
	}
	candidates = append(candidates,
		"/opt/home-ai/hikvision/lib/libhcnetsdk.so",
		"/opt/home-ai/hikvision/HCNetSDK/lib/libhcnetsdk.so",
		"/usr/local/lib/libhcnetsdk.so",
		"/usr/lib/libhcnetsdk.so",
	)

	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if candidate == "." || seen[candidate] {
			continue
		}
		seen[candidate] = true
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
