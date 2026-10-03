package nvr

import (
	"bufio"
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

func discoverCameraSSDP(ctx context.Context) []CameraDiscoveryDevice {
	localIPs, err := localDiscoveryIPv4()
	localIPs = discoveryBindIPs(localIPs, err)

	query := []byte(
		"M-SEARCH * HTTP/1.1\r\n" +
			"HOST: 239.255.255.250:1900\r\n" +
			"MAN: \"ssdp:discover\"\r\n" +
			"MX: 1\r\n" +
			"ST: ssdp:all\r\n\r\n",
	)

	var wg sync.WaitGroup
	results := make(chan CameraDiscoveryDevice, 32)
	for _, localIP := range localIPs {
		localIP := append(net.IP(nil), localIP...)
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: localIP, Port: 0})
			if err != nil {
				return
			}
			defer conn.Close()

			target := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
			for round := 0; round < 2; round++ {
				_, _ = conn.WriteToUDP(query, target)
			}

			buffer := make([]byte, 64<<10)
			deadline := time.Now().Add(1800 * time.Millisecond)
			if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
				deadline = value
			}
			_ = conn.SetReadDeadline(deadline)
			for {
				n, sender, err := conn.ReadFromUDP(buffer)
				if err != nil {
					return
				}
				if sender == nil || sender.IP == nil || n <= 0 {
					continue
				}
				if device, ok := parseCameraSSDPResponse(buffer[:n], sender.IP); ok {
					select {
					case results <- device:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	found := make(map[string]*CameraDiscoveryDevice)
	for device := range results {
		mergeCameraDiscoveryDevice(found, device)
	}
	out := make([]CameraDiscoveryDevice, 0, len(found))
	for _, device := range found {
		out = append(out, *device)
	}
	return out
}

func parseCameraSSDPResponse(raw []byte, senderIP net.IP) (CameraDiscoveryDevice, bool) {
	if senderIP == nil || (!senderIP.IsPrivate() && !senderIP.IsLinkLocalUnicast()) {
		return CameraDiscoveryDevice{}, false
	}

	headers := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if first {
			first = false
			continue
		}
		if line == "" {
			break
		}
		index := strings.IndexByte(line, ':')
		if index <= 0 {
			continue
		}
		headers[strings.ToLower(strings.TrimSpace(line[:index]))] =
			strings.TrimSpace(line[index+1:])
	}

	fingerprint := strings.Join([]string{
		headers["server"],
		headers["st"],
		headers["usn"],
		headers["location"],
	}, " ")
	vendor := detectCameraVendor(fingerprint)
	lower := strings.ToLower(fingerprint)
	cameraLike := vendor != "" ||
		strings.Contains(lower, "camera") ||
		strings.Contains(lower, "onvif") ||
		strings.Contains(lower, "networkvideo") ||
		strings.Contains(lower, "ipcam") ||
		strings.Contains(lower, "nvr") ||
		strings.Contains(lower, "dvr")
	if !cameraLike {
		return CameraDiscoveryDevice{}, false
	}

	ipText := senderIP.String()
	name := strings.TrimSpace(headers["server"])
	if name == "" {
		name = "UPnP camera " + ipText
	}
	device := CameraDiscoveryDevice{
		Name:       name,
		IP:         ipText,
		Vendor:     vendor,
		DeviceType: "possible_camera",
		Confidence: "possible",
	}
	addDiscoverySource(&device, "ssdp")

	if location := strings.TrimSpace(headers["location"]); location != "" {
		if parsed, err := url.Parse(location); err == nil &&
			(parsed.Scheme == "http" || parsed.Scheme == "https") {
			hostIP := net.ParseIP(parsed.Hostname())
			if hostIP != nil && hostIP.Equal(senderIP) {
				port := 80
				if parsed.Scheme == "https" {
					port = 443
				}
				if parsed.Port() != "" {
					if value, err := strconv.Atoi(parsed.Port()); err == nil {
						port = value
					}
				}
				addDiscoveryService(&device, parsed.Scheme, port, parsed.String())
			}
		}
	}
	if vendor != "" {
		addDiscoverySource(&device, "vendor:"+strings.ToLower(vendor))
	}
	classifyDiscoveryDevice(&device)
	return device, true
}
