package nvr

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	onvifDeviceNamespace = "http://www.onvif.org/ver10/device/wsdl"
	onvifMediaNamespace  = "http://www.onvif.org/ver10/media/wsdl"
)

type ONVIFProfile struct {
	Token      string  `json:"token"`
	Name       string  `json:"name"`
	StreamURI  string  `json:"stream_uri"`
	Codec      string  `json:"codec,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	FPS        float64 `json:"fps,omitempty"`
	BitrateBPS int64   `json:"bitrate_bps,omitempty"`
	HasAudio   bool    `json:"has_audio"`
}

type ONVIFProfileRequest struct {
	Address  string `json:"address"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type ONVIFImportInput struct {
	Name             string `json:"name"`
	Address          string `json:"address"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	MainProfileToken string `json:"main_profile_token"`
	SubProfileToken  string `json:"sub_profile_token,omitempty"`
	Transport        string `json:"transport,omitempty"`
	RecordingMode    string `json:"recording_mode,omitempty"`
	AudioEnabled     bool   `json:"audio_enabled"`
}

type ONVIFClient interface {
	Profiles(context.Context, string, CameraCredential) ([]ONVIFProfile, error)
}

type SOAPONVIFClient struct {
	client  *http.Client
	timeout time.Duration
}

func NewSOAPONVIFClient() *SOAPONVIFClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxIdleConnsPerHost = 2
	return &SOAPONVIFClient{
		client:  &http.Client{Transport: transport, Timeout: 12 * time.Second},
		timeout: 12 * time.Second,
	}
}

func (c *SOAPONVIFClient) Profiles(
	ctx context.Context,
	deviceAddress string,
	credential CameraCredential,
) ([]ONVIFProfile, error) {
	if c == nil || c.client == nil {
		return nil, ErrONVIFConnection
	}
	deviceURL, expectedIP, err := validateLocalONVIFEndpoint(deviceAddress)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(credential.Username) == "" && credential.Password != "" {
		return nil, errors.New("camera username is required when a password is supplied")
	}

	timeout := c.timeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := c.soap(
		callCtx,
		deviceURL.String(),
		credential,
		`<tds:GetCapabilities xmlns:tds="`+onvifDeviceNamespace+`"><tds:Category>Media</tds:Category></tds:GetCapabilities>`,
	)
	if err != nil {
		return nil, err
	}
	mediaAddress, err := parseONVIFMediaXAddr(raw)
	if err != nil {
		return nil, err
	}
	mediaAddress, err = normalizeONVIFServiceAddress(mediaAddress, expectedIP)
	if err != nil {
		return nil, err
	}

	raw, err = c.soap(
		callCtx,
		mediaAddress,
		credential,
		`<trt:GetProfiles xmlns:trt="`+onvifMediaNamespace+`"/>`,
	)
	if err != nil {
		return nil, err
	}
	profiles, err := parseONVIFProfiles(raw)
	if err != nil {
		return nil, err
	}
	if len(profiles) > 16 {
		profiles = profiles[:16]
	}

	result := make([]ONVIFProfile, 0, len(profiles))
	for _, profile := range profiles {
		body := `<trt:GetStreamUri xmlns:trt="`+onvifMediaNamespace+`">` +
			`<trt:StreamSetup>` +
			`<tt:Stream xmlns:tt="http://www.onvif.org/ver10/schema">RTP-Unicast</tt:Stream>` +
			`<tt:Transport xmlns:tt="http://www.onvif.org/ver10/schema"><tt:Protocol>RTSP</tt:Protocol></tt:Transport>` +
			`</trt:StreamSetup><trt:ProfileToken>` + xmlEscape(profile.Token) +
			`</trt:ProfileToken></trt:GetStreamUri>`
		raw, err = c.soap(callCtx, mediaAddress, credential, body)
		if err != nil {
			return nil, err
		}
		streamURI, err := parseONVIFStreamURI(raw)
		if err != nil {
			return nil, err
		}
		profile.StreamURI, err = sanitizeONVIFRTSPURI(streamURI, expectedIP)
		if err != nil {
			return nil, err
		}
		result = append(result, profile)
	}

	sort.SliceStable(result, func(i, j int) bool {
		left := result[i].Width * result[i].Height
		right := result[j].Width * result[j].Height
		if left == right {
			return result[i].FPS > result[j].FPS
		}
		return left > right
	})
	return result, nil
}

func (c *SOAPONVIFClient) soap(
	ctx context.Context,
	endpoint string,
	credential CameraCredential,
	body string,
) ([]byte, error) {
	parsed, _, err := validateLocalONVIFEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	securityHeader, err := onvifSecurityHeader(credential)
	if err != nil {
		return nil, err
	}
	payload := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		securityHeader + `<s:Body>` + body + `</s:Body></s:Envelope>`

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewBufferString(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: build request", ErrONVIFConnection)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("Accept", "application/soap+xml, application/xml, text/xml")
	req.Header.Set("User-Agent", "Home-AI-Core ONVIF")

	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: timeout", ErrONVIFConnection)
		}
		return nil, fmt.Errorf("%w: request failed", ErrONVIFConnection)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read response", ErrONVIFConnection)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrONVIFAuthentication
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrONVIFConnection, resp.StatusCode)
	}
	if fault := parseSOAPFault(raw); fault != "" {
		lower := strings.ToLower(fault)
		if strings.Contains(lower, "notauthorized") ||
			strings.Contains(lower, "unauthorized") ||
			strings.Contains(lower, "authentication") {
			return nil, ErrONVIFAuthentication
		}
		return nil, fmt.Errorf("%w: %s", ErrONVIFConnection, fault)
	}
	return raw, nil
}

func onvifSecurityHeader(credential CameraCredential) (string, error) {
	username := strings.TrimSpace(credential.Username)
	if username == "" && credential.Password == "" {
		return `<s:Header/>`, nil
	}
	if username == "" {
		return "", errors.New("camera username is required when a password is supplied")
	}
	nonce := make([]byte, 20)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate ONVIF nonce: %w", err)
	}
	created := time.Now().UTC().Format(time.RFC3339Nano)
	hash := sha1.New()
	_, _ = hash.Write(nonce)
	_, _ = hash.Write([]byte(created))
	_, _ = hash.Write([]byte(credential.Password))
	digest := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	encodedNonce := base64.StdEncoding.EncodeToString(nonce)

	return `<s:Header><wsse:Security s:mustUnderstand="1"` +
		` xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"` +
		` xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">` +
		`<wsse:UsernameToken><wsse:Username>` + xmlEscape(username) + `</wsse:Username>` +
		`<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">` +
		digest + `</wsse:Password>` +
		`<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">` +
		encodedNonce + `</wsse:Nonce><wsu:Created>` + created +
		`</wsu:Created></wsse:UsernameToken></wsse:Security></s:Header>`, nil
}

func xmlEscape(value string) string {
	var buffer strings.Builder
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.String()
}

func parseONVIFMediaXAddr(raw []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	inMedia := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w: malformed capabilities response", ErrONVIFConnection)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "Media" {
				inMedia = true
				for _, attribute := range value.Attr {
					if attribute.Name.Local == "XAddr" && strings.TrimSpace(attribute.Value) != "" {
						return strings.TrimSpace(attribute.Value), nil
					}
				}
			} else if inMedia && value.Name.Local == "XAddr" {
				var address string
				if decoder.DecodeElement(&address, &value) == nil && strings.TrimSpace(address) != "" {
					return strings.TrimSpace(address), nil
				}
			}
		case xml.EndElement:
			if value.Name.Local == "Media" {
				inMedia = false
			}
		}
	}
	return "", fmt.Errorf("%w: media service missing", ErrONVIFConnection)
}

func parseONVIFProfiles(raw []byte) ([]ONVIFProfile, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	result := make([]ONVIFProfile, 0, 4)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: malformed profile response", ErrONVIFConnection)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Profiles" {
			continue
		}
		profile, err := decodeONVIFProfile(decoder, start)
		if err != nil {
			return nil, err
		}
		if profile.Token != "" {
			result = append(result, profile)
		}
	}
	if len(result) == 0 {
		return nil, ErrONVIFNoMediaProfiles
	}
	return result, nil
}

func decodeONVIFProfile(decoder *xml.Decoder, start xml.StartElement) (ONVIFProfile, error) {
	var profile ONVIFProfile
	for _, attribute := range start.Attr {
		if attribute.Name.Local == "token" {
			profile.Token = strings.TrimSpace(attribute.Value)
		}
	}
	depth := 1
	inVideo := false
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return ONVIFProfile{}, fmt.Errorf("%w: malformed profile", ErrONVIFConnection)
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			switch value.Name.Local {
			case "Name":
				if profile.Name == "" {
					_ = decoder.DecodeElement(&profile.Name, &value)
					depth--
					profile.Name = strings.TrimSpace(profile.Name)
				}
			case "VideoEncoderConfiguration":
				inVideo = true
			case "AudioEncoderConfiguration":
				profile.HasAudio = true
			case "Encoding":
				if inVideo {
					var text string
					_ = decoder.DecodeElement(&text, &value)
					depth--
					profile.Codec = strings.ToLower(strings.TrimSpace(text))
				}
			case "Width":
				if inVideo {
					var number int
					_ = decoder.DecodeElement(&number, &value)
					depth--
					profile.Width = number
				}
			case "Height":
				if inVideo {
					var number int
					_ = decoder.DecodeElement(&number, &value)
					depth--
					profile.Height = number
				}
			case "FrameRateLimit":
				if inVideo {
					var number float64
					_ = decoder.DecodeElement(&number, &value)
					depth--
					profile.FPS = number
				}
			case "BitrateLimit":
				if inVideo {
					var number int64
					_ = decoder.DecodeElement(&number, &value)
					depth--
					if number > 0 && number < 1_000_000 {
						number *= 1000
					}
					profile.BitrateBPS = number
				}
			}
		case xml.EndElement:
			if value.Name.Local == "VideoEncoderConfiguration" {
				inVideo = false
			}
			depth--
		}
	}
	return profile, nil
}

func parseONVIFStreamURI(raw []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w: malformed stream URI response", ErrONVIFConnection)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Uri" {
			continue
		}
		var value string
		if decoder.DecodeElement(&value, &start) == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("%w: RTSP stream URI missing", ErrONVIFConnection)
}

func parseSOAPFault(raw []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Text" {
			continue
		}
		var value string
		if decoder.DecodeElement(&value, &start) == nil {
			value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
			if value != "" {
				return value
			}
		}
	}
}

func sanitizeONVIFRTSPURI(value string, expectedIP net.IP) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || expectedIP == nil {
		return "", ErrONVIFInvalidEndpoint
	}
	if parsed.Scheme != "rtsp" && parsed.Scheme != "rtsps" {
		return "", ErrONVIFInvalidEndpoint
	}
	parsed.User = nil
	parsed.Fragment = ""
	hostIP := net.ParseIP(parsed.Hostname())
	if hostIP == nil || !hostIP.Equal(expectedIP) {
		port := parsed.Port()
		if port != "" {
			parsed.Host = net.JoinHostPort(expectedIP.String(), port)
		} else {
			parsed.Host = expectedIP.String()
		}
	}
	return normalizeRTSPAddress(parsed.String())
}
