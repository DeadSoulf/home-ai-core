package security

import (
	"context"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestAccessCatalogIncludesCameraResources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	camera, err := store.CreateNVRCamera(
		ctx,
		"Garage",
		"rtsp",
		"rtsp://192.0.2.20/stream",
		"",
		"tcp",
		"off",
		"",
		false,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	service, err := New(ctx, store, dir)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := service.AccessCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, resource := range catalog.Resources {
		if resource.Type != "camera" || resource.ID != camera.ID {
			continue
		}
		found = true
		if resource.Name != "Garage" {
			t.Fatalf("camera resource name = %q", resource.Name)
		}
		want := map[string]bool{
			"camera.live":    true,
			"camera.archive": true,
			"camera.export":  true,
			"camera.ptz":     true,
			"camera.manage":  true,
		}
		for _, permission := range resource.Permissions {
			delete(want, permission)
		}
		if len(want) != 0 {
			t.Fatalf("camera resource missing permissions: %#v", want)
		}
	}
	if !found {
		t.Fatal("camera resource was not exposed by the access catalog")
	}
}
