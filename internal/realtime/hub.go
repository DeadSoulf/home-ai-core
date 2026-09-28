package realtime

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

type Hub struct {
	nodeID    string
	logger    *slog.Logger
	heartbeat time.Duration

	mu      sync.RWMutex
	clients map[*client]struct{}
}

type client struct {
	mu       sync.RWMutex
	topics   map[string]struct{}
	send     chan eventMessage
	done     chan struct{}
	closeOnce sync.Once
	streamID string
	sequence uint64
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
		logger:    logger,
		heartbeat: defaultHeartbeat,
		clients:   make(map[*client]struct{}),
	}
	for _, option := range options {
		option(h)
	}
	return h
}

func (h *Hub) Publish(eventType string, data any, requestID string) string {
	message := h.newMessage(eventType, data, requestID)

	h.mu.RLock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		if c.matches(eventType) && !c.deliver(message) {
			h.remove(c)
		}
	}
	return message.ID
}

func (h *Hub) newMessage(eventType string, data any, requestID string) eventMessage {
	return eventMessage{
		ID:        newID("evt_"),
		Type:      eventType,
		Time:      time.Now().UTC(),
		Source:    Source{NodeID: h.nodeID, Component: "core"},
		RequestID: requestID,
		Data:      data,
	}
}

func (h *Hub) envelope(c *client, message eventMessage) Envelope {
	c.sequence++
	return Envelope{
		Version:   ProtocolVersion,
		ID:        message.ID,
		StreamID:  c.streamID,
		Sequence:  c.sequence,
		Type:      message.Type,
		Time:      message.Time,
		Source:    message.Source,
		RequestID: message.RequestID,
		Data:      message.Data,
	}
}

func newClient() *client {
	return &client{
		topics:   map[string]struct{}{},
		send:     make(chan eventMessage, 64),
		done:     make(chan struct{}),
		streamID: newID("stream_"),
	}
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	c.close()
}

func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
	})
}

func (c *client) deliver(message eventMessage) bool {
	select {
	case <-c.done:
		return false
	case c.send <- message:
		return true
	default:
		return false
	}
}

func (c *client) matches(eventType string) bool {
	if eventType == "" {
		return false
	}
	if strings.HasPrefix(eventType, "core.") {
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
	sort.Strings(out)
	return out
}
