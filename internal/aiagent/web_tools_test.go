package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
)

func TestWebFetchRejectsPrivateTargets(t *testing.T) {
	service := &Service{}
	for _, target := range []string{
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://[::1]/",
		"http://localhost/",
		"https://example.com:8443/",
	} {
		t.Run(target, func(t *testing.T) {
			_, err := service.webFetch(context.Background(), json.RawMessage(`{"url":"`+target+`"}`))
			if !errors.Is(err, ErrInvalidToolInput) {
				t.Fatalf("webFetch(%q) error = %v, want invalid input", target, err)
			}
		})
	}
}

func TestWebSearchValidatesInputBeforeNetwork(t *testing.T) {
	service := &Service{}
	for _, input := range []string{
		`{"query":""}`,
		`{"query":"home ai","limit":9}`,
	} {
		if _, err := service.webSearch(context.Background(), json.RawMessage(input)); !errors.Is(err, ErrInvalidToolInput) {
			t.Fatalf("webSearch(%s) error = %v, want invalid input", input, err)
		}
	}
}

func TestPublicWebIPClassification(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.1.1", "192.168.1.1", "::1", "fc00::1", "2001:db8::1"} {
		if isPublicWebIP(net.ParseIP(raw)) {
			t.Fatalf("%s unexpectedly classified as public", raw)
		}
	}
	if !isPublicWebIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("1.1.1.1 should be public")
	}
}

func TestDuckDuckGoResultParsing(t *testing.T) {
	body := []byte(`<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fdocs">Example &amp; Docs</a>`)
	results := parseDuckDuckGoResults(body, 5)
	if len(results) != 1 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].URL != "https://example.com/docs" || results[0].Title != "Example & Docs" {
		t.Fatalf("result = %#v", results[0])
	}
}
