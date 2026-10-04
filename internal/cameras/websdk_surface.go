package cameras

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

const maxWebSDKRawBodySize int64 = 16 << 20

type WebSDKFunctionDescriptor struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Backend     string `json:"backend"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

type WebSDKRawRequest struct {
	Address     string `json:"address"`
	Port        int    `json:"port"`
	HTTPS       bool   `json:"https"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	ContentType string `json:"content_type,omitempty"`
	Body        string `json:"body,omitempty"`
	BodyBase64  string `json:"body_base64,omitempty"`
}

type WebSDKRawResponse struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Body        string `json:"body,omitempty"`
	BodyBase64  string `json:"body_base64,omitempty"`
	Binary      bool   `json:"binary"`
}

func WebSDKFunctions() []WebSDKFunctionDescriptor {
	return []WebSDKFunctionDescriptor{
		{Name: "I_SupportNoPlugin", Category: "compatibility", Backend: "home-ai-browser", Status: "implemented", Description: "Home-AI browser path never requires HCWebSDKPlugin.exe."},
		{Name: "I_Resize", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_InitPlugin", Category: "compatibility", Backend: "home-ai-browser", Status: "implemented", Description: "Mapped to Home-AI player initialization."},
		{Name: "I_InsertOBJECTPlugin", Category: "compatibility", Backend: "home-ai-browser", Status: "implemented", Description: "No OBJECT plugin is inserted; Home-AI uses normal DOM media surfaces."},
		{Name: "I_WriteOBJECT_XHTML", Category: "compatibility", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_OpenFileDlg", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_GetLocalCfg", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_SetLocalCfg", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_Login", Category: "session", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_Logout", Category: "session", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_GetAudioInfo", Category: "device", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_GetDeviceInfo", Category: "device", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_GetAnalogChannelInfo", Category: "channel", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_GetDigitalChannelInfo", Category: "channel", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_GetZeroChannelInfo", Category: "channel", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_StartRealPlay", Category: "live", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_StartPlay", Category: "live", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_SetSecretKey", Category: "media-security", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_GetEncryptString", Category: "media-security", Backend: "core-security", Status: "mapped"},
		{Name: "I_Stop", Category: "live", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_StopAllPlay", Category: "live", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_OpenSound", Category: "audio", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_CloseSound", Category: "audio", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_SetVolume", Category: "audio", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_CapturePic", Category: "capture", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_CapturePicData", Category: "capture", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_StartRecord", Category: "record", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_StopRecord", Category: "record", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_StartVoiceTalk", Category: "audio", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_StopVoiceTalk", Category: "audio", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_StartAudioPlay", Category: "audio", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_StopAudioPlay", Category: "audio", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_PTZControl", Category: "ptz", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_EnableEZoom", Category: "browser-ui", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_DisableEZoom", Category: "browser-ui", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_Enable3DZoom", Category: "ptz", Backend: "home-ai-browser+core-isapi", Status: "implemented"},
		{Name: "I_Disable3DZoom", Category: "ptz", Backend: "home-ai-browser+core-isapi", Status: "implemented"},
		{Name: "I_FullScreen", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_SetPreset", Category: "ptz", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_GoPreset", Category: "ptz", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_RecordSearch", Category: "playback", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_StartPlayback", Category: "playback", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_ReversePlayback", Category: "playback", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_Frame", Category: "playback", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_Pause", Category: "playback", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_Resume", Category: "playback", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_PlaySlow", Category: "playback", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_PlayFast", Category: "playback", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_GetOSDTime", Category: "playback", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_StartDownloadRecord", Category: "download", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_StartDownloadRecordByTime", Category: "download", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_GetDownloadStatus", Category: "download", Backend: "core-transfer", Status: "mapped"},
		{Name: "I_GetDownloadProgress", Category: "download", Backend: "core-transfer", Status: "mapped"},
		{Name: "I_StopDownloadRecord", Category: "download", Backend: "core-transfer", Status: "mapped"},
		{Name: "I_ExportDeviceConfig", Category: "configuration", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_ImportDeviceConfig", Category: "configuration", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_RestoreDefault", Category: "system", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_Restart", Category: "system", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_Reconnect", Category: "session", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_StartUpgrade", Category: "upgrade", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_UpgradeStatus", Category: "upgrade", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_UpgradeProgress", Category: "upgrade", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_StopUpgrade", Category: "upgrade", Backend: "core-transfer", Status: "mapped"},
		{Name: "I_CheckPluginInstall", Category: "compatibility", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_CheckPluginVersion", Category: "compatibility", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_SendHTTPRequest", Category: "http", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_ChangeWndNum", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_GetLastError", Category: "diagnostics", Backend: "home-ai-core", Status: "implemented"},
		{Name: "I_GetWindowStatus", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_GetIPInfoByMode", Category: "network", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_SetPlayModeType", Category: "browser-ui", Backend: "home-ai-browser", Status: "mapped"},
		{Name: "I_SetSnapDrawMode", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_SetSnapPolygonInfo", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_GetSnapPolygonInfo", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_ClearSnapInfo", Category: "browser-ui", Backend: "home-ai-browser", Status: "implemented"},
		{Name: "I_DeviceCapturePic", Category: "capture", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_SetPackageType", Category: "media", Backend: "hcnetsdk-media", Status: "mapped"},
		{Name: "I_GetDevicePort", Category: "device", Backend: "core-isapi", Status: "implemented"},
		{Name: "I_GetTextOverlay", Category: "device", Backend: "core-isapi", Status: "implemented"},
	}
}

func (s *Service) SendWebSDKRequest(ctx context.Context, request WebSDKRawRequest) (WebSDKRawResponse, error) {
	probe := WebSDKProbeRequest{
		Address:  request.Address,
		Port:     request.Port,
		HTTPS:    request.HTTPS,
		Username: request.Username,
		Password: request.Password,
	}
	if probe.Port == 0 {
		if probe.HTTPS {
			probe.Port = 443
		} else {
			probe.Port = 80
		}
	}
	if err := validateWebSDKRequest(probe); err != nil {
		return WebSDKRawResponse{}, err
	}

	method := strings.ToUpper(strings.TrimSpace(request.Method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return WebSDKRawResponse{}, fmt.Errorf("%w: unsupported WebSDK HTTP method", ErrInvalidTarget)
	}
	path, err := validateWebSDKPath(request.Path)
	if err != nil {
		return WebSDKRawResponse{}, err
	}

	var body []byte
	switch {
	case request.Body != "" && request.BodyBase64 != "":
		return WebSDKRawResponse{}, fmt.Errorf("%w: specify body or body_base64, not both", ErrInvalidTarget)
	case request.BodyBase64 != "":
		body, err = base64.StdEncoding.DecodeString(request.BodyBase64)
		if err != nil {
			return WebSDKRawResponse{}, fmt.Errorf("%w: body_base64 is invalid", ErrInvalidTarget)
		}
	default:
		body = []byte(request.Body)
	}
	if int64(len(body)) > maxWebSDKRawBodySize {
		return WebSDKRawResponse{}, fmt.Errorf("%w: WebSDK request body exceeds 16 MiB", ErrInvalidTarget)
	}

	client := newWebSDKClient(probe)
	status, contentType, payload, err := client.requestBytes(ctx, method, path, request.ContentType, body)
	if err != nil {
		return WebSDKRawResponse{}, err
	}

	result := WebSDKRawResponse{
		Status:      status,
		ContentType: contentType,
	}
	if webSDKContentIsText(contentType, payload) {
		result.Body = string(payload)
	} else {
		result.Binary = true
		result.BodyBase64 = base64.StdEncoding.EncodeToString(payload)
	}
	return result, nil
}

func validateWebSDKPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") {
		return "", fmt.Errorf("%w: invalid WebSDK path", ErrInvalidTarget)
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "", fmt.Errorf("%w: invalid WebSDK path", ErrInvalidTarget)
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("%w: WebSDK path traversal is not allowed", ErrInvalidTarget)
		}
	}
	allowed := strings.HasPrefix(parsed.Path, "/ISAPI/") ||
		strings.HasPrefix(parsed.Path, "/SDK/") ||
		strings.HasPrefix(parsed.Path, "/PSIA/Custom/SelfExt/ContentMgmt/ZeroStreaming/")
	if !allowed {
		return "", fmt.Errorf("%w: WebSDK path is outside the allowed Hikvision API namespaces", ErrInvalidTarget)
	}
	return parsed.RequestURI(), nil
}

func (c *webSDKClient) requestBytes(
	ctx context.Context,
	method string,
	path string,
	contentType string,
	body []byte,
) (int, string, []byte, error) {
	target := c.baseURL + path
	response, err := c.doBytes(ctx, method, target, contentType, body, "")
	if err != nil {
		return 0, "", nil, err
	}
	if response.StatusCode == http.StatusUnauthorized {
		challenges := response.Header.Values("WWW-Authenticate")
		_ = response.Body.Close()
		auth, authErr := buildWebSDKAuthorization(challenges, c.username, c.password, method, path)
		if authErr != nil {
			return 0, "", nil, authErr
		}
		response, err = c.doBytes(ctx, method, target, contentType, body, auth)
		if err != nil {
			return 0, "", nil, err
		}
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxWebSDKRawBodySize+1))
	if err != nil {
		return 0, "", nil, fmt.Errorf("read WebSDK response: %w", err)
	}
	if int64(len(payload)) > maxWebSDKRawBodySize {
		return 0, "", nil, errors.New("WebSDK response exceeds 16 MiB safety limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, "", nil, WebSDKHTTPError{Path: path, Status: response.StatusCode}
	}
	return response.StatusCode, response.Header.Get("Content-Type"), payload, nil
}

func (c *webSDKClient) doBytes(
	ctx context.Context,
	method string,
	target string,
	contentType string,
	body []byte,
	authorization string,
) (*http.Response, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = strings.NewReader(string(body))
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "*/*")
	if len(body) > 0 {
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/xml"
		}
		request.Header.Set("Content-Type", contentType)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("WebSDK request failed: %w", err)
	}
	return response, nil
}

func webSDKContentIsText(contentType string, payload []byte) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch {
	case strings.HasPrefix(mediaType, "text/"):
		return true
	case mediaType == "application/xml", mediaType == "application/json",
		mediaType == "application/problem+json", strings.HasSuffix(mediaType, "+xml"),
		strings.HasSuffix(mediaType, "+json"):
		return true
	}
	trimmed := strings.TrimSpace(string(payload))
	return strings.HasPrefix(trimmed, "<") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}
