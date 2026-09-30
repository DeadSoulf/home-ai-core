//go:build windows

package windowsclient

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsCredentialManagerRoundTrip(t *testing.T) {
	server := fmt.Sprintf("http://127.0.0.1:%d", 30000+time.Now().UnixNano()%20000)
	username := fmt.Sprintf("home-ai-test-%d", time.Now().UnixNano())

	_ = DeletePassword(server, username)
	if err := SavePassword(server, username, "first-secret"); err != nil {
		if errors.Is(err, windows.ERROR_NO_SUCH_LOGON_SESSION) {
			t.Skipf("Windows runner has no credential set: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DeletePassword(server, username) })

	password, err := LoadPassword(server, username)
	if err != nil {
		t.Fatal(err)
	}
	if password != "first-secret" {
		t.Fatalf("loaded password = %q", password)
	}

	if err := SavePassword(server, username, "replacement-secret"); err != nil {
		t.Fatal(err)
	}
	password, err = LoadPassword(server, username)
	if err != nil {
		t.Fatal(err)
	}
	if password != "replacement-secret" {
		t.Fatalf("replacement password = %q", password)
	}

	if err := DeletePassword(server, username); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPassword(server, username); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("load after delete = %v", err)
	}
}
