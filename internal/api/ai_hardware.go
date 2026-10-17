package api

import (
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

// AI hardware inventory is computed by Core (not by an untrusted container).
// GPUs are discovered afresh from sysfs on every request. The UI polls this
// endpoint to detect devices that the OS recognizes after hot-plug/reboot.
// Seeing a GPU does not prove that a compatible driver/toolkit is available.
func (s *server) aiHardware(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	info := systeminfo.Collect(s.nodeID)
	if info.GPUs == nil {
		info.GPUs = []systeminfo.GPU{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cpu":        info.CPU,
		"memory":     info.Memory,
		"gpus":       info.GPUs,
		"checked_at": time.Now().UTC(),
		"policy":     "cpu_first_gpu_requires_explicit_approval",
	})
}
