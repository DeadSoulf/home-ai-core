package nvr

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CameraDiscoveryService struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Address  string `json:"address,omitempty"`
}

type CameraDiscoveryDevice struct {
	ID                   string                   `json:"id"`
	Name                 string                   `json:"name"`
	IP                   string                   `json:"ip"`
	MAC                  string                   `json:"mac,omitempty"`
	Vendor               string                   `json:"vendor,omitempty"`
	Model                string                   `json:"model,omitempty"`
	DeviceType           string                   `json:"device_type"`
	Confidence           string                   `json:"confidence"`
	Sources              []string                 `json:"sources"`
	Services             []CameraDiscoveryService `json:"services"`
	ONVIFAddress         string                   `json:"onvif_address,omitempty"`
	RTSPAddressHint      string                   `json:"rtsp_address_hint,omitempty"`
	SubstreamAddressHint string                   `json:"substream_address_hint,omitempty"`
}

type CameraDiscoveryEngine struct {
	onvif ONVIFDiscoverer
}

type cameraNetworkEvidence struct {
	ip          string
	openPorts   map[int]bool
	rtspPorts   map[int]bool
	httpPorts   map[int]bool
	httpsPorts  map[int]bool
	fingerprint strings.Builder
}

var cameraDiscoveryPorts = []int{
	80, 81, 443, 554, 8000, 8080, 8554, 8899, 2020, 37777, 34567, 9000,
}

var cameraVendorSignatures = []struct {
	vendor   string
	keywords []string
}{
	{"Hikvision", []string{"hikvision", "isapi", "webcomponents"}},
	{"Dahua", []string{"dahua", "dhip", "configtool"}},
	{"Uniview", []string{"uniview", "unv ", "eztools"}},
	{"Hanwha", []string{"hanwha", "wisenet", "samsung techwin"}},
	{"Axis", []string{"axis communications", "axis camera", "axis video"}},
	{"Bosch", []string{"bosch security", "bosch video"}},
}

func NewCameraDiscoveryEngine(onvif ONVIFDiscoverer) *CameraDiscoveryEngine {
	return &CameraDiscoveryEngine{onvif: onvif}
}

func (e *CameraDiscoveryEngine) Discover(ctx context.Context) ([]CameraDiscoveryDevice, error) {
	discoverCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var (
		onvifDevices []ONVIFDevice
		network      map[string]*cameraNetworkEvidence
		onvifErr     error
		networkErr   error
		wg           sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		if e != nil && e.onvif != nil {
			onvifDevices, onvifErr = e.onvif.Discover(discoverCtx)
		}
	}()
	go func() {
		defer wg.Done()
		network, networkErr = scanCameraNetwork(discoverCtx)
	}()
	wg.Wait()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if network == nil {
		network = map[string]*cameraNetworkEvidence{}
	}

	found := make(map[string]*CameraDiscoveryDevice)
	for _, device := range onvifDevices {
		ip := strings.TrimSpace(device.IP)
		if ip == "" {
			continue
		}
		entry := ensureDiscoveryDevice(found, ip)
		entry.Name = strings.TrimSpace(device.Name)
		if entry.Name == "" {
			entry.Name = "Camera " + ip
		}
		entry.ONVIFAddress = strings.TrimSpace(device.Address)
		addDiscoverySource(entry, "onvif")
		if entry.ONVIFAddress != "" {
			if parsed, err := url.Parse(entry.ONVIFAddress); err == nil {
				port := 80
				if parsed.Scheme == "https" {
					port = 443
				}
				if parsed.Port() != "" {
					if value, err := strconv.Atoi(parsed.Port()); err == nil {
						port = value
					}
				}
				addDiscoveryService(entry, parsed.Scheme, port, entry.ONVIFAddress)
			}
		}
		if model := onvifScopeValue(device.Scopes, []string{"/hardware/", "/model/"}); model != "" {
			entry.Model = model
		}
		if vendor := detectCameraVendor(strings.Join(append(device.Scopes, device.Name), " ")); vendor != "" {
			entry.Vendor = vendor
			addDiscoverySource(entry, "vendor:"+strings.ToLower(vendor))
		}
	}

	for ip, evidence := range network {
		entry := ensureDiscoveryDevice(found, ip)
		if entry.Name == "" {
			entry.Name = "Device " + ip
		}
		for port := range evidence.openPorts {
			addDiscoveryService(entry, "tcp", port, "")
		}
		for port := range evidence.httpPorts {
			address := fmt.Sprintf("http://%s", hostPortForDiscovery(ip, port, 80))
			addDiscoveryService(entry, "http", port, address)
			addDiscoverySource(entry, "http")
		}
		for port := range evidence.httpsPorts {
			address := fmt.Sprintf("https://%s", hostPortForDiscovery(ip, port, 443))
			addDiscoveryService(entry, "https", port, address)
			addDiscoverySource(entry, "https")
		}
		for port := range evidence.rtspPorts {
			address := fmt.Sprintf("rtsp://%s/", hostPortForDiscovery(ip, port, 554))
			addDiscoveryService(entry, "rtsp", port, address)
			if entry.RTSPAddressHint == "" {
				entry.RTSPAddressHint = address
			}
			addDiscoverySource(entry, "rtsp")
		}
		fingerprint := evidence.fingerprint.String()
		if entry.Vendor == "" {
			entry.Vendor = detectCameraVendor(fingerprint)
			if entry.Vendor != "" {
				addDiscoverySource(entry, "vendor:"+strings.ToLower(entry.Vendor))
			}
		}
		if entry.Model == "" {
			entry.Model = detectModelHint(fingerprint, entry.Vendor)
		}
	}

	arp := readARPTable()
	result := make([]CameraDiscoveryDevice, 0, len(found))
	for ip, entry := range found {
		entry.MAC = arp[ip]
		classifyDiscoveryDevice(entry)
		if entry.Confidence == "possible" &&
			len(entry.Sources) == 0 &&
			!hasCameraLikePort(entry.Services) {
			continue
		}
		sort.Strings(entry.Sources)
		sort.Slice(entry.Services, func(i, j int) bool {
			if entry.Services[i].Port == entry.Services[j].Port {
				return entry.Services[i].Protocol < entry.Services[j].Protocol
			}
			return entry.Services[i].Port < entry.Services[j].Port
		})
		result = append(result, *entry)
	}
	sort.Slice(result, func(i, j int) bool {
		left := net.ParseIP(result[i].IP)
		right := net.ParseIP(result[j].IP)
		if left4, right4 := left.To4(), right.To4(); left4 != nil && right4 != nil {
			for index := range left4 {
				if left4[index] != right4[index] {
					return left4[index] < right4[index]
				}
			}
		}
		return result[i].IP < result[j].IP
	})

	if len(result) == 0 && networkErr != nil && onvifErr != nil {
		return nil, fmt.Errorf("camera discovery failed: network: %v; onvif: %v", networkErr, onvifErr)
	}
	return result, nil
}

func ensureDiscoveryDevice(found map[string]*CameraDiscoveryDevice, ip string) *CameraDiscoveryDevice {
	if existing := found[ip]; existing != nil {
		return existing
	}
	sum := sha256.Sum256([]byte(ip))
	entry := &CameraDiscoveryDevice{
		ID:         "camera_discovery_" + hex.EncodeToString(sum[:8]),
		IP:         ip,
		DeviceType: "possible_camera",
		Confidence: "possible",
	}
	found[ip] = entry
	return entry
}

func addDiscoverySource(device *CameraDiscoveryDevice, source string) {
	source = strings.TrimSpace(source)
	if source == "" {
		return
	}
	for _, current := range device.Sources {
		if current == source {
			return
		}
	}
	device.Sources = append(device.Sources, source)
}

func addDiscoveryService(device *CameraDiscoveryDevice, protocol string, port int, address string) {
	for i := range device.Services {
		if device.Services[i].Protocol == protocol && device.Services[i].Port == port {
			if device.Services[i].Address == "" {
				device.Services[i].Address = address
			}
			return
		}
	}
	device.Services = append(device.Services, CameraDiscoveryService{
		Protocol: protocol,
		Port:     port,
		Address:  address,
	})
}

func classifyDiscoveryDevice(device *CameraDiscoveryDevice) {
	text := strings.ToLower(strings.Join([]string{device.Name, device.Vendor, device.Model}, " "))
	isRecorder := strings.Contains(text, " nvr") || strings.HasPrefix(text, "nvr") ||
		strings.Contains(text, " dvr") || strings.HasPrefix(text, "dvr") ||
		strings.Contains(text, "recorder")

	hasONVIF := device.ONVIFAddress != ""
	hasRTSP := false
	hasHTTP := false
	for _, service := range device.Services {
		switch service.Protocol {
		case "rtsp":
			hasRTSP = true
		case "http", "https":
			hasHTTP = true
		}
	}

	switch {
	case isRecorder:
		device.DeviceType = "recorder"
	case hasONVIF || hasRTSP:
		device.DeviceType = "camera"
	default:
		device.DeviceType = "possible_camera"
	}
	switch {
	case hasONVIF && hasRTSP:
		device.Confidence = "high"
	case hasONVIF || hasRTSP:
		device.Confidence = "medium"
	case device.Vendor != "" && hasHTTP:
		device.Confidence = "medium"
	default:
		device.Confidence = "possible"
	}
}

func hasCameraLikePort(services []CameraDiscoveryService) bool {
	for _, service := range services {
		switch service.Port {
		case 554, 8554, 8000, 8899, 2020, 37777, 34567:
			return true
		}
	}
	return false
}

func onvifScopeValue(scopes []string, markers []string) string {
	for _, scope := range scopes {
		parsed, err := url.Parse(scope)
		if err != nil {
			continue
		}
		for _, marker := range markers {
			index := strings.Index(strings.ToLower(parsed.Path), strings.ToLower(marker))
			if index < 0 {
				continue
			}
			value, err := url.PathUnescape(strings.Trim(parsed.Path[index+len(marker):], "/"))
			if err == nil && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func detectCameraVendor(text string) string {
	lower := strings.ToLower(text)
	for _, signature := range cameraVendorSignatures {
		for _, keyword := range signature.keywords {
			if strings.Contains(lower, strings.ToLower(keyword)) {
				return signature.vendor
			}
		}
	}
	return ""
}

func detectModelHint(text, vendor string) string {
	if vendor == "" {
		return ""
	}
	for _, line := range strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '<' || r == '>' || r == ';'
	}) {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "model") && len(trimmed) <= 96 {
			return trimmed
		}
	}
	return ""
}

func scanCameraNetwork(ctx context.Context) (map[string]*cameraNetworkEvidence, error) {
	targets := localONVIFScanTargets()
	if len(targets) == 0 {
		return map[string]*cameraNetworkEvidence{}, nil
	}
	type task struct {
		ip   net.IP
		port int
	}
	type observation struct {
		ip          string
		port        int
		rtsp        bool
		http        bool
		https       bool
		fingerprint string
	}

	tasks := make(chan task)
	observations := make(chan observation, 128)
	var wg sync.WaitGroup
	workerCount := 128
	if total := len(targets) * len(cameraDiscoveryPorts); total < workerCount {
		workerCount = total
	}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dialer := &net.Dialer{Timeout: 180 * time.Millisecond}
			for item := range tasks {
				if ctx.Err() != nil {
					return
				}
				address := net.JoinHostPort(item.ip.String(), strconv.Itoa(item.port))
				conn, err := dialer.DialContext(ctx, "tcp4", address)
				if err != nil {
					continue
				}
				obs := observation{ip: item.ip.String(), port: item.port}
				switch item.port {
				case 554, 8554:
					obs.rtsp = probeRTSPConnection(conn, item.ip.String(), item.port)
					_ = conn.Close()
				case 443:
					_ = conn.Close()
					obs.https, obs.fingerprint = probeCameraHTTP(ctx, item.ip.String(), item.port, true)
				case 80, 81, 8000, 8080, 8899, 2020:
					_ = conn.Close()
					obs.http, obs.fingerprint = probeCameraHTTP(ctx, item.ip.String(), item.port, false)
				default:
					_ = conn.Close()
				}
				select {
				case observations <- obs:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(tasks)
		for _, ip := range targets {
			for _, port := range cameraDiscoveryPorts {
				select {
				case tasks <- task{ip: ip, port: port}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go func() {
		wg.Wait()
		close(observations)
	}()

	found := make(map[string]*cameraNetworkEvidence)
	for obs := range observations {
		entry := found[obs.ip]
		if entry == nil {
			entry = &cameraNetworkEvidence{
				ip:         obs.ip,
				openPorts:  map[int]bool{},
				rtspPorts:  map[int]bool{},
				httpPorts:  map[int]bool{},
				httpsPorts: map[int]bool{},
			}
			found[obs.ip] = entry
		}
		entry.openPorts[obs.port] = true
		if obs.rtsp {
			entry.rtspPorts[obs.port] = true
		}
		if obs.http {
			entry.httpPorts[obs.port] = true
		}
		if obs.https {
			entry.httpsPorts[obs.port] = true
		}
		if obs.fingerprint != "" {
			entry.fingerprint.WriteString(obs.fingerprint)
			entry.fingerprint.WriteByte('\n')
		}
	}
	return found, nil
}

func probeRTSPConnection(conn net.Conn, ip string, port int) bool {
	_ = conn.SetDeadline(time.Now().Add(450 * time.Millisecond))
	target := fmt.Sprintf("rtsp://%s/", hostPortForDiscovery(ip, port, 554))
	_, _ = fmt.Fprintf(conn, "OPTIONS %s RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: Home-AI-Core discovery\r\n\r\n", target)
	reader := bufio.NewReader(io.LimitReader(conn, 4096))
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(line), "RTSP/")
}

func probeCameraHTTP(ctx context.Context, ip string, port int, secure bool) (bool, string) {
	scheme := "http"
	defaultPort := 80
	if secure {
		scheme = "https"
		defaultPort = 443
	}
	address := scheme + "://" + hostPortForDiscovery(ip, port, defaultPort) + "/"
	requestCtx, cancel := context.WithTimeout(ctx, 650*time.Millisecond)
	defer cancel()

	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: 250 * time.Millisecond,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // local appliance discovery; certificates are commonly self-signed
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, address, nil)
	if err != nil {
		return false, ""
	}
	req.Header.Set("User-Agent", "Home-AI-Core camera discovery")
	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 96<<10))
	fingerprint := strings.Join([]string{
		resp.Header.Get("Server"),
		resp.Header.Get("WWW-Authenticate"),
		resp.Header.Get("Location"),
		string(raw),
	}, "\n")
	return true, fingerprint
}

func hostPortForDiscovery(ip string, port, defaultPort int) string {
	if port == defaultPort {
		return ip
	}
	return net.JoinHostPort(ip, strconv.Itoa(port))
}

func readARPTable() map[string]string {
	raw, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ip := net.ParseIP(fields[0])
		mac, err := net.ParseMAC(fields[3])
		if ip == nil || err != nil {
			continue
		}
		result[ip.String()] = strings.ToUpper(mac.String())
	}
	return result
}
