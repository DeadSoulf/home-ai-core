package realtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	ProtocolVersion  = 1
	MaxControlBytes  = 64 << 10
	MaxTopics        = 64
	MaxTopicLength   = 128
	defaultHeartbeat = 30 * time.Second
)

type Source struct {
	NodeID    string `json:"node_id"`
	Component string `json:"component"`
}

type Envelope struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	StreamID  string    `json:"stream_id"`
	Sequence  uint64    `json:"sequence"`
	Cursor    int64     `json:"cursor,omitempty"`
	Type      string    `json:"type"`
	Time      time.Time `json:"time"`
	Source    Source    `json:"source"`
	RequestID string    `json:"request_id,omitempty"`
	Data      any       `json:"data,omitempty"`
}

type PublishedEvent struct {
	ID        string
	Cursor    int64
	Type      string
	Time      time.Time
	Source    Source
	RequestID string
	Data      any
}

type eventMessage struct {
	ID        string
	Cursor    int64
	Type      string
	Time      time.Time
	Source    Source
	RequestID string
	Data      any
}

type Command struct {
	Op     string   `json:"op"`
	Topics []string `json:"topics,omitempty"`
}

type protocolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func newID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return prefix + "unavailable"
	}
	return prefix + hex.EncodeToString(raw[:])
}

func validateTopics(topics []string) error {
	if len(topics) == 0 {
		return fmt.Errorf("at least one topic is required")
	}
	if len(topics) > MaxTopics {
		return fmt.Errorf("too many topics")
	}

	for _, topic := range topics {
		if err := validateTopic(topic); err != nil {
			return err
		}
	}
	return nil
}

func validateTopic(topic string) error {
	if topic == "" || len(topic) > MaxTopicLength {
		return fmt.Errorf("invalid topic length")
	}
	if topic == "*" {
		return nil
	}
	if strings.HasSuffix(topic, "*") && !strings.HasSuffix(topic, ".*") {
		return fmt.Errorf("wildcard is only allowed as namespace .*")
	}
	for i, r := range topic {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		case r == '*' && i == len(topic)-1:
		default:
			return fmt.Errorf("invalid topic character")
		}
	}
	return nil
}

func topicMatches(topic, eventType string) bool {
	if strings.HasPrefix(eventType, "core.") {
		return true
	}
	if topic == "*" || topic == eventType {
		return true
	}
	if strings.HasSuffix(topic, ".*") {
		prefix := strings.TrimSuffix(topic, "*")
		return strings.HasPrefix(eventType, prefix)
	}
	return false
}
