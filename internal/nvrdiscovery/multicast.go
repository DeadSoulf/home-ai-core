package nvrdiscovery

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	wsDiscoveryMulticast = "239.255.255.250:3702"
	ssdpMulticast = "239.255.255.250:1900"
	mdnsMulticast = "224.0.0.251:5353"
)

// Multicast queries originate from the selected LAN interface address. The
// replies are never trusted as URL targets; only the source IP is considered,
// and it must belong to the currently authorized local network.
func listenUDP(ctx context.Context, selected Network, prefix netip.Prefix, destination string,
	payload []byte, parse func([]byte) signal, output chan<- signal) {
	local := net.ParseIP(selected.Address)
	if local == nil {
		return
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: local, Port: 0})
	if err != nil {
		return
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	dest, err := net.ResolveUDPAddr("udp4", destination)
	if err != nil {
		return
	}
	if _, err = conn.WriteToUDP(payload, dest); err != nil {
		return
	}
	deadline := time.Now().Add(1900 * time.Millisecond)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetReadDeadline(deadline)
	buffer := make([]byte, 8192)
	for {
		n, remote, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		addr, ok := netip.AddrFromSlice(remote.IP.To4())
		if !ok || !prefix.Contains(addr) || addr.String() == selected.Address {
			continue
		}
		item := parse(buffer[:n])
		if item.Service == "" {
			continue
		}
		item.IP = addr.String()
		emit(ctx, output, item)
	}
}

func listenWSDiscovery(ctx context.Context, selected Network, prefix netip.Prefix, output chan<- signal) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return
	}
	messageID := fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	probe := `<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"
 xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing"
 xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
 xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
<e:Header><w:MessageID>` + messageID + `</w:MessageID>
<w:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>
<w:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>
</e:Header><e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></e:Body>
</e:Envelope>`
	listenUDP(ctx, selected, prefix, wsDiscoveryMulticast, []byte(probe), func(data []byte) signal {
		return parseWSDiscovery(data, messageID)
	}, output)
}

func parseWSDiscovery(data []byte, sentID string) signal {
	if len(data) > 8192 || !bytes.Contains(data, []byte("ProbeMatch")) {
		return signal{}
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var matched, relate, scopes string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		tag, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if tag.Name.Local == "ProbeMatch" || tag.Name.Local == "ProbeMatches" {
			matched = "yes"
		}
		if tag.Name.Local == "RelatesTo" {
			_ = decoder.DecodeElement(&relate, &tag)
		}
		if tag.Name.Local == "Scopes" {
			_ = decoder.DecodeElement(&scopes, &tag)
		}
	}
	if matched == "" || !strings.EqualFold(strings.TrimSpace(relate), sentID) {
		return signal{}
	}
	var name string
	for _, scope := range strings.Fields(scopes) {
		index := strings.Index(strings.ToLower(scope), "/name/")
		if index < 0 {
			continue
		}
		value, err := url.PathUnescape(scope[index+len("/name/"):])
		if err == nil && value != "" {
			name = safeLabel(value, 96)
			break
		}
	}
	return signal{Service: "ONVIF WS-Discovery", Vendor: vendorFromText(scopes), Name: name, Confirmed: true}
}

func listenSSDP(ctx context.Context, selected Network, prefix netip.Prefix, output chan<- signal) {
	const request = "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: ssdp:all\r\n\r\n"
	listenUDP(ctx, selected, prefix, ssdpMulticast, []byte(request), parseSSDPResponse, output)
}

func parseSSDPResponse(packet []byte) signal {
	text := strings.ToLower(string(packet))
	if len(packet) > 8192 || !strings.HasPrefix(text, "http/1.") ||
		!strings.Contains(strings.SplitN(text, "\r\n", 2)[0], "200") {
		return signal{}
	}
	vendor := vendorFromText(text)
	isCamera := strings.Contains(text, "ipcamera") || strings.Contains(text, "ip camera") ||
		strings.Contains(text, "networkcamera") || strings.Contains(text, "network camera") ||
		strings.Contains(text, "onvif") || strings.Contains(text, "digital-security-camera")
	if vendor == "" && !isCamera {
		return signal{}
	}
	return signal{Service: "SSDP/UPnP", Vendor: vendor, Confirmed: isCamera}
}

func listenMDNS(ctx context.Context, selected Network, prefix netip.Prefix, output chan<- signal) {
	payload := buildMDNSQuestion([]string{"_rtsp._tcp.local", "_onvif._tcp.local"})
	listenUDP(ctx, selected, prefix, mdnsMulticast, payload, parseMDNSResponse, output)
}

func buildMDNSQuestion(names []string) []byte {
	out := make([]byte, 12)
	binary.BigEndian.PutUint16(out[4:6], uint16(len(names)))
	for _, name := range names {
		for _, label := range strings.Split(name, ".") {
			if len(label) == 0 || len(label) > 63 {
				continue
			}
			out = append(out, byte(len(label)))
			out = append(out, []byte(label)...)
		}
		// QTYPE PTR; QCLASS IN with the QU bit set (unicast reply).
		out = append(out, 0, 0, 12, 0x80, 1)
	}
	return out
}

// RFC 1035 name decoding with pointer compression, bounded hops and sizes.
func dnsName(data []byte, offset int) (string, int, error) {
	start := offset
	next := -1
	parts := make([]string, 0, 8)
	visited := make(map[int]bool)
	for hops := 0; hops < 64; hops++ {
		if offset >= len(data) || offset < 0 {
			return "", 0, errors.New("invalid DNS offset")
		}
		if visited[offset] {
			return "", 0, errors.New("DNS pointer cycle")
		}
		visited[offset] = true
		size := int(data[offset])
		if size&0xc0 == 0xc0 {
			if offset+1 >= len(data) {
				return "", 0, errors.New("truncated DNS pointer")
			}
			target := (size&0x3f)<<8 | int(data[offset+1])
			if next == -1 {
				next = offset+2
			}
			offset = target
			continue
		}
		if size&0xc0 != 0 || size > 63 {
			return "", 0, errors.New("bad DNS label")
		}
		offset++
		if size == 0 {
			if next == -1 {
				next = offset
			}
			return strings.Join(parts, "."), next, nil
		}
		if offset+size > len(data) {
			return "", 0, errors.New("truncated DNS label")
		}
		parts = append(parts, string(data[offset:offset+size]))
		if len(parts) > 20 {
			return "", 0, errors.New("too many DNS labels")
		}
		offset += size
	}
	return "", start, errors.New("DNS name compression overflow")
}

func parseMDNSResponse(packet []byte) signal {
	if len(packet) < 12 || len(packet) > 8192 || packet[2]&0x80 == 0 {
		return signal{}
	}
	questions := int(binary.BigEndian.Uint16(packet[4:6]))
	records := int(binary.BigEndian.Uint16(packet[6:8])) +
		int(binary.BigEndian.Uint16(packet[8:10])) +
		int(binary.BigEndian.Uint16(packet[10:12]))
	if questions > 16 || records > 100 {
		return signal{}
	}
	offset := 12
	for i := 0; i < questions; i++ {
		_, next, err := dnsName(packet, offset)
		if err != nil || next+4 > len(packet) {
			return signal{}
		}
		offset = next+4
	}
	for i := 0; i < records; i++ {
		name, next, err := dnsName(packet, offset)
		if err != nil || next+10 > len(packet) {
			return signal{}
		}
		recordType := binary.BigEndian.Uint16(packet[next : next+2])
		length := int(binary.BigEndian.Uint16(packet[next+8 : next+10]))
		start := next+10
		offset = start+length
		if offset > len(packet) {
			return signal{}
		}
		lower := strings.ToLower(name)
		isService := strings.HasSuffix(lower, "._rtsp._tcp.local") ||
			strings.HasSuffix(lower, "._onvif._tcp.local") ||
			lower == "_rtsp._tcp.local" || lower == "_onvif._tcp.local"
		if !isService || recordType != 12 && recordType != 33 {
			continue
		}
		nameHint := ""
		if recordType == 12 {
			value, _, err := dnsName(packet, start)
			if err != nil {
				continue
			}
			nameHint = strings.SplitN(value, "._", 2)[0]
		} else {
			nameHint = strings.SplitN(name, "._", 2)[0]
		}
		return signal{Service: "mDNS", Vendor: vendorFromText(nameHint), Name: nameHint, Confirmed: true}
	}
	return signal{}
}
