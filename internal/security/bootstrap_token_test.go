package security

import (
	"context"
	"errors"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapTokenIsLocalPersistentAndSingleUse(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sec, err := New(ctx, store, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "bootstrap-token"))
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(data))
	info, _ := os.Stat(filepath.Join(dir, "bootstrap-token"))
	if len(token) < 32 || info.Mode().Perm() != 0o600 {
		t.Fatal("weak setup secret")
	}
	for _, value := range []string{"", "wrong", token + "x"} {
		if !errors.Is(sec.CheckBootstrapToken(ctx, value), ErrInvalidBootstrapToken) {
			t.Fatal("invalid setup code accepted")
		}
	}
	sec2, err := New(ctx, store, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := sec2.CheckBootstrapToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := sec2.Bootstrap(ctx, "owner", "Owner", "correct owner passphrase", RequestContext{}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(sec2.CheckBootstrapToken(ctx, token), ErrAlreadyInitialized) {
		t.Fatal("setup token replay accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "bootstrap-token")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("setup code was not removed")
	}
}
