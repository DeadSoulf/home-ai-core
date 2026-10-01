package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/smb"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type accessFixture struct {
	store    *state.Store
	security *security.Service
	owner    security.AuthResult
	friend   security.User
	login    security.AuthResult
	hub      *realtime.Hub
	handler  http.Handler
}

func actualAccessFixture(t *testing.T) accessFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sec, err := security.New(ctx, store, dir)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := os.ReadFile(sec.BootstrapTokenPath())
	if err != nil {
		t.Fatal(err)
	}
	owner, err := sec.Bootstrap(ctx, string(bootstrap), "owner", "Owner", "owner correct password", security.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	friend, err := sec.CreateUserWithAccess(ctx, owner.Actor, "friend", "Friend", "friend correct password", security.UserAccessInput{Profile: security.ProfileFriend, Permissions: []string{"events.read"}}, security.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	login, err := sec.Login(ctx, "friend", "friend correct password", security.RequestContext{})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := realtime.New("test", logger, realtime.WithHeartbeat(time.Hour))
	handler := New("test", logger, store, sec, nil, nil, nil, nil, hub)
	return accessFixture{store, sec, owner, friend, login, hub, handler}
}

func accessSocket(t *testing.T, fixture accessFixture, token string) (*websocket.Conn, context.Context) {
	t.Helper()
	server := httptest.NewServer(fixture.handler)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/events", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	var message realtime.Envelope
	if err := wsjson.Read(ctx, conn, &message); err != nil || message.Type != "core.connected" {
		t.Fatalf("connect: %+v %v", message, err)
	}
	if err := wsjson.Write(ctx, conn, realtime.Command{Op: "subscribe", Topics: []string{"*"}}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, conn, &message); err != nil || message.Type != "core.subscription.updated" {
		t.Fatalf("subscribe: %+v %v", message, err)
	}
	return conn, ctx
}

func TestRealtimeRefreshesRealSessions(t *testing.T) {
	for _, action := range []string{"logout", "disable", "remove_permission", "expiry"} {
		t.Run(action, func(t *testing.T) {
			f := actualAccessFixture(t)
			token := f.login.Token
			if action == "expiry" {
				token = "short-lived-test-session"
				hash := sha256.Sum256([]byte(token))
				if err := f.store.CreateSession(context.Background(), "short-session", f.friend.ID, hex.EncodeToString(hash[:]), "csrf", time.Now(), time.Now().Add(400*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
			}
			conn, ctx := accessSocket(t, f, token)
			switch action {
			case "logout":
				if err := f.security.Logout(ctx, f.login.Actor, security.RequestContext{}); err != nil {
					t.Fatal(err)
				}
			case "disable", "remove_permission":
				_, err := f.security.UpdateUserAccess(ctx, f.owner.Actor, f.friend.ID, security.UserAccessInput{Profile: security.ProfileFriend, Disabled: action == "disable"}, security.RequestContext{})
				if err != nil {
					t.Fatal(err)
				}
			}
			var event realtime.Envelope
			err := wsjson.Read(ctx, conn, &event)
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatalf("session remained connected: %+v %v", event, err)
			}
		})
	}
}

func TestRealtimeDoesNotDisclosePrivateFolderActivity(t *testing.T) {
	f := actualAccessFixture(t)
	ctx := context.Background()
	pool, err := f.store.CreateNASPool(ctx, "Main", t.TempDir(), "/dev/test", "uuid", f.owner.Actor.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	visible, err := f.store.CreateNASFolder(ctx, pool.ID, "Visible", "shared", "", f.owner.Actor.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	private, err := f.store.CreateNASFolder(ctx, pool.ID, "Private", "private", f.owner.Actor.ID, f.owner.Actor.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.GrantUserResourcePermission(ctx, f.friend.ID, "files.read", "file_folder", visible.ID); err != nil {
		t.Fatal(err)
	}
	conn, socketCtx := accessSocket(t, f, f.login.Token)
	f.hub.Publish("files.file.changed", map[string]any{"folder_id": private.ID, "path": "secret.txt"}, "")
	f.hub.Publish("files.file.changed", map[string]any{"folder_id": visible.ID, "path": "public.txt"}, "")
	var message realtime.Envelope
	if err := wsjson.Read(socketCtx, conn, &message); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(message.Data)
	if !strings.Contains(string(encoded), "public.txt") {
		t.Fatalf("private event leaked or visible event lost: %+v %v", message, err)
	}
	if _, err := f.security.UpdateUserAccess(ctx, f.owner.Actor, f.friend.ID, security.UserAccessInput{Profile: security.ProfileFriend, Permissions: []string{"events.read"}}, security.RequestContext{}); err != nil {
		t.Fatal(err)
	}
	f.hub.Publish("files.file.changed", map[string]any{"folder_id": visible.ID, "path": "revoked.txt"}, "")
	f.hub.Publish("core.test.marker", map[string]any{"marker": true}, "")
	if err := wsjson.Read(socketCtx, conn, &message); err != nil || message.Type != "core.test.marker" {
		t.Fatalf("revoked folder activity leaked: %+v %v", message, err)
	}
}

func TestUserAccessMutationSuspendsSMBBeforePersisting(t *testing.T) {
	for _, mode := range []string{"suspend_failed", "apply_failed", "success"} {
		t.Run(mode, func(t *testing.T) {
			f := actualAccessFixture(t)
			oldSuspend, oldApply := suspendSMBAccess, applySMBAccess
			t.Cleanup(func() { suspendSMBAccess, applySMBAccess = oldSuspend, oldApply })
			suspended, applied := false, false
			suspendSMBAccess = func(ctx context.Context) (bool, error) {
				user, err := f.store.UserAccount(ctx, f.friend.ID)
				if err != nil || user.User.Disabled {
					t.Fatal("account changed before SMB stop", err)
				}
				if mode == "suspend_failed" {
					return false, errors.New("stop failed")
				}
				suspended = true
				return true, nil
			}
			applySMBAccess = func(ctx context.Context, _ string, shares []smb.Share) (string, error) {
				user, err := f.store.UserAccount(ctx, f.friend.ID)
				if !suspended || err != nil || !user.User.Disabled {
					t.Fatal("SMB apply before authoritative disable", err)
				}
				for _, share := range shares {
					for _, name := range share.ReadUsers {
						if name == smbSystemUsername(f.friend.ID) {
							t.Fatal("disabled account retained share")
						}
					}
				}
				applied = true
				if mode == "apply_failed" {
					return "", errors.New("apply failed")
				}
				return "ok", nil
			}
			req := httptest.NewRequest("PUT", "/api/v1/security/users/"+f.friend.ID+"/access", strings.NewReader(`{"profile":"friend","permissions":[],"resource_permissions":[],"disabled":true}`))
			req.Header.Set("Authorization", "Bearer "+f.owner.Token)
			rec := httptest.NewRecorder()
			f.handler.ServeHTTP(rec, req)
			user, _ := f.store.UserAccount(context.Background(), f.friend.ID)
			if mode == "suspend_failed" {
				if rec.Code != 502 || user.User.Disabled || applied {
					t.Fatalf("failed barrier changed access: %d %s", rec.Code, rec.Body)
				}
			} else {
				if rec.Code != 200 || !user.User.Disabled || !applied {
					t.Fatalf("disable failed: %d %s", rec.Code, rec.Body)
				}
				if mode == "apply_failed" && !strings.Contains(rec.Body.String(), "suspended") {
					t.Fatal("suspended access not visible", rec.Body)
				}
				if _, err := f.security.Authenticate(context.Background(), f.login.Token); !errors.Is(err, security.ErrUnauthorized) {
					t.Fatal("disabled session survives", err)
				}
			}
		})
	}
}
