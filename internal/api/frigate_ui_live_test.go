package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/webui"
)

// Explicit isolated browser fixture: real Core sessions and original Frigate
// frontend/API. The Docker fixture supplies only a synthetic camera and data.
// This never connects to the user's Core database or privileged updater.
func TestFrigateNativeBrowserFixture(t *testing.T) {
	if os.Getenv("FRIGATE_NATIVE_BROWSER") != "1" {
		t.Skip("explicit isolated native browser fixture")
	}
	address := os.Getenv("FRIGATE_NATIVE_CONTAINER_IP")
	ip := net.ParseIP(address)
	if ip == nil || !ip.IsPrivate() || ip.IsLoopback() {
		t.Fatal("a private isolated Docker fixture IP is required")
	}
	ready := os.Getenv("FRIGATE_NATIVE_READY_FILE")
	if ready == "" {
		t.Fatal("fixture readiness file is required")
	}
	f := actualAccessFixture(t)
	handler, ok := f.handler.(*Handler)
	if !ok {
		t.Fatal("API wrapper is unavailable")
	}
	handler.server.modules = frigateAPIModules(true)
	handler.server.jobs = &uiJobService{}
	handler.server.frigateEndpoint = func(context.Context) (string, error) { return address, nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	webDir, err := filepath.Abs("../../web/dist")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil {
		t.Fatal("build the real Core Web UI first", err)
	}
	core := httptest.NewUnstartedServer(webui.New(handler, webDir))
	core.Listener.Close()
	core.Listener, err = net.Listen("tcp", "127.0.0.1:18082")
	if err != nil {
		t.Fatal(err)
	}
	gateway := handler.ConfigureFrigateUI(FrigateUIConfig{ListenAddress: "127.0.0.1:18971", CoreListenAddress: "127.0.0.1:18082", Context: ctx})
	ui := httptest.NewUnstartedServer(gateway)
	ui.Listener.Close()
	ui.Listener, err = net.Listen("tcp", "127.0.0.1:18971")
	if err != nil {
		core.Listener.Close()
		t.Fatal(err)
	}
	core.StartTLS()
	defer core.Close()
	ui.StartTLS()
	defer ui.Close()
	handler.SetFrigateUIReady(true, "")
	payload, _ := json.Marshal(map[string]string{"core": core.URL, "ui": ui.URL, "mode": "isolated-real-Core-auth-and-original-Frigate"})
	if err := os.WriteFile(ready, payload, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(ready)
	t.Log("native browser fixture ready; synthetic camera only")
	timer := time.NewTimer(15 * time.Minute)
	defer timer.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-timer.C:
			t.Fatal("native browser fixture timed out")
		case <-tick.C:
			if _, err := os.Stat(ready + ".stop"); err == nil {
				return
			}
		}
	}
}
