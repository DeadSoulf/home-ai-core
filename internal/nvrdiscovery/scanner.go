package nvrdiscovery

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Device struct {
	Address    string   `json:"address"`
	Vendor     string   `json:"vendor"`
	Name       string   `json:"name,omitempty"`
	Services   []string `json:"services"`
	Confidence string   `json:"confidence"`
	RTSPPort   int      `json:"rtsp_port,omitempty"`
	HTTPPort   int      `json:"http_port,omitempty"`
	SDKPort    int      `json:"sdk_port,omitempty"`
}

type signal struct {
	IP        string
	Vendor    string
	Name      string
	Service   string
	Port      int
	Confirmed bool
}

var vendorKeywords = []struct {
	keyword string
	vendor  string
}{
	{"hikvision", "Hikvision"}, {"hiwatch", "Hikvision"}, {"ezviz", "EZVIZ"},
	{"dahua", "Dahua"}, {"amcrest", "Amcrest"}, {"imou", "Imou"},
	{"reolink", "Reolink"}, {"axis", "Axis"}, {"uniview", "Uniview"},
	{"unv ", "Uniview"}, {"wisenet", "Hanwha Wisenet"}, {"hanwha", "Hanwha Wisenet"},
	{"mobotix", "Mobotix"}, {"bosch", "Bosch"}, {"panasonic", "Panasonic"},
	{"i-pro", "i-PRO"}, {"foscam", "Foscam"}, {"vigi", "TP-Link VIGI"},
	{"tp-link", "TP-Link"}, {"tapo", "TP-Link Tapo"}, {"ubiquiti", "Ubiquiti"},
	{"unifi", "Ubiquiti"}, {"sony", "Sony"}, {"annke", "ANNKE"},
	{"xiongmai", "Xiongmai"}, {"xmeye", "Xiongmai"},
	{"intelbras", "Intelbras"}, {"vivotek", "Vivotek"}, {"avigilon", "Avigilon"},
	{"ubnt", "Ubiquiti"}, {"geovision", "GeoVision"},
}

func vendorFromText(value string) string {
	value = strings.ToLower(value)
	for _, keyword := range vendorKeywords {
		if strings.Contains(value, keyword.keyword) {
			return keyword.vendor
		}
	}
	return ""
}

// Scan searches only the selected directly connected /24-or-smaller network.
// UDP announcements, RTSP, vendor-specific TCP ports and web fingerprints
// complement ONVIF; no passwords, login attempts, or stream URL guesses.
func Scan(ctx context.Context, selected Network) ([]Device, error) {
	hosts, err := hostsForNetwork(selected)
	if err != nil {
		return nil, err
	}
	prefix, err := netip.ParsePrefix(selected.CIDR)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	results := make(chan signal, 4096)
	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); scanTCP(ctx, hosts, results) }()
	go func() { defer wg.Done(); listenWSDiscovery(ctx, selected, prefix, results) }()
	go func() { defer wg.Done(); listenSSDP(ctx, selected, prefix, results) }()
	go func() { defer wg.Done(); listenMDNS(ctx, selected, prefix, results) }()
	go func() { wg.Wait(); close(results) }()
	devices := make(map[string]*Device)
	signals := make(map[string]map[string]bool)
	for item := range results {
		addr, err := netip.ParseAddr(item.IP)
		if err != nil || !prefix.Contains(addr) || !addr.Is4() || item.IP == selected.Address {
			continue
		}
		device := devices[item.IP]
		if device == nil {
			device = &Device{Address: item.IP, Vendor: "Не определён", Confidence: "possible"}
			devices[item.IP] = device
		}
		if item.Vendor != "" && device.Vendor == "Не определён" {
			device.Vendor = safeLabel(item.Vendor, 72)
		}
		if item.Name != "" && device.Name == "" {
			device.Name = safeLabel(item.Name, 96)
		}
		if item.Confirmed {
			device.Confidence = "detected"
		}
		if signals[item.IP] == nil {
			signals[item.IP] = make(map[string]bool)
		}
		if item.Service != "" && !signals[item.IP][item.Service] {
			signals[item.IP][item.Service] = true
			device.Services = append(device.Services, item.Service)
		}
		switch {
		case item.Service == "RTSP" && item.Port > 0:
			if device.RTSPPort == 0 || item.Port == 554 {
				device.RTSPPort = item.Port
			}
		case (item.Service == "HTTP" || item.Service == "HTTPS") && item.Port > 0:
			device.HTTPPort = item.Port
		case strings.HasPrefix(item.Service, "SDK") && item.Port > 0:
			device.SDKPort = item.Port
		}
	}
	out := make([]Device, 0, len(devices))
	for _, device := range devices {
		// SDK-only results are marked 'possible'; routers with HTTP ports
		// alone never enter this list unless a camera-specific banner matched.
		sort.Strings(device.Services)
		out = append(out, *device)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence == "detected"
		}
		a, _ := netip.ParseAddr(out[i].Address)
		b, _ := netip.ParseAddr(out[j].Address)
		return a.Less(b)
	})
	return out, nil
}

func safeLabel(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > maxLen {
		value = value[:maxLen]
	}
	return value
}

// Limit simultaneous connections and total duration; never brute-force or
// try candidate credentials against local services.
func scanTCP(ctx context.Context, hosts []string, results chan<- signal) {
	queue := make(chan string)
	var workers sync.WaitGroup
	for i := 0; i < 64; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for host := range queue {
				probeHost(ctx, host, results)
			}
		}()
	}
sendHosts:
	for _, host := range hosts {
		select {
		case <-ctx.Done():
			break sendHosts
		case queue <- host:
		}
	}
	close(queue)
	workers.Wait()
}

func probeHost(ctx context.Context, host string, results chan<- signal) {
	for _, port := range []int{554, 8554, 8000, 37777, 34567, 80, 443} {
		if ctx.Err() != nil {
			return
		}
		address := net.JoinHostPort(host, strconv.Itoa(port))
		conn, err := (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext(ctx, "tcp4", address)
		if err != nil {
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(350 * time.Millisecond))
		switch port {
		case 554, 8554:
			result := probeRTSP(conn, host, port)
			result.IP = host
			emit(ctx, results, result)
		case 8000:
			emit(ctx, results, signal{IP: host, Service: "SDK 8000", Port: port})
		case 37777:
			emit(ctx, results, signal{IP: host, Service: "SDK 37777 (Dahua-совместимый)", Port: port})
		case 34567:
			emit(ctx, results, signal{IP: host, Service: "SDK 34567 (XM-совместимый)", Port: port})
		case 80:
			result := probeHTTP(conn, host, 80)
			if result.Service != "" {
				result.IP = host
				emit(ctx, results, result)
			}
		case 443:
			// A local camera can have a self-signed certificate. This
			// handshake only fingerprints an anonymous service; it never
			// transmits credentials or treats TLS as authenticated.
			tlsConn := tls.Client(conn, &tls.Config{
				InsecureSkipVerify: true, // probe-only; never for login, video or credentials
				MinVersion: tls.VersionTLS12,
			})
			if tlsConn.HandshakeContext(ctx) == nil {
				result := probeHTTP(tlsConn, host, 443)
				if result.Service != "" {
					result.IP = host
					emit(ctx, results, result)
				}
			}
		}
		_ = conn.Close()
	}
}

func emit(ctx context.Context, channel chan<- signal, value signal) {
	if value.Service == "" || value.IP == "" {
		return
	}
	select {
	case channel <- value:
	case <-ctx.Done():
	}
}

func probeRTSP(conn net.Conn, host string, port int) signal {
	_, err := fmt.Fprintf(conn, "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: HOME-AI-Core-Discovery\r\n\r\n")
	if err != nil {
		return signal{Service: "RTSP", Port: port}
	}
	reader := bufio.NewReader(io.LimitReader(conn, 1400))
	line, _ := reader.ReadString('\n')
	if !strings.HasPrefix(line, "RTSP/1.0 ") && !strings.HasPrefix(line, "RTSP/2.0 ") {
		// A TCP port alone is not conclusive proof of an RTSP camera.
		return signal{Service: "RTSP", Port: port}
	}
	header := line
	for i := 0; i < 16; i++ {
		line, err = reader.ReadString('\n')
		header += line
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}
	return signal{Service: "RTSP", Port: port, Vendor: vendorFromText(header), Confirmed: true}
}

func probeHTTP(conn net.Conn, host string, port int) signal {
	if _, err := fmt.Fprintf(conn, "HEAD / HTTP/1.1\r\nHost: %s\r\nUser-Agent: HOME-AI-Core-Discovery\r\nConnection: close\r\n\r\n", host); err != nil {
		return signal{}
	}
	// HEAD endpoints are allowed to keep their TCP connections alive. Read
	// response headers, not the whole response body or until connection close.
	reader := bufio.NewReader(io.LimitReader(conn, 1400))
	first, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(first, "HTTP/") {
		return signal{}
	}
	banner := first
	for i := 0; i < 24; i++ {
		line, err := reader.ReadString('\n')
		banner += line
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}
	vendor := vendorFromText(banner)
	cameraWords := strings.Contains(strings.ToLower(banner), "ip camera") ||
		strings.Contains(strings.ToLower(banner), "network camera") ||
		strings.Contains(strings.ToLower(banner), "onvif")
	if vendor == "" && !cameraWords {
		return signal{}
	}
	service := "HTTP"
	if port == 443 {
		service = "HTTPS"
	}
	return signal{Service: service, Port: port, Vendor: vendor, Confirmed: vendor != "" || cameraWords}
}
