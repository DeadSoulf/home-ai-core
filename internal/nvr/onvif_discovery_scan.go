package nvr

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	onvifFallbackWorkers = 96
	onvifFallbackBodyMax = 128 << 10
)

var onvifFallbackPorts = []int{80, 8000, 8080, 8899, 2020}

type onvifScanEndpoint struct {
	ip   net.IP
	port int
}

func discoverONVIFHTTPFallback(ctx context.Context) []ONVIFDevice {
	targets := localONVIFScanTargets()
	if len(targets) == 0 {
		return nil
	}

	endpoints := make(chan onvifScanEndpoint)
	results := make(chan ONVIFDevice, 32)
	var wg sync.WaitGroup

	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   180 * time.Millisecond,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		MaxIdleConns:        onvifFallbackWorkers,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     10 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}

	worker := func() {
		defer wg.Done()
		for endpoint := range endpoints {
			if ctx.Err() != nil {
				return
			}
			if device, ok := probeONVIFHTTPEndpoint(ctx, client, endpoint); ok {
				select {
				case results <- device:
				case <-ctx.Done():
					return
				}
			}
		}
	}

	workers := onvifFallbackWorkers
	if len(targets)*len(onvifFallbackPorts) < workers {
		workers = len(targets) * len(onvifFallbackPorts)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}

	go func() {
		defer close(endpoints)
		for _, ip := range targets {
			for _, port := range onvifFallbackPorts {
				select {
				case endpoints <- onvifScanEndpoint{ip: ip, port: port}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	found := make(map[string]ONVIFDevice)
	for device := range results {
		if len(found) >= 128 {
			break
		}
		mergeDiscoveredONVIFDevice(found, device)
	}
	out := make([]ONVIFDevice, 0, len(found))
	for _, device := range found {
		out = append(out, device)
	}
	return out
}

func probeONVIFHTTPEndpoint(
	ctx context.Context,
	client *http.Client,
	endpoint onvifScanEndpoint,
) (ONVIFDevice, bool) {
	host := endpoint.ip.String()
	if host == "" {
		return ONVIFDevice{}, false
	}
	hostPort := host
	if endpoint.port != 80 {
		hostPort = net.JoinHostPort(host, strconv.Itoa(endpoint.port))
	}
	address := (&url.URL{
		Scheme: "http",
		Host:   hostPort,
		Path:   "/onvif/device_service",
	}).String()

	requestCtx, cancel := context.WithTimeout(ctx, 650*time.Millisecond)
	defer cancel()

	payload := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><tds:GetSystemDateAndTime xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>` +
		`</s:Body></s:Envelope>`
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		address,
		bytes.NewBufferString(payload),
	)
	if err != nil {
		return ONVIFDevice{}, false
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("Accept", "application/soap+xml, application/xml, text/xml")
	req.Header.Set("User-Agent", "Home-AI-Core ONVIF discovery")

	resp, err := client.Do(req)
	if err != nil {
		return ONVIFDevice{}, false
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, onvifFallbackBodyMax))
	if err != nil {
		return ONVIFDevice{}, false
	}
	if !looksLikeONVIFHTTPResponse(resp.StatusCode, raw) {
		return ONVIFDevice{}, false
	}

	return ONVIFDevice{
		ID:      discoveredONVIFID(address),
		Name:    "ONVIF " + host,
		Address: address,
		IP:      host,
	}, true
}

func looksLikeONVIFHTTPResponse(status int, raw []byte) bool {
	lower := strings.ToLower(string(raw))
	if strings.Contains(lower, "onvif.org/ver10") ||
		strings.Contains(lower, "getsystemdateandtimeresponse") ||
		strings.Contains(lower, "notauthorized") && strings.Contains(lower, "soap") {
		return true
	}
	return status >= 200 && status < 300 &&
		strings.Contains(lower, "envelope") &&
		strings.Contains(lower, "device")
}

func localONVIFScanTargets() []net.IP {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	result := make([]net.IP, 0, 256)
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ipNet, ok := address.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || (!ip.IsPrivate() && !ip.IsLinkLocalUnicast()) {
				continue
			}
			for _, candidate := range scanTargetsForIPv4(ip, ipNet.Mask) {
				key := candidate.String()
				if key == "" || key == ip.String() || seen[key] {
					continue
				}
				seen[key] = true
				result = append(result, candidate)
				if len(result) >= 512 {
					return result
				}
			}
		}
	}
	return result
}

func scanTargetsForIPv4(ip net.IP, mask net.IPMask) []net.IP {
	ip = ip.To4()
	if ip == nil {
		return nil
	}
	ones, bits := mask.Size()
	if bits != 32 || ones < 0 {
		return nil
	}

	// Large routed networks can contain tens of thousands of hosts. For
	// discovery fallback we intentionally bound probing to the local /24
	// containing this interface address. Normal WS-Discovery still covers the
	// broadcast domain without this cap.
	effectiveMask := mask
	if ones < 24 {
		effectiveMask = net.CIDRMask(24, 32)
		ones = 24
	}
	hostBits := 32 - ones
	hostCount := 1 << hostBits
	if hostCount <= 2 || hostCount > 256 {
		return nil
	}

	network := ip.Mask(effectiveMask)
	result := make([]net.IP, 0, hostCount-2)
	for host := 1; host < hostCount-1; host++ {
		candidate := append(net.IP(nil), network...)
		candidate[3] = byte(int(candidate[3]) + host)
		result = append(result, candidate)
	}
	return result
}
