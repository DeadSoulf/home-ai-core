//go:build linux && amd64 && cgo

package cameras

/*
#cgo LDFLAGS: -ldl

#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>

typedef unsigned int hai_DWORD;
typedef unsigned short hai_WORD;
typedef unsigned char hai_BYTE;
typedef int hai_LONG;
typedef int hai_BOOL;
typedef void (*hai_login_cb_t)(hai_LONG, hai_DWORD, void*, void*);

typedef struct {
	char sDeviceAddress[129];
	hai_BYTE byUseTransport;
	hai_WORD wPort;
	char sUserName[64];
	char sPassword[64];
	hai_login_cb_t cbLoginResult;
	void *pUser;
	hai_BOOL bUseAsynLogin;
	hai_BYTE byProxyType;
	hai_BYTE byUseUTCTime;
	hai_BYTE byLoginMode;
	hai_BYTE byHttps;
	hai_LONG iProxyID;
	hai_BYTE byVerifyMode;
	hai_BYTE byRes3[119];
} hai_NET_DVR_USER_LOGIN_INFO;

typedef struct {
	hai_BYTE sSerialNumber[48];
	hai_BYTE byAlarmInPortNum;
	hai_BYTE byAlarmOutPortNum;
	hai_BYTE byDiskNum;
	hai_BYTE byDVRType;
	hai_BYTE byChanNum;
	hai_BYTE byStartChan;
	hai_BYTE byAudioChanNum;
	hai_BYTE byIPChanNum;
	hai_BYTE byZeroChanNum;
	hai_BYTE byMainProto;
	hai_BYTE bySubProto;
	hai_BYTE bySupport;
	hai_BYTE bySupport1;
	hai_BYTE bySupport2;
	hai_WORD wDevType;
	hai_BYTE bySupport3;
	hai_BYTE byMultiStreamProto;
	hai_BYTE byStartDChan;
	hai_BYTE byStartDTalkChan;
	hai_BYTE byHighDChanNum;
	hai_BYTE bySupport4;
	hai_BYTE byLanguageType;
	hai_BYTE byVoiceInChanNum;
	hai_BYTE byStartVoiceInChanNo;
	hai_BYTE bySupport5;
	hai_BYTE bySupport6;
	hai_BYTE byMirrorChanNum;
	hai_WORD wStartMirrorChanNo;
	hai_BYTE bySupport7;
	hai_BYTE byRes2;
} hai_NET_DVR_DEVICEINFO_V30;

typedef struct {
	hai_NET_DVR_DEVICEINFO_V30 struDeviceV30;
	hai_BYTE bySupportLock;
	hai_BYTE byRetryLoginTime;
	hai_BYTE byPasswordLevel;
	hai_BYTE byProxyType;
	hai_DWORD dwSurplusLockTime;
	hai_BYTE byCharEncodeType;
	hai_BYTE bySupportDev5;
	hai_BYTE bySupport;
	hai_BYTE byLoginMode;
	hai_DWORD dwOEMCode;
	int iResidualValidity;
	hai_BYTE byResidualValidity;
	hai_BYTE bySingleStartDTalkChan;
	hai_BYTE bySingleDTalkChanNums;
	hai_BYTE byPassWordResetLevel;
	hai_BYTE bySupportStreamEncrypt;
	hai_BYTE byMarketType;
	hai_BYTE byTLSCap;
	hai_BYTE byRes2[237];
} hai_NET_DVR_DEVICEINFO_V40;

typedef struct {
	hai_DWORD dwSize;
	hai_BYTE sDVRName[32];
	hai_DWORD dwDVRID;
	hai_DWORD dwRecycleRecord;
	hai_BYTE sSerialNumber[48];
	hai_DWORD dwSoftwareVersion;
	hai_DWORD dwSoftwareBuildDate;
	hai_DWORD dwDSPSoftwareVersion;
	hai_DWORD dwDSPSoftwareBuildDate;
	hai_DWORD dwPanelVersion;
	hai_DWORD dwHardwareVersion;
	hai_BYTE byAlarmInPortNum;
	hai_BYTE byAlarmOutPortNum;
	hai_BYTE byRS232Num;
	hai_BYTE byRS485Num;
	hai_BYTE byNetworkPortNum;
	hai_BYTE byDiskCtrlNum;
	hai_BYTE byDiskNum;
	hai_BYTE byDVRType;
	hai_BYTE byChanNum;
	hai_BYTE byStartChan;
	hai_BYTE byDecordChans;
	hai_BYTE byVGANum;
	hai_BYTE byUSBNum;
	hai_BYTE byAuxoutNum;
	hai_BYTE byAudioNum;
	hai_BYTE byIPChanNum;
	hai_BYTE byZeroChanNum;
	hai_BYTE bySupport;
	hai_BYTE byEsataUseage;
	hai_BYTE byIPCPlug;
	hai_BYTE byStorageMode;
	hai_BYTE bySupport1;
	hai_WORD wDevType;
	hai_BYTE byDevTypeName[24];
	hai_BYTE bySupport2;
	hai_BYTE byAnalogAlarmInPortNum;
	hai_BYTE byStartAlarmInNo;
	hai_BYTE byStartAlarmOutNo;
	hai_BYTE byStartIPAlarmInNo;
	hai_BYTE byStartIPAlarmOutNo;
	hai_BYTE byHighIPChanNum;
	hai_BYTE byEnableRemotePowerOn;
	hai_WORD wDevClass;
	hai_BYTE byRes2[6];
} hai_NET_DVR_DEVICECFG_V40;

typedef struct {
	char serial[49];
	char device_name[33];
	char device_type_name[25];
	hai_WORD device_type;
	hai_DWORD software_version;
	hai_DWORD software_build_date;
	hai_DWORD analog_channel_count;
	hai_DWORD start_analog_channel;
	hai_DWORD ip_channel_count;
	hai_DWORD start_ip_channel;
	hai_BYTE password_level;
	hai_BYTE login_mode;
	hai_BYTE config_available;
} hai_device_probe_result;

typedef int (*hai_NET_DVR_Init_t)(void);
typedef int (*hai_NET_DVR_Cleanup_t)(void);
typedef unsigned int (*hai_NET_DVR_GetLastError_t)(void);
typedef int (*hai_NET_DVR_Login_V40_t)(hai_NET_DVR_USER_LOGIN_INFO*, hai_NET_DVR_DEVICEINFO_V40*);
typedef int (*hai_NET_DVR_Logout_t)(int);
typedef int (*hai_NET_DVR_SetConnectTime_t)(unsigned int, unsigned int);
typedef int (*hai_NET_DVR_SetSDKInitCfg_t)(int, void*);
typedef int (*hai_NET_DVR_GetDVRConfig_t)(int, unsigned int, int, void*, unsigned int, unsigned int*);

static void *hai_hcnetsdk_handle = NULL;
static hai_NET_DVR_Init_t hai_NET_DVR_Init = NULL;
static hai_NET_DVR_Cleanup_t hai_NET_DVR_Cleanup = NULL;
static hai_NET_DVR_GetLastError_t hai_NET_DVR_GetLastError = NULL;
static hai_NET_DVR_Login_V40_t hai_NET_DVR_Login_V40 = NULL;
static hai_NET_DVR_Logout_t hai_NET_DVR_Logout = NULL;
static hai_NET_DVR_SetConnectTime_t hai_NET_DVR_SetConnectTime = NULL;
static hai_NET_DVR_SetSDKInitCfg_t hai_NET_DVR_SetSDKInitCfg = NULL;
static hai_NET_DVR_GetDVRConfig_t hai_NET_DVR_GetDVRConfig = NULL;

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
	hai_NET_DVR_Login_V40 = (hai_NET_DVR_Login_V40_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_Login_V40");
	hai_NET_DVR_Logout = (hai_NET_DVR_Logout_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_Logout");
	hai_NET_DVR_SetConnectTime = (hai_NET_DVR_SetConnectTime_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_SetConnectTime");
	hai_NET_DVR_SetSDKInitCfg = (hai_NET_DVR_SetSDKInitCfg_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_SetSDKInitCfg");
	hai_NET_DVR_GetDVRConfig = (hai_NET_DVR_GetDVRConfig_t)dlsym(hai_hcnetsdk_handle, "NET_DVR_GetDVRConfig");

	if (hai_NET_DVR_Init == NULL ||
		hai_NET_DVR_Cleanup == NULL ||
		hai_NET_DVR_GetLastError == NULL ||
		hai_NET_DVR_Login_V40 == NULL ||
		hai_NET_DVR_Logout == NULL) {
		dlclose(hai_hcnetsdk_handle);
		hai_hcnetsdk_handle = NULL;
		hai_NET_DVR_Init = NULL;
		hai_NET_DVR_Cleanup = NULL;
		hai_NET_DVR_GetLastError = NULL;
		hai_NET_DVR_Login_V40 = NULL;
		hai_NET_DVR_Logout = NULL;
		hai_NET_DVR_SetConnectTime = NULL;
		hai_NET_DVR_SetSDKInitCfg = NULL;
		hai_NET_DVR_GetDVRConfig = NULL;
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

static void hai_copy_text(char *dst, size_t dst_size, const unsigned char *src, size_t src_size) {
	if (dst == NULL || dst_size == 0) return;
	size_t len = 0;
	while (len < src_size && src[len] != 0) len++;
	if (len >= dst_size) len = dst_size - 1;
	if (len > 0) memcpy(dst, src, len);
	dst[len] = 0;
}

static int hai_hcnetsdk_login_v40(
	const char *address,
	unsigned short port,
	const char *username,
	const char *password,
	hai_device_probe_result *probe
) {
	if (hai_NET_DVR_Login_V40 == NULL || probe == NULL) {
		return -1;
	}

	hai_NET_DVR_USER_LOGIN_INFO login;
	hai_NET_DVR_DEVICEINFO_V40 device;
	memset(&login, 0, sizeof(login));
	memset(&device, 0, sizeof(device));
	memset(probe, 0, sizeof(*probe));
	strncpy(login.sDeviceAddress, address, sizeof(login.sDeviceAddress)-1);
	strncpy(login.sUserName, username, sizeof(login.sUserName)-1);
	strncpy(login.sPassword, password, sizeof(login.sPassword)-1);
	login.wPort = port;
	login.bUseAsynLogin = 0;
	login.byLoginMode = 0;

	int user_id = hai_NET_DVR_Login_V40(&login, &device);
	if (user_id < 0) {
		return user_id;
	}

	hai_copy_text(probe->serial, sizeof(probe->serial), device.struDeviceV30.sSerialNumber, sizeof(device.struDeviceV30.sSerialNumber));
	probe->device_type = device.struDeviceV30.wDevType;
	probe->analog_channel_count = device.struDeviceV30.byChanNum;
	probe->start_analog_channel = device.struDeviceV30.byStartChan;
	probe->ip_channel_count = ((unsigned int)device.struDeviceV30.byHighDChanNum << 8) | device.struDeviceV30.byIPChanNum;
	probe->start_ip_channel = device.struDeviceV30.byStartDChan;
	probe->password_level = device.byPasswordLevel;
	probe->login_mode = device.byLoginMode;

	if (hai_NET_DVR_GetDVRConfig != NULL) {
		hai_NET_DVR_DEVICECFG_V40 cfg;
		unsigned int returned = 0;
		memset(&cfg, 0, sizeof(cfg));
		cfg.dwSize = sizeof(cfg);
		if (hai_NET_DVR_GetDVRConfig(user_id, 1100, 0, &cfg, sizeof(cfg), &returned)) {
			probe->config_available = 1;
			hai_copy_text(probe->device_name, sizeof(probe->device_name), cfg.sDVRName, sizeof(cfg.sDVRName));
			hai_copy_text(probe->device_type_name, sizeof(probe->device_type_name), cfg.byDevTypeName, sizeof(cfg.byDevTypeName));
			hai_copy_text(probe->serial, sizeof(probe->serial), cfg.sSerialNumber, sizeof(cfg.sSerialNumber));
			probe->device_type = cfg.wDevType;
			probe->software_version = cfg.dwSoftwareVersion;
			probe->software_build_date = cfg.dwSoftwareBuildDate;
			probe->analog_channel_count = cfg.byChanNum;
			probe->start_analog_channel = cfg.byStartChan;
			probe->ip_channel_count = ((unsigned int)cfg.byHighIPChanNum << 8) | cfg.byIPChanNum;
		}
	}
	return user_id;
}

static const char* hai_probe_serial(hai_device_probe_result *probe) { return probe->serial; }
static const char* hai_probe_device_name(hai_device_probe_result *probe) { return probe->device_name; }
static const char* hai_probe_device_type_name(hai_device_probe_result *probe) { return probe->device_type_name; }

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
	hai_NET_DVR_Login_V40 = NULL;
	hai_NET_DVR_Logout = NULL;
	hai_NET_DVR_SetConnectTime = NULL;
	hai_NET_DVR_SetSDKInitCfg = NULL;
	hai_NET_DVR_GetDVRConfig = NULL;
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

	var probe C.hai_device_probe_result
	userID := int(C.hai_hcnetsdk_login_v40(
		cAddress,
		C.ushort(request.Port),
		cUsername,
		cPassword,
		&probe,
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
	defer C.hai_hcnetsdk_logout(C.int(userID))

	device := &DeviceMetadata{
		SerialNumber:       cleanHCNetSDKText(C.GoString(C.hai_probe_serial(&probe))),
		DeviceName:         cleanHCNetSDKText(C.GoString(C.hai_probe_device_name(&probe))),
		DeviceType:         uint16(probe.device_type),
		DeviceTypeName:     cleanHCNetSDKText(C.GoString(C.hai_probe_device_type_name(&probe))),
		SoftwareVersion:    uint32(probe.software_version),
		SoftwareBuildDate:  uint32(probe.software_build_date),
		AnalogChannelCount: int(probe.analog_channel_count),
		IPChannelCount:     int(probe.ip_channel_count),
		StartAnalogChannel: int(probe.start_analog_channel),
		StartIPChannel:     int(probe.start_ip_channel),
		PasswordLevel:      int(probe.password_level),
		LoginMode:          int(probe.login_mode),
	}
	device.Firmware = formatHCNetSDKFirmware(device.SoftwareVersion, device.SoftwareBuildDate)
	device.Channels = enumerateDeviceChannels(*device)

	return LoginResult{
		OK:      true,
		Address: request.Address,
		Port:    request.Port,
		Backend: "HCNetSDK",
		Device:  device,
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
