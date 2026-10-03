package nvr

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

func discoverHikvisionSADP(ctx context.Context) []CameraDiscoveryDevice {
	localIPs, err := localDiscoveryIPv4()
	localIPs = discoveryBindIPs(localIPs, err)

	probe := hikvisionSADPProbe()
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

			target := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 37020}
			for round := 0; round < 2; round++ {
				_, _ = conn.WriteToUDP(probe, target)
			}

			buffer := make([]byte, 64<<10)
			deadline := time.Now().Add(2200 * time.Millisecond)
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
				if device, ok := parseHikvisionSADPResponse(buffer[:n], sender.IP); ok {
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

func hikvisionSADPProbe() []byte {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	uuid := strings.ToUpper(hex.EncodeToString(raw))
	if len(uuid) == 32 {
		uuid = uuid[0:8] + "-" + uuid[8:12] + "-" + uuid[12:16] + "-" + uuid[16:20] + "-" + uuid[20:32]
	}
	return []byte(`<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>` +
		uuid + `</Uuid><Types>inquiry</Types></Probe>`)
}

func parseHikvisionSADPResponse(raw []byte, senderIP net.IP) (CameraDiscoveryDevice, bool) {
	fields := simpleXMLFields(raw)
	if len(fields) == 0 {
		return CameraDiscoveryDevice{}, false
	}
	types := strings.ToLower(firstField(fields, "Types", "Type"))
	if !strings.Contains(types, "inquiry") &&
		firstField(fields, "DeviceType", "DeviceDescription", "SerialNO", "SerialNo") == "" {
		return CameraDiscoveryDevice{}, false
	}

	ipText := firstField(fields, "IPv4Address", "IPAddress", "IP")
	if net.ParseIP(ipText) == nil {
		ipText = senderIP.String()
	}
	ip := net.ParseIP(ipText)
	if ip == nil || (!ip.IsPrivate() && !ip.IsLinkLocalUnicast()) {
		return CameraDiscoveryDevice{}, false
	}

	model := firstField(fields, "DeviceDescription", "DeviceType", "Model")
	name := strings.TrimSpace(model)
	if name == "" {
		name = "Hikvision " + ipText
	}
	device := CameraDiscoveryDevice{
		Name:       name,
		IP:         ipText,
		MAC:        normalizeDiscoveredMAC(firstField(fields, "MAC", "Mac")),
		Vendor:     "Hikvision",
		Model:      model,
		DeviceType: "possible_camera",
		Confidence: "medium",
	}
	addDiscoverySource(&device, "hikvision:sadp")

	httpPort := intField(fields, "HttpPort", "HTTPPort")
	if httpPort <= 0 {
		httpPort = 80
	}
	addDiscoveryService(
		&device,
		"http",
		httpPort,
		"http://"+hostPortForDiscovery(ipText, httpPort, 80),
	)
	if rtspPort := intField(fields, "RtspPort", "RTSPPort"); rtspPort > 0 {
		mainAddress := fmt.Sprintf(
			"rtsp://%s/Streaming/Channels/101",
			hostPortForDiscovery(ipText, rtspPort, 554),
		)
		subAddress := fmt.Sprintf(
			"rtsp://%s/Streaming/Channels/102",
			hostPortForDiscovery(ipText, rtspPort, 554),
		)
		addDiscoveryService(&device, "rtsp", rtspPort, mainAddress)
		device.RTSPAddressHint = mainAddress
		device.SubstreamAddressHint = subAddress
	}
	if commandPort := intField(fields, "CommandPort", "SDKPort"); commandPort > 0 {
		addDiscoveryService(&device, "hikvision-sdk", commandPort, "")
	}
	classifyDiscoveryDevice(&device)
	return device, true
}

func discoverDahuaDHIP(ctx context.Context) []CameraDiscoveryDevice {
	localIPs, err := localDiscoveryIPv4()
	localIPs = discoveryBindIPs(localIPs, err)
	probe := dahuaDHIPDiscoveryProbe()

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

			target := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 251), Port: 37810}
			for round := 0; round < 2; round++ {
				_, _ = conn.WriteToUDP(probe, target)
			}

			buffer := make([]byte, 64<<10)
			deadline := time.Now().Add(2200 * time.Millisecond)
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
				if device, ok := parseDahuaDHIPDiscovery(buffer[:n], sender.IP); ok {
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

func dahuaDHIPDiscoveryProbe() []byte {
	body, _ := json.Marshal(map[string]any{
		"method": "DHDiscover.search",
		"params": map[string]any{
			"mac": "",
			"uni": 1,
		},
	})
	header := make([]byte, 32)
	binary.LittleEndian.PutUint32(header[0:4], 32)
	binary.LittleEndian.PutUint32(header[4:8], 0x50494844)
	binary.LittleEndian.PutUint32(header[16:20], uint32(len(body)))
	binary.LittleEndian.PutUint32(header[24:28], uint32(len(body)))
	return append(header, body...)
}

func parseDahuaDHIPDiscovery(raw []byte, senderIP net.IP) (CameraDiscoveryDevice, bool) {
	body := raw
	if len(raw) >= 32 && binary.LittleEndian.Uint32(raw[4:8]) == 0x50494844 {
		body = raw[32:]
	}
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) != nil {
		return CameraDiscoveryDevice{}, false
	}
	info := mapValue(envelope, "params")
	if nested := mapValue(info, "deviceInfo"); len(nested) > 0 {
		info = nested
	}
	if len(info) == 0 {
		return CameraDiscoveryDevice{}, false
	}

	ipText := anyString(info, "IPv4Address", "IPAddress", "IpAddress", "ip", "IP")
	if net.ParseIP(ipText) == nil {
		ipText = senderIP.String()
	}
	ip := net.ParseIP(ipText)
	if ip == nil || (!ip.IsPrivate() && !ip.IsLinkLocalUnicast()) {
		return CameraDiscoveryDevice{}, false
	}

	model := anyString(info, "DeviceType", "DeviceClass", "Model", "MachineName")
	name := strings.TrimSpace(anyString(info, "DeviceName", "MachineName"))
	if name == "" {
		name = strings.TrimSpace(model)
	}
	if name == "" {
		name = "Dahua " + ipText
	}

	device := CameraDiscoveryDevice{
		Name:       name,
		IP:         ipText,
		MAC:        normalizeDiscoveredMAC(anyString(info, "Mac", "MAC", "mac")),
		Vendor:     "Dahua",
		Model:      model,
		DeviceType: "possible_camera",
		Confidence: "medium",
	}
	addDiscoverySource(&device, "dahua:dhip")

	if port := anyInt(info, "HttpPort", "HTTPPort"); port > 0 {
		addDiscoveryService(
			&device,
			"http",
			port,
			"http://"+hostPortForDiscovery(ipText, port, 80),
		)
	}
	if port := anyInt(info, "RtspPort", "RTSPPort"); port > 0 {
		address := fmt.Sprintf("rtsp://%s/", hostPortForDiscovery(ipText, port, 554))
		addDiscoveryService(&device, "rtsp", port, address)
		device.RTSPAddressHint = address
	}
	if port := anyInt(info, "Port", "TcpPort", "TCPPort"); port > 0 {
		addDiscoveryService(&device, "dahua-dhip", port, "")
	}
	classifyDiscoveryDevice(&device)
	return device, true
}

func simpleXMLFields(raw []byte) map[string]string {
	decoder := xml.NewDecoder(strings.NewReader(string(raw)))
	result := make(map[string]string)
	var current string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch value := token.(type) {
		case xml.StartElement:
			current = value.Name.Local
		case xml.CharData:
			text := strings.TrimSpace(string(value))
			if current != "" && text != "" {
				result[strings.ToLower(current)] = text
			}
		case xml.EndElement:
			if value.Name.Local == current {
				current = ""
			}
		}
	}
	return result
}

func firstField(fields map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(fields[strings.ToLower(name)]); value != "" {
			return value
		}
	}
	return ""
}

func intField(fields map[string]string, names ...string) int {
	value := firstField(fields, names...)
	number, _ := strconv.Atoi(value)
	return number
}

func mapValue(source map[string]any, key string) map[string]any {
	if source == nil {
		return nil
	}
	for current, value := range source {
		if !strings.EqualFold(current, key) {
			continue
		}
		if result, ok := value.(map[string]any); ok {
			return result
		}
	}
	return nil
}

func anyString(source map[string]any, names ...string) string {
	for _, name := range names {
		for key, value := range source {
			if !strings.EqualFold(key, name) {
				continue
			}
			switch typed := value.(type) {
			case string:
				return strings.TrimSpace(typed)
			case float64:
				return strconv.FormatInt(int64(typed), 10)
			case json.Number:
				return typed.String()
			}
		}
	}
	return ""
}

func anyInt(source map[string]any, names ...string) int {
	value := anyString(source, names...)
	number, _ := strconv.Atoi(value)
	return number
}

func normalizeDiscoveredMAC(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "-", ":")
	mac, err := net.ParseMAC(value)
	if err != nil {
		return strings.ToUpper(value)
	}
	return strings.ToUpper(mac.String())
}
