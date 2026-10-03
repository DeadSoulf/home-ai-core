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
	return &WSDiscovery{timeout: 2500 * time.Millisecond}
}

func (d *WSDiscovery) Discover(ctx context.Context) ([]ONVIFDevice, error) {
	timeout := d.timeout
	if timeout <= 0 {
		timeout = 2500 * time.Millisecond
	}
	discoverCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	localIPs, err := localDiscoveryIPv4()
	localIPs = discoveryBindIPs(localIPs, err)

	probe, err := wsDiscoveryProbe()
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	found := make(map[string]ONVIFDevice)
	for _, localIP := range localIPs {
		localIP := append(net.IP(nil), localIP...)
		wg.Add(1)
		go func() {
			defer wg.Done()
			devices := discoverONVIFOnIP(discoverCtx, localIP, probe)
			mu.Lock()
			for _, device := range devices {
				if len(found) >= 64 {
					break
				}
				found[device.Address] = device
			}
			mu.Unlock()
		}()
	}
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
	message := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"` +
		` xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing"` +
		` xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"` +
		` xmlns:dn="http://www.onvif.org/ver10/network/wsdl">` +
		`<e:Header><w:MessageID>` + id + `</w:MessageID>` +
		`<w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>` +
		`<w:Action e:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>` +
		`</e:Header><e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></e:Body></e:Envelope>`
	return []byte(message), nil
}

func discoverONVIFOnIP(ctx context.Context, localIP net.IP, probe []byte) []ONVIFDevice {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: localIP, Port: 0})
	if err != nil {
		return nil
	}
	defer conn.Close()

	target := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 3702}
	if _, err := conn.WriteToUDP(probe, target); err != nil {
		return nil
	}

	result := make([]ONVIFDevice, 0, 4)
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
		devices := parseWSDiscoveryResponse(buffer[:n], sender.IP)
		result = append(result, devices...)
		if len(result) >= 64 {
			return result[:64]
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
