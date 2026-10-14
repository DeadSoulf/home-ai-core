package modules

import (
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
	"testing"
)

func TestRuntimeInspectionRequiresEngineEvidence(t *testing.T) {
	response := updaterhelper.Response{OK: true, ProtocolVersion: updaterhelper.ProtocolVersion, DockerAvailable: true, DockerActive: true, DockerNetworkReady: true}
	status, err := runtimeStatusFromResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if status.EngineReady || status.NetworkReady {
		t.Fatal("socket/service presence falsely advertised engine readiness")
	}
	response.DockerVersion = "29.0.0"
	status, err = runtimeStatusFromResponse(response)
	if err != nil || !status.EngineReady || !status.NetworkReady || status.CheckedAt.IsZero() {
		t.Fatalf("verified runtime = %#v, %v", status, err)
	}
	response.DockerNetworkReady = false
	status, _ = runtimeStatusFromResponse(response)
	if !status.EngineReady || status.NetworkReady {
		t.Fatal("degraded bridge must not hide a working engine")
	}
}
func TestRuntimeInspectionFailsClosedForRejectedOrOldHelper(t *testing.T) {
	for _, response := range []updaterhelper.Response{{OK: false}, {OK: true, ProtocolVersion: updaterhelper.ProtocolVersion - 1}} {
		if _, err := runtimeStatusFromResponse(response); err == nil {
			t.Fatal("unverified response accepted")
		}
	}
}
