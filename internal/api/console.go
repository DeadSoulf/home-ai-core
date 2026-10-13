package api

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	consoleTicketLifetime  = 30 * time.Second
	consoleSessionLifetime = 2 * time.Hour
	consoleReadLimit       = 32 << 10
	consoleChunkSize       = 4096
)

type consoleTicket struct {
	ActorID   string
	SessionID string
	ExpiresAt time.Time
}

type consoleState struct {
	mu      sync.Mutex
	tickets map[string]consoleTicket
}

type consoleClientMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
}

type consoleServerMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Code int    `json:"code,omitempty"`
}

func newConsoleState() *consoleState {
	return &consoleState{tickets: make(map[string]consoleTicket)}
}

func (s *server) consoleSession(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "console_ticket_failed", "failed to create console ticket", nil)
		return
	}
	ticket := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().UTC().Add(consoleTicketLifetime)

	s.console.mu.Lock()
	now := time.Now().UTC()
	for key, item := range s.console.tickets {
		if !item.ExpiresAt.After(now) {
			delete(s.console.tickets, key)
		}
	}
	s.console.tickets[ticket] = consoleTicket{
		ActorID:   actor.ID,
		SessionID: actor.SessionID,
		ExpiresAt: expires,
	}
	s.console.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"ticket":     ticket,
		"expires_at": expires,
	})
}

func (s *server) consoleSocket(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" || !s.consumeConsoleTicket(ticket, actor) {
		writeAPIError(w, r, http.StatusForbidden, "invalid_console_ticket", "console ticket is invalid or expired", nil)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(consoleReadLimit)
	defer conn.Close(websocket.StatusNormalClosure, "console closed")

	ctx, cancel := context.WithTimeout(r.Context(), consoleSessionLifetime)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/bash", "--noprofile", "--norc")
	cmd.Dir = consoleWorkingDirectory()
	cmd.Env = consoleEnvironment()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = wsjson.Write(ctx, conn, consoleServerMessage{Type: "error", Data: "failed to open shell input"})
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = wsjson.Write(ctx, conn, consoleServerMessage{Type: "error", Data: "failed to open shell output"})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = wsjson.Write(ctx, conn, consoleServerMessage{Type: "error", Data: "failed to open shell error stream"})
		return
	}
	if err := cmd.Start(); err != nil {
		_ = wsjson.Write(ctx, conn, consoleServerMessage{Type: "error", Data: "failed to start shell"})
		return
	}
	defer stopConsoleProcess(cmd)

	meta := s.securityRequestContext(r)
	started := time.Now().UTC()
	s.security.RecordAudit(ctx, meta, actor, "system.console.open", "node", s.nodeID, "success", map[string]any{
		"shell": "/bin/bash",
	})
	defer func() {
		s.security.RecordAudit(context.Background(), meta, actor, "system.console.close", "node", s.nodeID, "success", map[string]any{
			"duration_seconds": int(time.Since(started).Seconds()),
		})
	}()

	if err := wsjson.Write(ctx, conn, consoleServerMessage{
		Type: "ready",
		Data: "Connected as the unprivileged Home-AI-Core service user. This is not a root shell.\n",
	}); err != nil {
		return
	}

	outputCh := make(chan string, 32)
	processDone := make(chan error, 1)
	readErr := make(chan error, 1)
	var outputWG sync.WaitGroup
	outputWG.Add(2)
	go streamConsoleOutput(ctx, stdout, outputCh, &outputWG)
	go streamConsoleOutput(ctx, stderr, outputCh, &outputWG)
	go func() {
		outputWG.Wait()
		close(outputCh)
	}()
	go func() {
		processDone <- cmd.Wait()
	}()
	go readConsoleInput(ctx, conn, stdin, readErr)

	token, _ := sessionToken(r)
	authTicker := time.NewTicker(5 * time.Second)
	defer authTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-readErr:
			if err != nil && websocket.CloseStatus(err) == -1 {
				s.logger.Debug("console websocket ended", "actor_id", actor.ID, "error", err)
			}
			return
		case output, ok := <-outputCh:
			if !ok {
				outputCh = nil
				continue
			}
			writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
			err := wsjson.Write(writeCtx, conn, consoleServerMessage{Type: "output", Data: output})
			writeCancel()
			if err != nil {
				return
			}
		case err := <-processDone:
			code := 0
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					code = exitErr.ExitCode()
				} else {
					code = -1
				}
			}
			_ = wsjson.Write(ctx, conn, consoleServerMessage{Type: "exit", Code: code})
			return
		case <-authTicker.C:
			current, err := s.security.Authenticate(ctx, token)
			if err != nil || current.ID != actor.ID || !current.Has("system.console") {
				_ = conn.Close(websocket.StatusPolicyViolation, "console permission expired")
				return
			}
		}
	}
}

func (s *server) consumeConsoleTicket(value string, actor security.Actor) bool {
	s.console.mu.Lock()
	defer s.console.mu.Unlock()

	item, ok := s.console.tickets[value]
	delete(s.console.tickets, value)
	if !ok || !item.ExpiresAt.After(time.Now().UTC()) {
		return false
	}
	return item.ActorID == actor.ID && item.SessionID == actor.SessionID
}

func readConsoleInput(
	ctx context.Context,
	conn *websocket.Conn,
	stdin io.WriteCloser,
	errCh chan<- error,
) {
	defer stdin.Close()
	for {
		var message consoleClientMessage
		if err := wsjson.Read(ctx, conn, &message); err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}
		if message.Type != "input" || message.Data == "" {
			continue
		}
		if len(message.Data) > consoleReadLimit {
			select {
			case errCh <- errors.New("console input is too large"):
			case <-ctx.Done():
			}
			return
		}
		if _, err := io.WriteString(stdin, message.Data); err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}
	}
}

func streamConsoleOutput(
	ctx context.Context,
	reader io.Reader,
	output chan<- string,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	buffer := make([]byte, consoleChunkSize)
	stream := bufio.NewReader(reader)
	for {
		count, err := stream.Read(buffer)
		if count > 0 {
			text := strings.ReplaceAll(string(buffer[:count]), "\r\n", "\n")
			select {
			case output <- text:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func consoleWorkingDirectory() string {
	const stateDir = "/var/lib/home-ai-core"
	if info, err := os.Stat(stateDir); err == nil && info.IsDir() {
		return stateDir
	}
	return "/"
}

func consoleEnvironment() []string {
	home := consoleWorkingDirectory()
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		user = "home-ai-core"
	}
	return []string{
		"HOME=" + home,
		"USER=" + user,
		"LOGNAME=" + user,
		"SHELL=/bin/bash",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"TERM=dumb",
	}
}

func stopConsoleProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}
	_ = cmd.Process.Kill()
}
