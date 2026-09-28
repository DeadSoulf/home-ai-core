package realtime

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request, requestID string) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(MaxControlBytes)
	defer conn.Close(websocket.StatusNormalClosure, "connection closed")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	c := newClient()
	defer h.remove(c)

	if err := h.writeMessage(ctx, conn, c, h.newMessage("core.connected", map[string]any{
		"protocol_version":  ProtocolVersion,
		"heartbeat_seconds": int(h.heartbeat.Seconds()),
		"subscriptions":     []string{},
	}, requestID)); err != nil {
		return
	}

	// Register only after core.connected has been written, so published events
	// cannot appear before the first message of this connection stream.
	h.add(c)

	commandCh := make(chan Command)
	readErrCh := make(chan error, 1)
	go readCommands(ctx, conn, commandCh, readErrCh)

	ticker := time.NewTicker(h.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			_ = conn.Close(websocket.StatusPolicyViolation, "event consumer too slow")
			return
		case err := <-readErrCh:
			if err != nil && websocket.CloseStatus(err) == -1 && h.logger != nil {
				h.logger.Debug("realtime connection ended", "error", err)
			}
			return
		case command := <-commandCh:
			if !h.handleCommand(ctx, conn, c, command, requestID) {
				return
			}
		case message := <-c.send:
			if err := h.writeMessage(ctx, conn, c, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := h.writeMessage(ctx, conn, c, h.newMessage("core.heartbeat", map[string]any{
				"time": time.Now().UTC(),
			}, requestID)); err != nil {
				return
			}
		}
	}
}

func (h *Hub) handleCommand(
	ctx context.Context,
	conn *websocket.Conn,
	c *client,
	command Command,
	requestID string,
) bool {
	var topics []string
	var err error

	switch command.Op {
	case "subscribe":
		err = validateTopics(command.Topics)
		if err == nil {
			topics = c.subscribe(command.Topics)
		}
	case "unsubscribe":
		err = validateTopics(command.Topics)
		if err == nil {
			topics = c.unsubscribe(command.Topics)
		}
	default:
		err = errors.New("unsupported operation")
	}

	if err != nil {
		return h.writeMessage(ctx, conn, c, h.newMessage("core.error", protocolError{
			Code:    "invalid_control_message",
			Message: err.Error(),
		}, requestID)) == nil
	}

	return h.writeMessage(ctx, conn, c, h.newMessage("core.subscription.updated", map[string]any{
		"topics": topics,
	}, requestID)) == nil
}

func (h *Hub) writeMessage(
	ctx context.Context,
	conn *websocket.Conn,
	c *client,
	message eventMessage,
) error {
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return wsjson.Write(writeCtx, conn, h.envelope(c, message))
}

func readCommands(
	ctx context.Context,
	conn *websocket.Conn,
	commandCh chan<- Command,
	errCh chan<- error,
) {
	for {
		var command Command
		if err := wsjson.Read(ctx, conn, &command); err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}

		select {
		case commandCh <- command:
		case <-ctx.Done():
			return
		}
	}
}
