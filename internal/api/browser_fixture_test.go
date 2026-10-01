package api

import (
	"context"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/webui"
)

// A disposable real-API fixture for manual/automated Web acceptance. It runs
// only when requested, never invokes the privileged broker or real Samba.
func TestUsersFilesBrowserFixture(t *testing.T) {
	if os.Getenv("HOME_AI_BROWSER_QA") != "1" {
		t.Skip("explicit browser fixture only")
	}
	f := actualAccessFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	pool, err := f.store.CreateNASPool(ctx, "Home storage", root, "/dev/test", "qa-filesystem", f.owner.Actor.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct{ name, kind, owner string }{{"Family", "shared", ""}, {"Friend personal", "private", f.friend.ID}} {
		folder, err := f.store.CreateNASFolder(ctx, pool.ID, spec.name, spec.kind, spec.owner, f.owner.Actor.ID, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".home-ai", folder.RelativePath), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".home-ai", folder.RelativePath, "Welcome.txt"), []byte("Home-AI acceptance fixture"), 0640); err != nil {
			t.Fatal(err)
		}
	}
	oldSuspend, oldPrepare := suspendSMBAccess, prepareFileFolder
	t.Cleanup(func() { suspendSMBAccess, prepareFileFolder = oldSuspend, oldPrepare })
	suspendSMBAccess = func(context.Context) (bool, error) { return false, nil }
	prepareFileFolder = func(_ context.Context, root, relative string) error {
		return os.MkdirAll(filepath.Join(root, ".home-ai", relative), 0750)
	}
	webDir, err := filepath.Abs("../../web/dist")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(webui.New(f.handler, webDir))
	server.Listener, err = net.Listen("tcp", "0.0.0.0:18087")
	if err != nil {
		t.Fatal(err)
	}
	server.Start()
	defer server.Close()
	t.Log("Browser fixture ready on :18087; disposable owner / owner correct password")
	timer := time.NewTimer(10 * time.Minute)
	defer timer.Stop()
	stop := time.NewTicker(time.Second)
	defer stop.Stop()
	for {
		select {
		case <-timer.C:
			return
		case <-stop.C:
			if _, err := os.Stat(filepath.Join(webDir, "..", "..", "..", "stop-users-files-browser")); err == nil {
				return
			}
		}
	}
}
