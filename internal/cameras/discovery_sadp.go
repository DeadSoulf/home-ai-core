package cameras

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

const (
	sadpGroup = "239.255.255.250"
	sadpPort  = 37020
)

type sadpProbeMatch struct {
	XMLName           xml.Name `xml:"ProbeMatch"`
	DeviceDescription string   `xml:"DeviceDescription"`
	DeviceSN          string   `xml:"DeviceSN"`
	CommandPort       int      `xml:"CommandPort"`
	HttpPort          int      `xml:"HttpPort"`
	MAC               string   `xml:"MAC"`
	IPv4Address       string   `xml:"IPv4Address"`
	SoftwareVersion   string   `xml:"SoftwareVersion"`
}

func (s *Service) Discover(ctx context.Context) ([]DiscoveredDevice, error) {
	type result struct {
		items []DiscoveredDevice
		err   error
	}
	ch := make(chan result, 2)
	go func() {
		items, err := s.discoverSADP(ctx)
		ch <- result{items: items, err: err}
	}()
	go func() {
		items, err := s.discoverONVIF(ctx)
		ch <- result{items: items, err: err}
	}()

	found := map[string]DiscoveredDevice{}
	var errs []string
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err != nil {
			errs = append(errs, r.err.Error())
			continue
		}
		for _, item := range r.items {
			existing, ok := found[item.Address]
			// Prefer SADP: it reports Hikvision's actual SDK command port and
			// model even when ONVIF is disabled or configured on another port.
			if !ok || item.Port == 8000 {
				found[item.Address] = item
			} else if existing.Name == "" && item.Name != "" {
				existing.Name = item.Name
				found[item.Address] = existing
			}
		}
	}
	if len(found) == 0 && len(errs) == 2 {
		return nil, fmt.Errorf("camera discovery failed: %s", strings.Join(errs, "; "))
	}
	items := make([]DiscoveredDevice, 0, len(found))
	for _, item := range found {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Address < items[j].Address })
	return items, nil
}

func (s *Service) discoverSADP(ctx context.Context) ([]DiscoveredDevice, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("open Hikvision SADP socket: %w", err)
	}
	defer conn.Close()

	rawID := make([]byte, 16)
	_, _ = rand.Read(rawID)
	id := strings.ToUpper(hex.EncodeToString(rawID))
	if len(id) == 32 {
		id = id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:32]
	}
	probe := `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>` + id + `</Uuid><Types>inquiry</Types></Probe>`
	dst := &net.UDPAddr{IP: net.ParseIP(sadpGroup), Port: sadpPort}

	// Official SADP sends multicast discovery. Sending it twice improves
	// compatibility with devices that miss the first datagram during startup.
	for i := 0; i < 2; i++ {
		if _, err := conn.WriteToUDP([]byte(probe), dst); err != nil {
			return nil, fmt.Errorf("send Hikvision SADP discovery: %w", err)
		}
	}

	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	found := map[string]DiscoveredDevice{}
	buf := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, fmt.Errorf("read Hikvision SADP response: %w", err)
		}
		var match sadpProbeMatch
		if err := xml.Unmarshal(buf[:n], &match); err != nil || match.XMLName.Local != "ProbeMatch" {
			continue
		}
		address := strings.TrimSpace(match.IPv4Address)
		if net.ParseIP(address) == nil {
			address = remote.IP.String()
		}
		port := match.CommandPort
		if port <= 0 || port > 65535 {
			port = 8000
		}
		name := strings.TrimSpace(match.DeviceDescription)
		if name == "" {
			name = strings.TrimSpace(match.DeviceSN)
		}
		found[address] = DiscoveredDevice{
			Address: address,
			Port:    port,
			Name:    name,
			Scopes:  "hikvision:sadp mac=" + strings.TrimSpace(match.MAC) + " firmware=" + strings.TrimSpace(match.SoftwareVersion),
		}
	}
	items := make([]DiscoveredDevice, 0, len(found))
	for _, item := range found {
		items = append(items, item)
	}
	return items, nil
}
