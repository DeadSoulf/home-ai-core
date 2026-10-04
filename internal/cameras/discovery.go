package cameras

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

type DiscoveredDevice struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Name     string `json:"name,omitempty"`
	Scopes   string `json:"scopes,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	XAddr    string `json:"xaddr,omitempty"`
}

type discoveryEnvelope struct {
	Body struct {
		ProbeMatches struct {
			Matches []struct {
				Endpoint struct {
					Address string `xml:"Address"`
				} `xml:"EndpointReference"`
				Scopes string `xml:"Scopes"`
				XAddrs string `xml:"XAddrs"`
			} `xml:"ProbeMatch"`
		} `xml:"ProbeMatches"`
	} `xml:"Body"`
}

func (s *Service) discoverONVIF(ctx context.Context) ([]DiscoveredDevice, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("open ONVIF discovery socket: %w", err)
	}
	defer conn.Close()

	rawID := make([]byte, 16)
	_, _ = rand.Read(rawID)
	id := hex.EncodeToString(rawID)
	probe := `<?xml version="1.0" encoding="UTF-8"?><e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><e:Header><w:MessageID>uuid:` + id + `</w:MessageID><w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To><w:Action e:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action></e:Header><e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></e:Body></e:Envelope>`
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 3702}
	if _, err := conn.WriteToUDP([]byte(probe), dst); err != nil {
		return nil, fmt.Errorf("send ONVIF discovery: %w", err)
	}

	deadline := time.Now().Add(4 * time.Second)
	_ = conn.SetReadDeadline(deadline)
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
			return nil, err
		}
		var env discoveryEnvelope
		if xml.Unmarshal(buf[:n], &env) != nil {
			continue
		}
		for _, match := range env.Body.ProbeMatches.Matches {
			address := remote.IP.String()
			port := 80
			xaddr := strings.Fields(match.XAddrs)
			if len(xaddr) > 0 {
				if u, err := url.Parse(xaddr[0]); err == nil {
					if h := u.Hostname(); h != "" {
						address = h
					}
					if p := u.Port(); p != "" {
						fmt.Sscanf(p, "%d", &port)
					}
				}
			}
			name := scopeName(match.Scopes)
			found[address] = DiscoveredDevice{Address: address, Port: port, Name: name, Scopes: match.Scopes, Endpoint: match.Endpoint.Address, XAddr: strings.Join(xaddr, " ")}
		}
	}
	result := make([]DiscoveredDevice, 0, len(found))
	for _, item := range found {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Address < result[j].Address })
	return result, nil
}

func scopeName(scopes string) string {
	for _, item := range strings.Fields(scopes) {
		const prefix = "onvif://www.onvif.org/name/"
		if strings.HasPrefix(item, prefix) {
			name, _ := url.PathUnescape(strings.TrimPrefix(item, prefix))
			return name
		}
	}
	return ""
}
