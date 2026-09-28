package realtime

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type Hub struct {
	nodeID    string
	streamID  string
	logger    *slog.Logger
	heartbeat time.Duration
	sequence  atomic.Uint64

	mu      sync.RWMutex
	clients map[*client]struct{}
}

type client struct {
	mu     sync.RWMutex
	topics map[string]struct{}
	send   chan Envelope
}

type Option func(*Hub)

func WithHeartbeat(interval time.Duration) Option {
	return func(h *Hub) {
		if interval > 0 {
			h.heartbeat = interval
		}
	}
}

func New(nodeID string, logger *slog.Logger, options ...Option) *Hub {
	h := &Hub{
		nodeID:    nodeID,
		streamID:  newID("stream_"),
		logger:    logger,
		heartbeat: defaultHeartbeat,
		clients:   make(map[*client]struct{}),
	}
	for _, option := range options {
		option(h)
	}
	return h
}

func (h *Hub) Publish(eventType string, data any, requestID string) Envelope {
	envelope := h.newEnvelope(eventType, data, requestID)

	h.mu.RLock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		if c.matches(eventType) && !c.deliver(envelope) {
			h.remove(c)
		}
	}
	return envelope
}

func (h *Hub) newEnvelope(eventType string, data any, requestID string) Envelope {
	return Envelope{
		Version:   ProtocolVersion,
		ID:        newID("evt_"),
		StreamID:  h.streamID,
		Sequence:  h.sequence.Add(1),
		Type:      eventType,
		Time:      time.Now().UTC(),
		Source:    Source{NodeID: h.nodeID, Component: "core"},
		RequestID: requestID,
		Data:      data,
	}
}

func (h *Hub) add() *client {
	c := &client{
		topics: map[string]struct{}{},
		send:   make(chan Envelope, 64),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

func (c *client) deliver(envelope Envelope) bool {
	select {
	case c.send <- envelope:
		return true
	default:
		return false
	}
}

func (c *client) matches(eventType string) bool {
	if eventType == "" {
		return false
	}
	if len(eventType) >= 5 && eventType[:5] == "core." {
		return true
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for topic := range c.topics {
		if topicMatches(topic, eventType) {
			return true
		}
	}
	return false
}

func (c *client) subscribe(topics []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, topic := range topics {
		c.topics[topic] = struct{}{}
	}
	return c.topicListLocked()
}

func (c *client) unsubscribe(topics []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, topic := range topics {
		delete(c.topics, topic)
	}
	return c.topicListLocked()
}

func (c *client) topicListLocked() []string {
	out := make([]string, 0, len(c.topics))
	for topic := range c.topics {
		out = append(out, topic)
	}
	sortStrings(out)
	return out
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
