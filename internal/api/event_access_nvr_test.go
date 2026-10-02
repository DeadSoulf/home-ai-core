package api

import (
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func TestActorAllowsNVREventByExactCameraScope(t *testing.T) {
	actor := security.Actor{
		Permissions: []string{"system.read"},
		ResourcePermissions: []security.PermissionScope{{
			Permission:   "camera.live",
			ResourceType: "camera",
			ResourceID:   "cam-front",
		}},
	}

	if !actorAllowsEvent(actor, "nvr.camera.online", map[string]any{"camera_id": "cam-front"}) {
		t.Fatal("scoped camera event was not visible")
	}
	if actorAllowsEvent(actor, "nvr.camera.offline", map[string]any{"camera_id": "cam-back"}) {
		t.Fatal("NVR event leaked to another camera scope")
	}

	global := security.Actor{Permissions: []string{"camera.list"}}
	if !actorAllowsEvent(global, "nvr.camera.online", map[string]any{"camera_id": "cam-back"}) {
		t.Fatal("global camera access did not allow NVR event")
	}
}
