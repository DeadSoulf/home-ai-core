package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxAIWebResponseBytes = 512 << 10
	defaultAIWebMaxChars  = 8000
	maxAIWebMaxChars      = 12000
)

var (
	webScriptStylePattern = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>|<noscript[^>]*>.*?</noscript>`)
	webTagPattern         = regexp.MustCompile(`(?s)<[^>]+>`)
	webWhitespacePattern  = regexp.MustCompile(`\s+`)
	webAnchorPattern      = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
)

type webSearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type webSearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type webFetchInput struct {
	URL      string `json:"url"`
	MaxChars int    `json:"max_chars,omitempty"`
}

type publicWebResponse struct {
	Body        []byte
	FinalURL    string
	ContentType string
	Truncated   bool
}

func (s *Service) webSearch(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var request webSearchInput
	if err := decodeToolInput(input, &request); err != nil {
		return nil, err
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" || utf8.RuneCountInString(request.Query) > 500 {
		return nil, ErrInvalidToolInput
	}
	if request.Limit == 0 {
		request.Limit = 5
	}
	if request.Limit < 1 || request.Limit > 8 {
		return nil, ErrInvalidToolInput
	}

	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(request.Query)
	response, err := publicWebGET(ctx, searchURL, maxAIWebResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("web search failed: %w", err)
	}

	results := parseDuckDuckGoResults(response.Body, request.Limit)
	return json.Marshal(map[string]any{
		"query":   request.Query,
		"engine":  "duckduckgo",
		"results": results,
	})
}

func (s *Service) webFetch(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var request webFetchInput
	if err := decodeToolInput(input, &request); err != nil {
		return nil, err
	}
	request.URL = strings.TrimSpace(request.URL)
	if request.MaxChars == 0 {
		request.MaxChars = defaultAIWebMaxChars
	}
	if request.MaxChars < 1000 || request.MaxChars > maxAIWebMaxChars {
		return nil, ErrInvalidToolInput
	}
	if err := validatePublicWebURL(request.URL); err != nil {
		return nil, err
	}

	response, err := publicWebGET(ctx, request.URL, maxAIWebResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("web fetch failed: %w", err)
	}

	mediaType, _, _ := mime.ParseMediaType(response.ContentType)
	text := string(response.Body)
	switch mediaType {
	case "text/html", "application/xhtml+xml":
		text = readableHTMLText(text)
	default:
		if !strings.HasPrefix(mediaType, "text/") &&
			mediaType != "application/json" &&
			mediaType != "application/xml" &&
			mediaType != "application/rss+xml" &&
			mediaType != "application/atom+xml" &&
			mediaType != "" {
			return nil, fmt.Errorf("web fetch content type %q is not readable text", mediaType)
		}
		text = strings.TrimSpace(text)
	}

	truncated := response.Truncated
	runes := []rune(text)
	if len(runes) > request.MaxChars {
		text = string(runes[:request.MaxChars])
		truncated = true
	}
	return json.Marshal(map[string]any{
		"url":          response.FinalURL,
		"content_type": mediaType,
		"text":         text,
		"truncated":    truncated,
	})
}

func publicWebGET(ctx context.Context, rawURL string, maxBytes int64) (publicWebResponse, error) {
	if err := validatePublicWebURL(rawURL); err != nil {
		return publicWebResponse{}, err
	}

	client := newPublicWebClient()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return publicWebResponse{}, err
	}
	request.Header.Set("Accept", "text/html, text/plain, application/json, application/xml;q=0.9, */*;q=0.1")
	request.Header.Set("User-Agent", "HOME-AI web tool/0.1")

	response, err := client.Do(request)
	if err != nil {
		return publicWebResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return publicWebResponse{}, fmt.Errorf("remote server returned HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return publicWebResponse{}, err
	}
	truncated := int64(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}
	return publicWebResponse{
		Body:        body,
		FinalURL:    response.Request.URL.String(),
		ContentType: response.Header.Get("Content-Type"),
		Truncated:   truncated,
	}, nil
}

func newPublicWebClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("web host did not resolve")
		}
		for _, resolved := range ips {
			if !isPublicWebIP(resolved.IP) {
				return nil, fmt.Errorf("web host resolved to a non-public address")
			}
		}

		var lastErr error
		for _, resolved := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("web host has no dialable public address")
		}
		return nil, lastErr
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many web redirects")
			}
			return validatePublicWebURL(req.URL.String())
		},
	}
}

func validatePublicWebURL(rawURL string) error {
	if utf8.RuneCountInString(rawURL) == 0 || utf8.RuneCountInString(rawURL) > 2048 {
		return ErrInvalidToolInput
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ErrInvalidToolInput
	}
	if parsed.User != nil {
		return ErrInvalidToolInput
	}
	if strings.EqualFold(parsed.Hostname(), "localhost") || strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".localhost") {
		return fmt.Errorf("%w: local web targets are not allowed", ErrInvalidToolInput)
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || (value != 80 && value != 443) {
			return fmt.Errorf("%w: only ports 80 and 443 are allowed", ErrInvalidToolInput)
		}
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !isPublicWebIP(ip) {
		return fmt.Errorf("%w: private or local web targets are not allowed", ErrInvalidToolInput)
	}
	return nil
}

func isPublicWebIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127:
			return false
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 0:
			return false
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19):
			return false
		case v4[0] >= 224:
			return false
		}
		return true
	}
	v6 := ip.To16()
	if v6 == nil {
		return false
	}
	if v6[0] == 0x20 && v6[1] == 0x01 && v6[2] == 0x0d && v6[3] == 0xb8 {
		return false
	}
	return true
}

func parseDuckDuckGoResults(body []byte, limit int) []webSearchResult {
	matches := webAnchorPattern.FindAllStringSubmatch(string(body), -1)
	results := make([]webSearchResult, 0, limit)
	seen := make(map[string]struct{})
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		target := decodeDuckDuckGoURL(html.UnescapeString(match[1]))
		if target == "" {
			continue
		}
		title := readableHTMLText(match[2])
		if title == "" {
			continue
		}
		if _, exists := seen[target]; exists {
			continue
		}
		seen[target] = struct{}{}
		results = append(results, webSearchResult{Title: title, URL: target})
		if len(results) >= limit {
			break
		}
	}
	return results
}

func decodeDuckDuckGoURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if strings.HasSuffix(host, "duckduckgo.com") && parsed.Path == "/l/" {
		raw = parsed.Query().Get("uddg")
		parsed, err = url.Parse(raw)
		if err != nil {
			return ""
		}
		host = strings.ToLower(parsed.Hostname())
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	if host == "" || strings.HasSuffix(host, "duckduckgo.com") {
		return ""
	}
	if err := validatePublicWebURL(parsed.String()); err != nil {
		return ""
	}
	return parsed.String()
}

func readableHTMLText(value string) string {
	value = webScriptStylePattern.ReplaceAllString(value, " ")
	value = webTagPattern.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	value = webWhitespacePattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}
