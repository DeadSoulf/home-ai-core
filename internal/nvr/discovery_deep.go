package nvr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CameraDiscoveryInput struct {
	Mode string `json:"mode,omitempty"`
	CIDR string `json:"cidr,omitempty"`
}

var deepCameraSeedPorts = []int{
	80, 443, 554, 2020, 5000, 8000, 8080, 8554, 8899, 9000, 34567, 37777,
}

var deepCameraExtraPorts = []int{
	81, 88, 2000, 3000, 5001, 5555, 6000, 6666, 6789, 7001, 7070, 7447,
	8001, 8081, 8082, 8088, 8443, 8888, 9527, 9988, 10000, 10080, 10554,
	37778, 37779,
}

func (e *CameraDiscoveryEngine) DiscoverWithInput(
	ctx context.Context,
	input CameraDiscoveryInput,
) ([]CameraDiscoveryDevice, error) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" || mode == "quick" {
		return e.Discover(ctx)
	}
	if mode != "deep" {
		return nil, errors.New("camera discovery mode must be quick or deep")
	}
	return e.discoverDeep(ctx, strings.TrimSpace(input.CIDR))
}

func (e *CameraDiscoveryEngine) discoverDeep(
	ctx context.Context,
	cidr string,
) ([]CameraDiscoveryDevice, error) {
	targets, err := deepCameraTargets(cidr)
	if err != nil {
		return nil, err
	}

	discoverCtx, cancel := context.WithTimeout(ctx, 32*time.Second)
	defer cancel()

	var (
		quickDevices []CameraDiscoveryDevice
		network      map[string]*cameraNetworkEvidence
		hikvision    []CameraDiscoveryDevice
		dahua        []CameraDiscoveryDevice
		ssdp         []CameraDiscoveryDevice
		quickErr     error
		networkErr   error
		wg           sync.WaitGroup
	)

	wg.Add(5)
	go func() {
		defer wg.Done()
		quickDevices, quickErr = e.Discover(discoverCtx)
	}()
	go func() {
		defer wg.Done()
		network, networkErr = scanDeepCameraNetwork(discoverCtx, targets)
	}()
	go func() {
		defer wg.Done()
		hikvision = discoverHikvisionSADP(discoverCtx)
	}()
	go func() {
		defer wg.Done()
		dahua = discoverDahuaDHIP(discoverCtx)
	}()
	go func() {
		defer wg.Done()
		ssdp = discoverCameraSSDP(discoverCtx)
	}()
	wg.Wait()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	found := make(map[string]*CameraDiscoveryDevice)
	for _, devices := range [][]CameraDiscoveryDevice{quickDevices, hikvision, dahua, ssdp} {
		for _, device := range devices {
			mergeCameraDiscoveryDevice(found, device)
		}
	}
	for _, device := range cameraDevicesFromNetworkEvidence(network) {
		mergeCameraDiscoveryDevice(found, device)
	}

	arp := readARPTable()
	result := make([]CameraDiscoveryDevice, 0, len(found))
	for ip, device := range found {
		if device.MAC == "" {
			device.MAC = arp[ip]
		}
		classifyDiscoveryDevice(device)
		if !strongCameraDiscoveryEvidence(device) {
			continue
		}
		sort.Strings(device.Sources)
		sort.Slice(device.Services, func(i, j int) bool {
			if device.Services[i].Port == device.Services[j].Port {
				return device.Services[i].Protocol < device.Services[j].Protocol
			}
			return device.Services[i].Port < device.Services[j].Port
		})
		result = append(result, *device)
	}
	sort.Slice(result, func(i, j int) bool {
		return lessIPv4(result[i].IP, result[j].IP)
	})

	if len(result) == 0 && quickErr != nil && networkErr != nil {
		return nil, fmt.Errorf("deep camera discovery failed: quick: %v; network: %v", quickErr, networkErr)
	}
	return result, nil
}

func deepCameraTargets(cidr string) ([]net.IP, error) {
	if cidr != "" {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, errors.New("camera discovery CIDR is invalid")
		}
		base := network.IP.To4()
		if base == nil || (!base.IsPrivate() && !base.IsLinkLocalUnicast()) {
			return nil, errors.New("camera discovery CIDR must be a private IPv4 network")
		}
		ones, bits := network.Mask.Size()
		if bits != 32 || ones < 20 || ones > 30 {
			return nil, errors.New("camera discovery CIDR must be between /20 and /30")
		}
		return expandIPv4Network(network, 4096), nil
	}

	seen := make(map[string]bool)
	result := make([]net.IP, 0, 1024)
	add := func(ip net.IP) {
		ip = ip.To4()
		if ip == nil || (!ip.IsPrivate() && !ip.IsLinkLocalUnicast()) {
			return
		}
		key := ip.String()
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		result = append(result, append(net.IP(nil), ip...))
	}
	addNetwork := func(network *net.IPNet) {
		network = boundedAutoScanNetwork(network)
		if network == nil {
			return
		}
		for _, candidate := range expandIPv4Network(network, 4096) {
			add(candidate)
		}
	}

	interfaces, interfaceErr := net.Interfaces()
	if interfaceErr == nil {
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
				addNetwork(&net.IPNet{IP: ip, Mask: ipNet.Mask})
			}
		}
	}

	// Some appliance kernels expose IPv4 routing but reject the netlink
	// interface enumeration used by net.Interfaces(). /proc/net/route is a
	// stable fallback for connected IPv4 networks and avoids making Deep Scan
	// depend on that netlink family.
	if interfaceErr != nil || len(result) == 0 {
		if file, err := os.Open("/proc/net/route"); err == nil {
			for _, network := range parseProcNetRouteNetworks(file) {
				addNetwork(network)
			}
			_ = file.Close()
		}
	}

	// The ARP cache is the last fallback and also contributes networks that
	// have recently been reached through a route that was not enumerable.
	for ip := range readARPTable() {
		parsed := net.ParseIP(ip).To4()
		if parsed == nil || (!parsed.IsPrivate() && !parsed.IsLinkLocalUnicast()) {
			continue
		}
		addNetwork(&net.IPNet{IP: parsed, Mask: net.CIDRMask(24, 32)})
	}

	if len(result) == 0 {
		if interfaceErr != nil {
			return nil, fmt.Errorf(
				"camera discovery could not determine a private IPv4 network automatically; enter CIDR manually: %w",
				interfaceErr,
			)
		}
		return nil, errors.New("camera discovery could not determine a private IPv4 network automatically; enter CIDR manually")
	}
	if len(result) > 8192 {
		result = result[:8192]
	}
	return result, nil
}

func boundedAutoScanNetwork(network *net.IPNet) *net.IPNet {
	if network == nil {
		return nil
	}
	base := network.IP.To4()
	if base == nil || (!base.IsPrivate() && !base.IsLinkLocalUnicast()) {
		return nil
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones <= 0 || ones > 30 {
		return nil
	}
	if ones >= 20 {
		return &net.IPNet{IP: base.Mask(network.Mask), Mask: network.Mask}
	}

	// For broad routes such as /16, identify the actual local source address
	// selected by the kernel and bound the scan to the containing /20.
	if local := localIPv4ForNetwork(network); local != nil {
		mask := net.CIDRMask(20, 32)
		return &net.IPNet{IP: local.Mask(mask), Mask: mask}
	}
	return nil
}

func localIPv4ForNetwork(network *net.IPNet) net.IP {
	if network == nil {
		return nil
	}
	base := network.IP.To4()
	if base == nil {
		return nil
	}
	target := append(net.IP(nil), base...)
	target[3]++
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: target, Port: 9})
	if err != nil {
		return nil
	}
	defer conn.Close()
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || local.IP == nil {
		return nil
	}
	ip := local.IP.To4()
	if ip == nil || !network.Contains(ip) {
		return nil
	}
	return append(net.IP(nil), ip...)
}

func parseProcNetRouteNetworks(reader io.Reader) []*net.IPNet {
	scanner := bufio.NewScanner(reader)
	result := make([]*net.IPNet, 0, 8)
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 {
			continue
		}
		flags, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil || flags&0x1 == 0 {
			continue
		}
		destination := parseProcRouteIPv4(fields[1])
		maskIP := parseProcRouteIPv4(fields[7])
		if destination == nil || maskIP == nil {
			continue
		}
		mask := net.IPMask(maskIP.To4())
		ones, bits := mask.Size()
		if bits != 32 || ones <= 0 || ones > 30 {
			continue
		}
		networkIP := destination.Mask(mask)
		if networkIP == nil || (!networkIP.IsPrivate() && !networkIP.IsLinkLocalUnicast()) {
			continue
		}
		result = append(result, &net.IPNet{IP: networkIP, Mask: mask})
	}
	return result
}

func parseProcRouteIPv4(value string) net.IP {
	number, err := strconv.ParseUint(strings.TrimSpace(value), 16, 32)
	if err != nil {
		return nil
	}
	return net.IPv4(
		byte(number),
		byte(number>>8),
		byte(number>>16),
		byte(number>>24),
	).To4()
}

func expandIPv4Network(network *net.IPNet, limit int) []net.IP {
	if network == nil {
		return nil
	}
	base := network.IP.To4()
	if base == nil {
		return nil
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones < 0 {
		return nil
	}
	count := 1 << (32 - ones)
	if count > limit {
		count = limit
	}
	start := ipv4ToUint32(base.Mask(network.Mask))
	result := make([]net.IP, 0, count)
	for offset := 1; offset < count-1; offset++ {
		value := start + uint32(offset)
		result = append(result, uint32ToIPv4(value))
	}
	return result
}

func ipv4ToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIPv4(value uint32) net.IP {
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value)).To4()
}

func scanDeepCameraNetwork(
	ctx context.Context,
	targets []net.IP,
) (map[string]*cameraNetworkEvidence, error) {
	found := make(map[string]*cameraNetworkEvidence)
	if len(targets) == 0 {
		return found, nil
	}
	if err := scanCameraPorts(ctx, targets, deepCameraSeedPorts, found, 256); err != nil && ctx.Err() != nil {
		return found, err
	}

	active := make([]net.IP, 0, len(found))
	for ip := range found {
		active = append(active, net.ParseIP(ip))
	}
	if len(active) > 0 {
		_ = scanCameraPorts(ctx, active, deepCameraExtraPorts, found, 128)
	}
	return found, nil
}

func scanCameraPorts(
	ctx context.Context,
	targets []net.IP,
	ports []int,
	found map[string]*cameraNetworkEvidence,
	workers int,
) error {
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

	total := len(targets) * len(ports)
	if total == 0 {
		return nil
	}
	if workers > total {
		workers = total
	}

	tasks := make(chan task)
	observations := make(chan observation, 256)
	var wg sync.WaitGroup
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dialer := &net.Dialer{Timeout: 140 * time.Millisecond}
			for item := range tasks {
				if ctx.Err() != nil {
					return
				}
				ipText := item.ip.String()
				conn, err := dialer.DialContext(
					ctx,
					"tcp4",
					net.JoinHostPort(ipText, strconv.Itoa(item.port)),
				)
				if err != nil {
					continue
				}
				obs := observation{ip: ipText, port: item.port}
				switch {
				case isCameraRTSPPort(item.port):
					obs.rtsp = probeRTSPConnection(conn, ipText, item.port)
					_ = conn.Close()
				case isCameraHTTPSPort(item.port):
					_ = conn.Close()
					obs.https, obs.fingerprint = probeCameraHTTP(ctx, ipText, item.port, true)
				case isCameraHTTPPort(item.port):
					_ = conn.Close()
					obs.http, obs.fingerprint = probeCameraHTTP(ctx, ipText, item.port, false)
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
			if ip == nil {
				continue
			}
			for _, port := range ports {
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
	return ctx.Err()
}

func isCameraRTSPPort(port int) bool {
	switch port {
	case 554, 8554, 10554:
		return true
	default:
		return false
	}
}

func isCameraHTTPSPort(port int) bool {
	switch port {
	case 443, 8443:
		return true
	default:
		return false
	}
}

func isCameraHTTPPort(port int) bool {
	switch port {
	case 80, 81, 88, 2020, 8000, 8001, 8080, 8081, 8082, 8088, 8888, 8899,
		9000, 9527, 9988, 10000, 10080:
		return true
	default:
		return false
	}
}

func cameraDevicesFromNetworkEvidence(
	network map[string]*cameraNetworkEvidence,
) []CameraDiscoveryDevice {
	result := make([]CameraDiscoveryDevice, 0, len(network))
	for ip, evidence := range network {
		device := CameraDiscoveryDevice{
			Name: "Device " + ip,
			IP:   ip,
		}
		for port := range evidence.openPorts {
			addDiscoveryService(&device, "tcp", port, "")
		}
		for port := range evidence.httpPorts {
			address := fmt.Sprintf("http://%s", hostPortForDiscovery(ip, port, 80))
			addDiscoveryService(&device, "http", port, address)
			addDiscoverySource(&device, "http")
		}
		for port := range evidence.httpsPorts {
			address := fmt.Sprintf("https://%s", hostPortForDiscovery(ip, port, 443))
			addDiscoveryService(&device, "https", port, address)
			addDiscoverySource(&device, "https")
		}
		for port := range evidence.rtspPorts {
			address := fmt.Sprintf("rtsp://%s/", hostPortForDiscovery(ip, port, 554))
			addDiscoveryService(&device, "rtsp", port, address)
			if device.RTSPAddressHint == "" {
				device.RTSPAddressHint = address
			}
			addDiscoverySource(&device, "rtsp")
		}
		fingerprint := evidence.fingerprint.String()
		device.Vendor = detectCameraVendor(fingerprint)
		if device.Vendor != "" {
			addDiscoverySource(&device, "vendor:"+strings.ToLower(device.Vendor))
			device.Model = detectModelHint(fingerprint, device.Vendor)
		}
		if hasCameraLikePort(device.Services) || hasDeepCameraLikePort(device.Services) {
			addDiscoverySource(&device, "portscan")
		}
		classifyDiscoveryDevice(&device)
		result = append(result, device)
	}
	return result
}

func hasDeepCameraLikePort(services []CameraDiscoveryService) bool {
	for _, service := range services {
		switch service.Port {
		case 5000, 5555, 7070, 7447, 8001, 8081, 8088, 8443, 8888, 9000, 9527,
			9988, 10000, 10080, 10554, 37778, 37779:
			return true
		}
	}
	return false
}

func strongCameraDiscoveryEvidence(device *CameraDiscoveryDevice) bool {
	if device == nil {
		return false
	}
	if device.ONVIFAddress != "" || device.RTSPAddressHint != "" || device.Vendor != "" {
		return true
	}
	for _, source := range device.Sources {
		if strings.HasPrefix(source, "hikvision:") ||
			strings.HasPrefix(source, "dahua:") ||
			source == "onvif" ||
			source == "rtsp" ||
			source == "portscan" {
			return true
		}
	}
	return hasCameraLikePort(device.Services) || hasDeepCameraLikePort(device.Services)
}

func mergeCameraDiscoveryDevice(
	found map[string]*CameraDiscoveryDevice,
	incoming CameraDiscoveryDevice,
) {
	ip := strings.TrimSpace(incoming.IP)
	if ip == "" {
		return
	}
	current := found[ip]
	if current == nil {
		copy := incoming
		if copy.ID == "" {
			copy.ID = ensureDiscoveryDevice(map[string]*CameraDiscoveryDevice{}, ip).ID
		}
		found[ip] = &copy
		return
	}
	if current.Name == "" ||
		(strings.HasPrefix(current.Name, "Device ") && !strings.HasPrefix(incoming.Name, "Device ")) {
		current.Name = incoming.Name
	}
	if current.MAC == "" {
		current.MAC = incoming.MAC
	}
	if current.Vendor == "" {
		current.Vendor = incoming.Vendor
	}
	if current.Model == "" {
		current.Model = incoming.Model
	}
	if current.ONVIFAddress == "" {
		current.ONVIFAddress = incoming.ONVIFAddress
	}
	if current.RTSPAddressHint == "" {
		current.RTSPAddressHint = incoming.RTSPAddressHint
	}
	if current.SubstreamAddressHint == "" {
		current.SubstreamAddressHint = incoming.SubstreamAddressHint
	}
	for _, source := range incoming.Sources {
		addDiscoverySource(current, source)
	}
	for _, service := range incoming.Services {
		addDiscoveryService(current, service.Protocol, service.Port, service.Address)
	}
}

func lessIPv4(left, right string) bool {
	leftIP := net.ParseIP(left).To4()
	rightIP := net.ParseIP(right).To4()
	if leftIP == nil || rightIP == nil {
		return left < right
	}
	for index := 0; index < 4; index++ {
		if leftIP[index] != rightIP[index] {
			return leftIP[index] < rightIP[index]
		}
	}
	return false
}
