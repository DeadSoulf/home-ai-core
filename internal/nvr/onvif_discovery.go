package nvr

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrONVIFDiscoveryUnavailable = errors.New("ONVIF discovery is unavailable")
	ErrONVIFInvalidEndpoint      = errors.New("ONVIF endpoint is invalid")
	ErrONVIFAuthentication       = errors.New("ONVIF authentication failed")
	ErrONVIFConnection           = errors.New("ONVIF connection failed")
	ErrONVIFNoMediaProfiles      = errors.New("ONVIF device returned no media profiles")
)

type ONVIFDevice struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Address string   `json:"address"`
	IP      string   `json:"ip"`
	Scopes  []string `json:"scopes,omitempty"`
}

type ONVIFDiscoverer interface {
	Discover(context.Context) ([]ONVIFDevice, error)
}

type WSDiscovery struct {
	timeout time.Duration
}

func NewWSDiscovery() *WSDiscovery {
	return &WSDiscovery{timeout: 5 * time.Second}
}

func (d *WSDiscovery) Discover(ctx context.Context) ([]ONVIFDevice, error) {
	timeout := d.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	discoverCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	localIPs, err := localDiscoveryIPv4()
	localIPs = discoveryBindIPs(localIPs, err)

	probes, err := wsDiscoveryProbes()
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	found := make(map[string]ONVIFDevice)
	addDevices := func(devices []ONVIFDevice) {
		mu.Lock()
		defer mu.Unlock()
		for _, device := range devices {
			if len(found) >= 128 {
				break
			}
			mergeDiscoveredONVIFDevice(found, device)
		}
	}

	for _, localIP := range localIPs {
		localIP := append(net.IP(nil), localIP...)
		wg.Add(1)
		go func() {
			defer wg.Done()
			addDevices(discoverONVIFOnIP(discoverCtx, localIP, probes))
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		addDevices(discoverONVIFHTTPFallback(discoverCtx))
	}()

	wg.Wait()

	result := make([]ONVIFDevice, 0, len(found))
	for _, device := range found {
		result = append(result, device)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].IP < result[j].IP
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func mergeDiscoveredONVIFDevice(found map[string]ONVIFDevice, device ONVIFDevice) {
	if strings.TrimSpace(device.Address) == "" {
		return
	}
	key := strings.TrimSpace(device.IP)
	if key == "" {
		key = strings.TrimSpace(device.Address)
	}
	existing, ok := found[key]
	if !ok {
		found[key] = device
		return
	}
	if strings.HasPrefix(existing.Name, "ONVIF ") && !strings.HasPrefix(device.Name, "ONVIF ") {
		existing.Name = device.Name
	}
	if len(device.Scopes) > len(existing.Scopes) {
		existing.Scopes = append([]string(nil), device.Scopes...)
	}
	if strings.Contains(strings.ToLower(device.Address), "/onvif/") &&
		!strings.Contains(strings.ToLower(existing.Address), "/onvif/") {
		existing.Address = device.Address
	}
	found[key] = existing
}

func discoveryBindIPs(localIPs []net.IP, discoveryErr error) []net.IP {
	if discoveryErr == nil && len(localIPs) > 0 {
		return localIPs
	}
	return []net.IP{append(net.IP(nil), net.IPv4zero...)}
}

func localDiscoveryIPv4() ([]net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("%w: list interfaces", ErrONVIFDiscoveryUnavailable)
	}
	result := make([]net.IP, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() {
				continue
			}
			result = append(result, append(net.IP(nil), ip...))
		}
	}
	return result, nil
}

func wsDiscoveryProbe() ([]byte, error) {
	return buildWSDiscoveryProbe(
		"http://schemas.xmlsoap.org/ws/2004/08/addressing",
		"http://schemas.xmlsoap.org/ws/2005/04/discovery",
		"urn:schemas-xmlsoap-org:ws:2005:04:discovery",
		"http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe",
		"dn:NetworkVideoTransmitter",
	)
}

func wsDiscoveryProbes() ([][]byte, error) {
	type variant struct {
		addressingNS string
		discoveryNS  string
		to           string
		action       string
		types        string
	}
	versions := []variant{
		{
			addressingNS: "http://schemas.xmlsoap.org/ws/2004/08/addressing",
			discoveryNS:  "http://schemas.xmlsoap.org/ws/2005/04/discovery",
			to:           "urn:schemas-xmlsoap-org:ws:2005:04:discovery",
			action:       "http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe",
		},
		{
			addressingNS: "http://www.w3.org/2005/08/addressing",
			discoveryNS:  "http://docs.oasis-open.org/ws-dd/ns/discovery/2009/01",
			to:           "urn:docs-oasis-open-org:ws-dd:ns:discovery:2009:01",
			action:       "http://docs.oasis-open.org/ws-dd/ns/discovery/2009/01/Probe",
		},
	}
	typeFilters := []string{"dn:NetworkVideoTransmitter", "tds:Device", ""}
	result := make([][]byte, 0, len(versions)*len(typeFilters))
	for _, version := range versions {
		for _, types := range typeFilters {
			probe, err := buildWSDiscoveryProbe(
				version.addressingNS,
				version.discoveryNS,
				version.to,
				version.action,
				types,
			)
			if err != nil {
				return nil, err
			}
			result = append(result, probe)
		}
	}
	return result, nil
}

func buildWSDiscoveryProbe(addressingNS, discoveryNS, to, action, types string) ([]byte, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate ONVIF discovery id: %w", err)
	}
	id := fmt.Sprintf(
		"urn:uuid:%x-%x-%x-%x-%x",
		raw[0:4],
		raw[4:6],
		raw[6:8],
		raw[8:10],
		raw[10:16],
	)
	typeElement := ""
	if types != "" {
		typeElement = "<d:Types>" + types + "</d:Types>"
	}
	message := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"` +
		` xmlns:w="` + addressingNS + `"` +
		` xmlns:d="` + discoveryNS + `"` +
		` xmlns:dn="http://www.onvif.org/ver10/network/wsdl"` +
		` xmlns:tds="http://www.onvif.org/ver10/device/wsdl">` +
		`<e:Header><w:MessageID>` + id + `</w:MessageID>` +
		`<w:To e:mustUnderstand="true">` + to + `</w:To>` +
		`<w:Action e:mustUnderstand="true">` + action + `</w:Action>` +
		`</e:Header><e:Body><d:Probe>` + typeElement + `</d:Probe></e:Body></e:Envelope>`
	return []byte(message), nil
}

func discoverONVIFOnIP(ctx context.Context, localIP net.IP, probes [][]byte) []ONVIFDevice {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: localIP, Port: 0})
	if err != nil {
		return nil
	}
	defer conn.Close()

	target := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 3702}
	sent := false
	for round := 0; round < 2; round++ {
		for _, probe := range probes {
			if _, err := conn.WriteToUDP(probe, target); err == nil {
				sent = true
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	if !sent {
		return nil
	}

	result := make([]ONVIFDevice, 0, 8)
	buffer := make([]byte, 64<<10)
	for {
		deadline := time.Now().Add(250 * time.Millisecond)
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
		_ = conn.SetReadDeadline(deadline)
		n, sender, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return result
			}
			if networkErr, ok := err.(net.Error); ok && networkErr.Timeout() {
				select {
				case <-ctx.Done():
					return result
				default:
					continue
				}
			}
			return result
		}
		if sender == nil || sender.IP == nil || n <= 0 {
			continue
		}
		result = append(result, parseWSDiscoveryResponse(buffer[:n], sender.IP)...)
		if len(result) >= 128 {
			return result[:128]
		}
	}
}

type wsDiscoveryEnvelope struct {
	Body struct {
		ProbeMatches struct {
			Matches []struct {
				EndpointReference struct {
					Address string `xml:"Address"`
				} `xml:"EndpointReference"`
				Scopes string `xml:"Scopes"`
				XAddrs string `xml:"XAddrs"`
			} `xml:"ProbeMatch"`
		} `xml:"ProbeMatches"`
	} `xml:"Body"`
}

func parseWSDiscoveryResponse(raw []byte, senderIP net.IP) []ONVIFDevice {
	var envelope wsDiscoveryEnvelope
	if len(raw) == 0 || senderIP == nil || xml.Unmarshal(raw, &envelope) != nil {
		return nil
	}
	result := make([]ONVIFDevice, 0, len(envelope.Body.ProbeMatches.Matches))
	for _, match := range envelope.Body.ProbeMatches.Matches {
		scopes := strings.Fields(strings.TrimSpace(match.Scopes))
		if !looksLikeONVIFDiscoveryMatch(scopes, match.XAddrs) {
			continue
		}
		for _, candidate := range strings.Fields(strings.TrimSpace(match.XAddrs)) {
			address, err := normalizeDiscoveredONVIFAddress(candidate, senderIP)
			if err != nil {
				continue
			}
			ipText := senderIP.String()
			sum := sha256.Sum256([]byte(address))
			device := ONVIFDevice{
				ID:      "onvif_" + hex.EncodeToString(sum[:8]),
				Name:    onvifNameFromScopes(scopes, ipText),
				Address: address,
				IP:      ipText,
				Scopes:  append([]string(nil), scopes...),
			}
			result = append(result, device)
			break
		}
	}
	return result
}

func looksLikeONVIFDiscoveryMatch(scopes []string, xaddrs string) bool {
	for _, scope := range scopes {
		lower := strings.ToLower(scope)
		if strings.Contains(lower, "onvif.org") || strings.Contains(lower, "networkvideotransmitter") {
			return true
		}
	}
	for _, candidate := range strings.Fields(strings.TrimSpace(xaddrs)) {
		lower := strings.ToLower(candidate)
		if strings.Contains(lower, "/onvif/") || strings.Contains(lower, "onvif") {
			return true
		}
	}
	return false
}

func normalizeDiscoveredONVIFAddress(value string, senderIP net.IP) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return "", ErrONVIFInvalidEndpoint
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrONVIFInvalidEndpoint
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", ErrONVIFInvalidEndpoint
	}
	port := parsed.Port()
	if port != "" {
		parsed.Host = net.JoinHostPort(senderIP.String(), port)
	} else {
		parsed.Host = senderIP.String()
	}
	return parsed.String(), nil
}

func onvifNameFromScopes(scopes []string, fallbackIP string) string {
	for _, scope := range scopes {
		parsed, err := url.Parse(scope)
		if err != nil {
			continue
		}
		const marker = "/name/"
		index := strings.Index(parsed.Path, marker)
		if index < 0 {
			continue
		}
		name, err := url.PathUnescape(strings.Trim(parsed.Path[index+len(marker):], "/"))
		if err == nil && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return "ONVIF " + fallbackIP
}

func validateLocalONVIFEndpoint(value string) (*url.URL, net.IP, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return nil, nil, ErrONVIFInvalidEndpoint
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, nil, ErrONVIFInvalidEndpoint
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, nil, ErrONVIFInvalidEndpoint
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, nil, ErrONVIFInvalidEndpoint
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() {
		return nil, nil, ErrONVIFInvalidEndpoint
	}
	if !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
		return nil, nil, ErrONVIFInvalidEndpoint
	}
	return parsed, ip, nil
}

func normalizeONVIFServiceAddress(value string, expectedIP net.IP) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || expectedIP == nil {
		return "", ErrONVIFInvalidEndpoint
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrONVIFInvalidEndpoint
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", ErrONVIFInvalidEndpoint
	}
	port := parsed.Port()
	if port != "" {
		parsed.Host = net.JoinHostPort(expectedIP.String(), port)
	} else {
		parsed.Host = expectedIP.String()
	}
	return parsed.String(), nil
}
