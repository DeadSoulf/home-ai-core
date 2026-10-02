package api

import (
	"encoding/json"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func actorAllowsEvent(actor security.Actor, eventType string, data any) bool {
	switch {
	case strings.HasPrefix(eventType, "core."):
		return true
	case strings.HasPrefix(eventType, "files."):
		if actor.Has("files.manage") {
			return true
		}
		values, ok := data.(map[string]any)
		if !ok {
			encoded, err := json.Marshal(data)
			if err != nil || json.Unmarshal(encoded, &values) != nil {
				return false
			}
		}
		id, _ := values["folder_id"].(string)
		return id != "" && actor.Allows("files.read", "file_folder", id)
	case strings.HasPrefix(eventType, "nvr."):
		if actor.Has("camera.list") || actor.Has("camera.live") || actor.Has("camera.archive") || actor.Has("camera.manage") {
			return true
		}
		values, ok := data.(map[string]any)
		if !ok {
			encoded, err := json.Marshal(data)
			if err != nil || json.Unmarshal(encoded, &values) != nil {
				return false
			}
		}
		id, _ := values["camera_id"].(string)
		if id == "" {
			return false
		}
		return actor.Allows("camera.live", "camera", id) ||
			actor.Allows("camera.archive", "camera", id) ||
			actor.Allows("camera.export", "camera", id) ||
			actor.Allows("camera.ptz", "camera", id) ||
			actor.Allows("camera.manage", "camera", id)
	case strings.HasPrefix(eventType, "security."):
		return actor.Has("security.users.read")
	case strings.HasPrefix(eventType, "storage."):
		return actor.Has("system.read")
	case strings.HasPrefix(eventType, "network."):
		return actor.Has("network.read")
	case strings.HasPrefix(eventType, "update."):
		return actor.Has("updates.read")
	case strings.HasPrefix(eventType, "modules."):
		return actor.Has("modules.read")
	case strings.HasPrefix(eventType, "jobs."):
		return actor.Has("jobs.read")
	default:
		return actor.Has("system.read")
	}
}
