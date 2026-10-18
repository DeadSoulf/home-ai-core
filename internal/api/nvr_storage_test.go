package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func testNVRVolume(uuid, path string) state.StoragePurposeRecord {
	return state.StoragePurposeRecord{DevicePath: path, FilesystemUUID: uuid, Purpose: state.StoragePurposeNVR}
}
func testNVRNode(path, uuid, mount string, size, free uint64, known bool) systeminfo.BlockNode {
	mounts := []string{}
	if mount != "" { mounts = append(mounts, mount) }
	return systeminfo.BlockNode{
		Path: path, UUID: uuid, Type: "part", Label: "NVR", Filesystem: "ext4",
		SizeBytes: size, FreeBytes: free, FreeKnown: known, Mountpoints: mounts,
	}
}

func TestNVRDiskFreeSpaceByUUIDAndMountpoint(t *testing.T) {
	assignments := []state.StoragePurposeRecord{
		{DevicePath: "/dev/sda1", Purpose: state.StoragePurposeFiles},
		testNVRVolume("nvr-uuid", "/dev/sdb1"),
	}
	nodes := []systeminfo.BlockNode{testNVRNode("/dev/sdc1", "nvr-uuid", "/mnt/home-ai-core/sdc1", 1000, 100, true)}
	inspection := storage.Inspection{Filesystems: []updaterhelper.FilesystemStat{{
		Device: "/dev/sdc1", Mountpoint: "/mnt/home-ai-core/sdc1",
		TotalBytes: 900, TotalKnown: true, FreeBytes: 640, FreeKnown: true,
	}}}
	got := nvrCapacityFromInventory(assignments, nodes, inspection)
	if got.AssignedCount != 1 || got.MountedCount != 1 || !got.CapacityKnown || got.TotalBytes != 900 || got.FreeBytes != 640 {
		t.Fatalf("aggregate = %+v", got)
	}
	if len(got.Disks) != 1 || got.Disks[0].Device != "/dev/sdc1" || got.Disks[0].UsedBytes != 260 {
		t.Fatalf("UUID and capacity = %+v", got.Disks)
	}
}

func TestNVRMissingUnmountedAndUnknownNeverFakeZeroFree(t *testing.T) {
	assignments := []state.StoragePurposeRecord{
		testNVRVolume("missing", "/dev/sdd1"), testNVRVolume("offline", "/dev/sdb1"),
	}
	nodes := []systeminfo.BlockNode{testNVRNode("/dev/sdb1", "offline", "", 1000, 0, true)}
	got := nvrCapacityFromInventory(assignments, nodes, storage.Inspection{})
	if got.AssignedCount != 2 || got.CapacityKnown || got.TotalBytes != 0 || got.FreeBytes != 0 ||
		got.Disks[0].State != "missing" || got.Disks[1].State != "unmounted" {
		t.Fatalf("offline disk looks full: %+v", got)
	}
	full := testNVRNode("/dev/sdb1", "offline", "/mnt/home-ai-core/sdb1", 1000, 0, true)
	got = nvrCapacityFromInventory(assignments[1:], []systeminfo.BlockNode{full}, storage.Inspection{})
	if !got.CapacityKnown || got.Disks[0].FreeBytes != 0 || got.Disks[0].UsedBytes != 1000 {
		t.Fatalf("genuine full disk not recognized: %+v", got)
	}
	full.FreeKnown = false
	got = nvrCapacityFromInventory(assignments[1:], []systeminfo.BlockNode{full}, storage.Inspection{})
	if got.CapacityKnown || got.Disks[0].CapacityKnown {
		t.Fatalf("unknown free space was reported full: %+v", got)
	}
}

func TestNVRStorageDoesNotSumPartialOrStaleInventory(t *testing.T) {
	assignments := []state.StoragePurposeRecord{testNVRVolume("fs1", "/dev/sdb1"), testNVRVolume("fs2", "/dev/sdc1")}
	nodes := []systeminfo.BlockNode{
		testNVRNode("/dev/sdb1", "fs1", "/mnt/home-ai-core/sdb1", 1000, 500, true),
		testNVRNode("/dev/sdc1", "fs2", "", 2000, 1000, true),
	}
	got := nvrCapacityFromInventory(assignments, nodes, storage.Inspection{})
	if got.CapacityKnown || got.FreeBytes != 0 || !got.Disks[0].CapacityKnown {
		t.Fatalf("incomplete disk list was aggregated: %+v", got)
	}
	inspection := storage.Inspection{Filesystems: []updaterhelper.FilesystemStat{{
		Device: "/dev/sdb1", Mountpoint: "/mnt/home-ai-core/old",
		TotalBytes: 8000, TotalKnown: true, FreeBytes: 6000, FreeKnown: true,
	}}}
	got = nvrCapacityFromInventory(assignments[:1], nodes[:1], inspection)
	if !got.CapacityKnown || got.TotalBytes != 1000 || got.FreeBytes != 500 {
		t.Fatalf("stale helper mount overrode current mount: %+v", got)
	}
	nodes[0].FreeBytes = 2000
	got = nvrCapacityFromInventory(assignments[:1], nodes[:1], storage.Inspection{})
	if got.CapacityKnown {
		t.Fatalf("free > total accepted: %+v", got)
	}
}

func TestNVRStorageEndpointPrivacyAndReadOnlyStatus(t *testing.T) {
	s := &server{
		modules: nvrTestService(true),
		state: fakeState{storagePurposes: []state.StoragePurposeRecord{
			{DevicePath: "/dev/sda1", Purpose: state.StoragePurposeFiles},
			testNVRVolume("nvr", "/dev/sdc1"),
		}},
		nvrStorageInventory: func(context.Context) ([]systeminfo.BlockNode, storage.Inspection) {
			return []systeminfo.BlockNode{testNVRNode("/dev/sdc1", "nvr", "/mnt/home-ai-core/sdc1", 2000, 1500, true)}, storage.Inspection{}
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/storage", nil)
	rec := httptest.NewRecorder()
	s.nvrStorage(rec, req, security.Actor{}, authBearer)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "private, no-store" ||
		strings.Contains(rec.Body.String(), "sda1") {
		t.Fatalf("storage response: %d %s", rec.Code, rec.Body.String())
	}
	var result nvrStorageSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil { t.Fatal(err) }
	if !result.CapacityKnown || result.FreeBytes != 1500 { t.Fatalf("bad JSON: %+v", result) }
	s.modules = nvrTestService(false)
	rec = httptest.NewRecorder()
	s.nvrStorage(rec, req, security.Actor{}, authBearer)
	if rec.Code != http.StatusServiceUnavailable { t.Fatalf("disabled module leaked storage: %d", rec.Code) }
}

func TestNVRStorageRequiresModulesRead(t *testing.T) {
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"system.read"}
	h := uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: nvrTestService(true)})
	if r := uiRequest(h, http.MethodGet, "/api/v1/nvr/storage", ""); r.Code != http.StatusForbidden {
		t.Fatalf("unauthorized disk read: %d", r.Code)
	}
	sec.authErr = security.ErrUnauthorized
	h = uiAPI(sec, &uiJobService{}, uiModuleService{fakeModules: nvrTestService(true)})
	if r := uiRequest(h, http.MethodGet, "/api/v1/nvr/storage", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated disk read: %d", r.Code)
	}
}
