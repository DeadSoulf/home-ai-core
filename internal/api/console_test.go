package api

import (
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func TestConsoleTicketIsOneTimeAndSessionBound(t *testing.T) {
	s := &server{console: newConsoleState()}
	actor := security.Actor{ID: "usr_admin", SessionID: "ses_one"}

	s.console.mu.Lock()
	s.console.tickets["ticket"] = consoleTicket{
		ActorID:   actor.ID,
		SessionID: actor.SessionID,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	s.console.mu.Unlock()

	if !s.consumeConsoleTicket("ticket", actor) {
		t.Fatal("expected matching console ticket to be accepted")
	}
	if s.consumeConsoleTicket("ticket", actor) {
		t.Fatal("expected console ticket to be one-time")
	}
}

func TestConsoleTicketRejectsDifferentSession(t *testing.T) {
	s := &server{console: newConsoleState()}
	s.console.mu.Lock()
	s.console.tickets["ticket"] = consoleTicket{
		ActorID:   "usr_admin",
		SessionID: "ses_one",
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	s.console.mu.Unlock()

	if s.consumeConsoleTicket("ticket", security.Actor{ID: "usr_admin", SessionID: "ses_two"}) {
		t.Fatal("expected console ticket to reject another session")
	}
}

func TestConsoleTicketRejectsExpiredTicket(t *testing.T) {
	s := &server{console: newConsoleState()}
	actor := security.Actor{ID: "usr_admin", SessionID: "ses_one"}
	s.console.mu.Lock()
	s.console.tickets["ticket"] = consoleTicket{
		ActorID:   actor.ID,
		SessionID: actor.SessionID,
		ExpiresAt: time.Now().UTC().Add(-time.Second),
	}
	s.console.mu.Unlock()

	if s.consumeConsoleTicket("ticket", actor) {
		t.Fatal("expected expired console ticket to be rejected")
	}
}
