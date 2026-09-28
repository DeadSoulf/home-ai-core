package realtime

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestTopicMatches(t *testing.T) {
	tests := []struct {
		topic string
		event string
		want  bool
	}{
		{"system.updated", "system.updated", true},
		{"system.*", "system.updated", true},
		{"system.*", "system", false},
		{"*", "camera.motion", true},
		{"job.*", "system.updated", false},
		{"nothing", "core.heartbeat", true},
	}

	for _, tt := range tests {
		if got := topicMatches(tt.topic, tt.event); got != tt.want {
			t.Fatalf("topicMatches(%q, %q) = %v, want %v", tt.topic, tt.event, got, tt.want)
		}
	}
}

func TestEnvelopeSequence(t *testing.T) {
	hub := New("node-1", slog.New(slog.NewTextHandler(io.Discard, nil)))
	first := hub.newEnvelope("core.test", nil, "")
	second := hub.newEnvelope("core.test", nil, "")

	if first.StreamID == "" || first.StreamID != second.StreamID {
		t.Fatal("stream ID is missing or changed")
	}
	if second.Sequence != first.Sequence+1 {
		t.Fatalf("sequence = %d after %d", second.Sequence, first.Sequence)
	}
	if first.Version != ProtocolVersion {
		t.Fatalf("version = %d", first.Version)
	}
}

func TestWebSocketSubscribeAndPublish(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := New("node-1", logger, WithHeartbeat(time.Hour))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeHTTP(w, r, "req-test")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test complete")

	var connected Envelope
	if err := wsjson.Read(ctx, conn, &connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected.Type != "core.connected" || connected.Source.NodeID != "node-1" {
		t.Fatalf("unexpected connected envelope: %#v", connected)
	}

	if err := wsjson.Write(ctx, conn, Command{
		Op:     "subscribe",
		Topics: []string{"system.*"},
	}); err != nil {
		t.Fatalf("subscribe write: %v", err)
	}

	var ack Envelope
	if err := wsjson.Read(ctx, conn, &ack); err != nil {
		t.Fatalf("read subscription ack: %v", err)
	}
	if ack.Type != "core.subscription.updated" {
		t.Fatalf("ack type = %q", ack.Type)
	}

	hub.Publish("system.updated", map[string]any{"reason": "test"}, "req-2")

	var event Envelope
	if err := wsjson.Read(ctx, conn, &event); err != nil {
		t.Fatalf("read published event: %v", err)
	}
	if event.Type != "system.updated" {
		t.Fatalf("event type = %q", event.Type)
	}
	if event.RequestID != "req-2" {
		t.Fatalf("request ID = %q", event.RequestID)
	}
}

func TestWebSocketRejectsInvalidCommand(t *testing.T) {
	hub := New("node-1", slog.New(slog.NewTextHandler(io.Discard, nil)), WithHeartbeat(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeHTTP(w, r, "req-test")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test complete")

	var connected Envelope
	if err := wsjson.Read(ctx, conn, &connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}

	if err := wsjson.Write(ctx, conn, Command{
		Op:     "subscribe",
		Topics: []string{"bad*topic"},
	}); err != nil {
		t.Fatalf("invalid command write: %v", err)
	}

	var response Envelope
	if err := wsjson.Read(ctx, conn, &response); err != nil {
		t.Fatalf("read error event: %v", err)
	}
	if response.Type != "core.error" {
		t.Fatalf("response type = %q", response.Type)
	}
}
