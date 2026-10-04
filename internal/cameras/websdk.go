package cameras

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxWebSDKResponseSize int64 = 2 << 20

type WebSDKProbeRequest struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	HTTPS    bool   `json:"https"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type WebSDKDeviceInfo struct {
	DeviceName           string `json:"device_name,omitempty"`
	DeviceID             string `json:"device_id,omitempty"`
	DeviceType           string `json:"device_type,omitempty"`
	Model                string `json:"model,omitempty"`
	SerialNumber         string `json:"serial_number,omitempty"`
	MACAddress           string `json:"mac_address,omitempty"`
	FirmwareVersion      string `json:"firmware_version,omitempty"`
	FirmwareReleasedDate string `json:"firmware_released_date,omitempty"`
	EncoderVersion       string `json:"encoder_version,omitempty"`
	EncoderReleasedDate  string `json:"encoder_released_date,omitempty"`
}

type WebSDKPortInfo struct {
	HTTPPort   int `json:"http_port,omitempty"`
	RTSPPort   int `json:"rtsp_port,omitempty"`
	DevicePort int `json:"device_port,omitempty"`
}

type WebSDKChannel struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Name            string `json:"name,omitempty"`
	InputPort       string `json:"input_port,omitempty"`
	VideoFormat     string `json:"video_format,omitempty"`
	Online          *bool  `json:"online,omitempty"`
	IPAddress       string `json:"ip_address,omitempty"`
	ManagePort      int    `json:"manage_port,omitempty"`
	SourceInputPort string `json:"source_input_port,omitempty"`
	ProxyProtocol   string `json:"proxy_protocol,omitempty"`
	StreamType      string `json:"stream_type,omitempty"`
}

type WebSDKStream struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type WebSDKProbeResult struct {
	OK       bool             `json:"ok"`
	Address  string           `json:"address"`
	Port     int              `json:"port"`
	HTTPS    bool             `json:"https"`
	Backend  string           `json:"backend"`
	Device   WebSDKDeviceInfo `json:"device"`
	Ports    WebSDKPortInfo   `json:"ports"`
	Channels []WebSDKChannel  `json:"channels"`
	Streams  []WebSDKStream   `json:"streams"`
	Warnings []string         `json:"warnings,omitempty"`
}

type WebSDKHTTPError struct {
	Path   string
	Status int
}

func (e WebSDKHTTPError) Error() string {
	return fmt.Sprintf("WebSDK/ISAPI request %s returned HTTP %d", e.Path, e.Status)
}

func (s *Service) ProbeWebSDK(ctx context.Context, request WebSDKProbeRequest) (WebSDKProbeResult, error) {
	request.Address = strings.TrimSpace(request.Address)
	request.Username = strings.TrimSpace(request.Username)
	if request.Port == 0 {
		if request.HTTPS {
			request.Port = 443
		} else {
			request.Port = 80
		}
	}
	if err := validateWebSDKRequest(request); err != nil {
		return WebSDKProbeResult{}, err
	}

	client := newWebSDKClient(request)

	if _, err := client.get(ctx, "/ISAPI/Security/userCheck?format=json"); err != nil {
		return WebSDKProbeResult{}, err
	}

	devicePayload, err := client.get(ctx, "/ISAPI/System/deviceInfo")
	if err != nil {
		return WebSDKProbeResult{}, err
	}
	device, err := parseWebSDKDeviceInfo(devicePayload)
	if err != nil {
		return WebSDKProbeResult{}, fmt.Errorf("parse WebSDK device info: %w", err)
	}

	result := WebSDKProbeResult{
		OK:       true,
		Address:  request.Address,
		Port:     request.Port,
		HTTPS:    request.HTTPS,
		Backend:  "HCWebSDK/ISAPI",
		Device:   device,
		Channels: []WebSDKChannel{},
		Streams:  []WebSDKStream{},
	}

	if payload, requestErr := client.get(ctx, "/ISAPI/System/Video/inputs/channels"); requestErr == nil {
		channels, parseErr := parseWebSDKAnalogChannels(payload)
		if parseErr != nil {
			result.Warnings = append(result.Warnings, "could not parse analog channel list")
		} else {
			result.Channels = append(result.Channels, channels...)
		}
	} else if !isOptionalWebSDKStatus(requestErr) {
		result.Warnings = append(result.Warnings, "analog channel list is unavailable")
	}

	digitalNames := map[string]string{}
	if payload, requestErr := client.get(ctx, "/ISAPI/ContentMgmt/InputProxy/channels"); requestErr == nil {
		names, parseErr := parseWebSDKDigitalChannels(payload)
		if parseErr != nil {
			result.Warnings = append(result.Warnings, "could not parse digital channel list")
		} else {
			digitalNames = names
		}
	} else if !isOptionalWebSDKStatus(requestErr) {
		result.Warnings = append(result.Warnings, "digital channel list is unavailable")
	}

	if payload, requestErr := client.get(ctx, "/ISAPI/ContentMgmt/InputProxy/channels/status"); requestErr == nil {
		channels, parseErr := parseWebSDKDigitalChannelStatus(payload, digitalNames)
		if parseErr != nil {
			result.Warnings = append(result.Warnings, "could not parse digital channel status")
		} else {
			result.Channels = append(result.Channels, channels...)
		}
	} else if len(digitalNames) > 0 {
		for id, name := range digitalNames {
			result.Channels = append(result.Channels, WebSDKChannel{ID: id, Kind: "digital", Name: name})
		}
	}

	if payload, requestErr := client.get(ctx, "/ISAPI/Security/adminAccesses"); requestErr == nil {
		ports, parseErr := parseWebSDKPorts(payload)
		if parseErr != nil {
			result.Warnings = append(result.Warnings, "could not parse device service ports")
		} else {
			result.Ports = ports
		}
	} else if !isOptionalWebSDKStatus(requestErr) {
		result.Warnings = append(result.Warnings, "device service ports are unavailable")
	}

	streamPath := "/ISAPI/ContentMgmt/StreamingProxy/channels"
	for _, channel := range result.Channels {
		if channel.Kind == "analog" {
			streamPath = "/ISAPI/Streaming/channels"
			break
		}
	}
	if payload, requestErr := client.get(ctx, streamPath); requestErr == nil {
		streams, parseErr := parseWebSDKStreams(payload)
		if parseErr != nil {
			result.Warnings = append(result.Warnings, "could not parse streaming channel list")
		} else {
			result.Streams = streams
		}
	} else if !isOptionalWebSDKStatus(requestErr) {
		result.Warnings = append(result.Warnings, "streaming channel list is unavailable")
	}

	sortWebSDKChannels(result.Channels)
	return result, nil
}

func validateWebSDKRequest(request WebSDKProbeRequest) error {
	if request.Address == "" {
		return fmt.Errorf("%w: camera address is required", ErrInvalidTarget)
	}
	ip := net.ParseIP(request.Address)
	if ip == nil {
		return fmt.Errorf("%w: camera address must be a literal IP", ErrInvalidTarget)
	}
	if !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
		return fmt.Errorf("%w: camera address must be private or link-local", ErrInvalidTarget)
	}
	if request.Port < 1 || request.Port > 65535 {
		return fmt.Errorf("%w: camera Web port must be between 1 and 65535", ErrInvalidTarget)
	}
	if request.Username == "" {
		return fmt.Errorf("%w: username is required", ErrInvalidTarget)
	}
	return nil
}

type webSDKClient struct {
	client   *http.Client
	baseURL  string
	username string
	password string
}

func newWebSDKClient(request WebSDKProbeRequest) *webSDKClient {
	scheme := "http"
	if request.HTTPS {
		scheme = "https"
	}
	transport := &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			// Hikvision devices commonly ship with a self-signed certificate.
			// The destination is already restricted to a literal private/link-local IP.
			InsecureSkipVerify: request.HTTPS, //nolint:gosec
		},
	}
	return &webSDKClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		baseURL:  scheme + "://" + net.JoinHostPort(request.Address, strconv.Itoa(request.Port)),
		username: request.Username,
		password: request.Password,
	}
}

func (c *webSDKClient) get(ctx context.Context, path string) ([]byte, error) {
	return c.request(ctx, http.MethodGet, path, "")
}

func (c *webSDKClient) request(ctx context.Context, method, path, body string) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("WebSDK/ISAPI path must be absolute")
	}
	target := c.baseURL + path
	response, err := c.do(ctx, method, target, body, "")
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusUnauthorized {
		challenges := response.Header.Values("WWW-Authenticate")
		_ = response.Body.Close()
		auth, authErr := buildWebSDKAuthorization(challenges, c.username, c.password, method, path)
		if authErr != nil {
			return nil, authErr
		}
		response, err = c.do(ctx, method, target, body, auth)
		if err != nil {
			return nil, err
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, WebSDKHTTPError{Path: path, Status: response.StatusCode}
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxWebSDKResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read WebSDK/ISAPI response: %w", err)
	}
	if int64(len(payload)) > maxWebSDKResponseSize {
		return nil, errors.New("WebSDK/ISAPI response exceeds safety limit")
	}
	return payload, nil
}

func (c *webSDKClient) do(
	ctx context.Context,
	method string,
	target string,
	body string,
	authorization string,
) (*http.Response, error) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/xml, application/json")
	if body != "" {
		request.Header.Set("Content-Type", "application/xml")
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("WebSDK/ISAPI request failed: %w", err)
	}
	return response, nil
}

func isOptionalWebSDKStatus(err error) bool {
	var httpErr WebSDKHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.Status == http.StatusNotFound ||
		httpErr.Status == http.StatusMethodNotAllowed ||
		httpErr.Status == http.StatusForbidden
}

type webSDKDeviceInfoXML struct {
	DeviceName           string `xml:"deviceName"`
	DeviceID             string `xml:"deviceID"`
	DeviceType           string `xml:"deviceType"`
	Model                string `xml:"model"`
	SerialNumber         string `xml:"serialNumber"`
	MACAddress           string `xml:"macAddress"`
	FirmwareVersion      string `xml:"firmwareVersion"`
	FirmwareReleasedDate string `xml:"firmwareReleasedDate"`
	EncoderVersion       string `xml:"encoderVersion"`
	EncoderReleasedDate  string `xml:"encoderReleasedDate"`
}

func parseWebSDKDeviceInfo(payload []byte) (WebSDKDeviceInfo, error) {
	var value webSDKDeviceInfoXML
	if err := xml.Unmarshal(payload, &value); err != nil {
		return WebSDKDeviceInfo{}, err
	}
	return WebSDKDeviceInfo{
		DeviceName:           cleanWebSDKText(value.DeviceName),
		DeviceID:             cleanWebSDKText(value.DeviceID),
		DeviceType:           cleanWebSDKText(value.DeviceType),
		Model:                cleanWebSDKText(value.Model),
		SerialNumber:         cleanWebSDKText(value.SerialNumber),
		MACAddress:           cleanWebSDKText(value.MACAddress),
		FirmwareVersion:      cleanWebSDKText(value.FirmwareVersion),
		FirmwareReleasedDate: cleanWebSDKText(value.FirmwareReleasedDate),
		EncoderVersion:       cleanWebSDKText(value.EncoderVersion),
		EncoderReleasedDate:  cleanWebSDKText(value.EncoderReleasedDate),
	}, nil
}

type webSDKAnalogChannelsXML struct {
	Channels []struct {
		ID          string `xml:"id"`
		InputPort   string `xml:"inputPort"`
		Name        string `xml:"name"`
		VideoFormat string `xml:"videoFormat"`
	} `xml:"VideoInputChannel"`
}

func parseWebSDKAnalogChannels(payload []byte) ([]WebSDKChannel, error) {
	var value webSDKAnalogChannelsXML
	if err := xml.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	result := make([]WebSDKChannel, 0, len(value.Channels))
	for _, channel := range value.Channels {
		result = append(result, WebSDKChannel{
			ID:          cleanWebSDKText(channel.ID),
			Kind:        "analog",
			Name:        cleanWebSDKText(channel.Name),
			InputPort:   cleanWebSDKText(channel.InputPort),
			VideoFormat: cleanWebSDKText(channel.VideoFormat),
		})
	}
	return result, nil
}

type webSDKDigitalChannelsXML struct {
	Channels []struct {
		ID   string `xml:"id"`
		Name string `xml:"name"`
	} `xml:"InputProxyChannel"`
}

func parseWebSDKDigitalChannels(payload []byte) (map[string]string, error) {
	var value webSDKDigitalChannelsXML
	if err := xml.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(value.Channels))
	for _, channel := range value.Channels {
		id := cleanWebSDKText(channel.ID)
		if id != "" {
			result[id] = cleanWebSDKText(channel.Name)
		}
	}
	return result, nil
}

type webSDKDigitalChannelStatusXML struct {
	Channels []struct {
		ID         string `xml:"id"`
		Descriptor struct {
			ProxyProtocol   string `xml:"proxyProtocol"`
			IPAddress       string `xml:"ipAddress"`
			ManagePortNo    int    `xml:"managePortNo"`
			SourceInputPort string `xml:"srcInputPort"`
			StreamType      string `xml:"streamType"`
			Online          string `xml:"online"`
		} `xml:"sourceInputPortDescriptor"`
	} `xml:"InputProxyChannelStatus"`
}

func parseWebSDKDigitalChannelStatus(
	payload []byte,
	names map[string]string,
) ([]WebSDKChannel, error) {
	var value webSDKDigitalChannelStatusXML
	if err := xml.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	result := make([]WebSDKChannel, 0, len(value.Channels))
	seen := make(map[string]bool, len(value.Channels))
	for _, channel := range value.Channels {
		id := cleanWebSDKText(channel.ID)
		online := parseWebSDKBool(channel.Descriptor.Online)
		result = append(result, WebSDKChannel{
			ID:              id,
			Kind:            "digital",
			Name:            cleanWebSDKText(names[id]),
			Online:          online,
			IPAddress:       cleanWebSDKText(channel.Descriptor.IPAddress),
			ManagePort:      channel.Descriptor.ManagePortNo,
			SourceInputPort: cleanWebSDKText(channel.Descriptor.SourceInputPort),
			ProxyProtocol:   cleanWebSDKText(channel.Descriptor.ProxyProtocol),
			StreamType:      cleanWebSDKText(channel.Descriptor.StreamType),
		})
		seen[id] = true
	}
	for id, name := range names {
		if !seen[id] {
			result = append(result, WebSDKChannel{ID: id, Kind: "digital", Name: cleanWebSDKText(name)})
		}
	}
	return result, nil
}

type webSDKAdminAccessXML struct {
	Protocols []struct {
		Protocol string `xml:"protocol"`
		PortNo   int    `xml:"portNo"`
	} `xml:"AdminAccessProtocol"`
}

func parseWebSDKPorts(payload []byte) (WebSDKPortInfo, error) {
	var value webSDKAdminAccessXML
	if err := xml.Unmarshal(payload, &value); err != nil {
		return WebSDKPortInfo{}, err
	}
	var result WebSDKPortInfo
	for _, item := range value.Protocols {
		switch strings.ToLower(strings.TrimSpace(item.Protocol)) {
		case "http":
			result.HTTPPort = item.PortNo
		case "rtsp":
			result.RTSPPort = item.PortNo
		case "dev_manage":
			result.DevicePort = item.PortNo
		}
	}
	return result, nil
}

type webSDKStreamsXML struct {
	Channels []struct {
		ID      string `xml:"id"`
		Name    string `xml:"channelName"`
		Enabled string `xml:"enabled"`
	} `xml:"StreamingChannel"`
	ProxyChannels []struct {
		ID      string `xml:"id"`
		Name    string `xml:"channelName"`
		Enabled string `xml:"enabled"`
	} `xml:"StreamingProxyChannel"`
}

func parseWebSDKStreams(payload []byte) ([]WebSDKStream, error) {
	var value webSDKStreamsXML
	if err := xml.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	result := make([]WebSDKStream, 0, len(value.Channels)+len(value.ProxyChannels))
	appendStream := func(id, name, enabled string) {
		id = cleanWebSDKText(id)
		if id == "" {
			return
		}
		result = append(result, WebSDKStream{
			ID:      id,
			Name:    cleanWebSDKText(name),
			Enabled: parseWebSDKBool(enabled),
		})
	}
	for _, channel := range value.Channels {
		appendStream(channel.ID, channel.Name, channel.Enabled)
	}
	for _, channel := range value.ProxyChannels {
		appendStream(channel.ID, channel.Name, channel.Enabled)
	}
	return result, nil
}

func cleanWebSDKText(value string) string {
	return strings.TrimSpace(strings.ToValidUTF8(value, "�"))
}

func parseWebSDKBool(value string) *bool {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "true", "1", "yes":
		result := true
		return &result
	case "false", "0", "no":
		result := false
		return &result
	default:
		return nil
	}
}

func sortWebSDKChannels(channels []WebSDKChannel) {
	for i := 1; i < len(channels); i++ {
		for j := i; j > 0 && webSDKChannelLess(channels[j], channels[j-1]); j-- {
			channels[j], channels[j-1] = channels[j-1], channels[j]
		}
	}
}

func webSDKChannelLess(left, right WebSDKChannel) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	leftID, leftErr := strconv.Atoi(left.ID)
	rightID, rightErr := strconv.Atoi(right.ID)
	if leftErr == nil && rightErr == nil {
		return leftID < rightID
	}
	return left.ID < right.ID
}

type webSDKDigestChallenge struct {
	Realm     string
	Nonce     string
	Opaque    string
	Algorithm string
	QOP       string
}

func buildWebSDKAuthorization(
	challenges []string,
	username string,
	password string,
	method string,
	uri string,
) (string, error) {
	for _, challenge := range challenges {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(challenge)), "digest ") {
			return buildWebSDKDigestAuthorization(challenge, username, password, method, uri)
		}
	}
	for _, challenge := range challenges {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(challenge)), "basic") {
			token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
			return "Basic " + token, nil
		}
	}
	return "", errors.New("camera authentication challenge is unsupported")
}

func buildWebSDKDigestAuthorization(
	challengeValue string,
	username string,
	password string,
	method string,
	uri string,
) (string, error) {
	challenge, err := parseWebSDKDigestChallenge(challengeValue)
	if err != nil {
		return "", err
	}
	algorithm := strings.ToUpper(strings.TrimSpace(challenge.Algorithm))
	session := strings.HasSuffix(algorithm, "-SESS")
	baseAlgorithm := strings.TrimSuffix(algorithm, "-SESS")
	if baseAlgorithm != "MD5" && baseAlgorithm != "SHA-256" {
		return "", fmt.Errorf("HTTP Digest algorithm %s is unsupported", challenge.Algorithm)
	}

	cnonceRaw := make([]byte, 16)
	if _, err := rand.Read(cnonceRaw); err != nil {
		return "", fmt.Errorf("generate HTTP Digest cnonce: %w", err)
	}
	cnonce := hex.EncodeToString(cnonceRaw)
	nc := "00000001"
	hash := func(value string) string {
		if baseAlgorithm == "SHA-256" {
			sum := sha256.Sum256([]byte(value))
			return hex.EncodeToString(sum[:])
		}
		sum := md5.Sum([]byte(value))
		return hex.EncodeToString(sum[:])
	}

	ha1 := hash(username + ":" + challenge.Realm + ":" + password)
	if session {
		ha1 = hash(ha1 + ":" + challenge.Nonce + ":" + cnonce)
	}
	ha2 := hash(strings.ToUpper(method) + ":" + uri)

	var response string
	if challenge.QOP != "" {
		response = hash(ha1 + ":" + challenge.Nonce + ":" + nc + ":" + cnonce + ":" + challenge.QOP + ":" + ha2)
	} else {
		response = hash(ha1 + ":" + challenge.Nonce + ":" + ha2)
	}

	parts := []string{
		"username=\"" + escapeWebSDKDigestValue(username) + "\"",
		"realm=\"" + escapeWebSDKDigestValue(challenge.Realm) + "\"",
		"nonce=\"" + escapeWebSDKDigestValue(challenge.Nonce) + "\"",
		"uri=\"" + escapeWebSDKDigestValue(uri) + "\"",
		"response=\"" + response + "\"",
		"algorithm=" + challenge.Algorithm,
	}
	if challenge.Opaque != "" {
		parts = append(parts, "opaque=\""+escapeWebSDKDigestValue(challenge.Opaque)+"\"")
	}
	if challenge.QOP != "" {
		parts = append(parts, "qop="+challenge.QOP, "nc="+nc, "cnonce=\""+cnonce+"\"")
	}
	return "Digest " + strings.Join(parts, ", "), nil
}

func parseWebSDKDigestChallenge(value string) (webSDKDigestChallenge, error) {
	value = strings.TrimSpace(value)
	if len(value) < len("Digest ") || !strings.EqualFold(value[:len("Digest ")], "Digest ") {
		return webSDKDigestChallenge{}, errors.New("HTTP Digest challenge missing")
	}
	params := parseWebSDKAuthParams(strings.TrimSpace(value[len("Digest "):]))
	challenge := webSDKDigestChallenge{
		Realm:     params["realm"],
		Nonce:     params["nonce"],
		Opaque:    params["opaque"],
		Algorithm: strings.TrimSpace(params["algorithm"]),
	}
	if challenge.Realm == "" || challenge.Nonce == "" {
		return webSDKDigestChallenge{}, errors.New("HTTP Digest challenge is incomplete")
	}
	if challenge.Algorithm == "" {
		challenge.Algorithm = "MD5"
	}
	qop := strings.TrimSpace(params["qop"])
	if qop != "" {
		for _, candidate := range strings.Split(qop, ",") {
			if strings.EqualFold(strings.TrimSpace(candidate), "auth") {
				challenge.QOP = "auth"
				break
			}
		}
		if challenge.QOP == "" {
			return webSDKDigestChallenge{}, errors.New("HTTP Digest qop is unsupported")
		}
	}
	return challenge, nil
}

func parseWebSDKAuthParams(value string) map[string]string {
	result := make(map[string]string)
	for len(value) > 0 {
		value = strings.TrimLeft(value, " ,\t")
		if value == "" {
			break
		}
		equal := strings.IndexByte(value, '=')
		if equal <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(value[:equal]))
		value = strings.TrimSpace(value[equal+1:])
		if value == "" {
			result[key] = ""
			break
		}

		var parsed string
		if value[0] == '"' {
			var builder strings.Builder
			escaped := false
			index := 1
			for ; index < len(value); index++ {
				character := value[index]
				switch {
				case escaped:
					builder.WriteByte(character)
					escaped = false
				case character == '\\':
					escaped = true
				case character == '"':
					index++
					goto quotedDone
				default:
					builder.WriteByte(character)
				}
			}
		quotedDone:
			parsed = builder.String()
			value = value[index:]
		} else {
			index := strings.IndexByte(value, ',')
			if index < 0 {
				parsed = strings.TrimSpace(value)
				value = ""
			} else {
				parsed = strings.TrimSpace(value[:index])
				value = value[index+1:]
			}
		}
		result[key] = parsed
	}
	return result
}

func escapeWebSDKDigestValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "\"", "\\\"")
}

func webSDKRequestURI(path string) string {
	parsed, err := url.Parse(path)
	if err != nil {
		return path
	}
	return parsed.RequestURI()
}
