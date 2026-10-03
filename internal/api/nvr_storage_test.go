package api

import (
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

func TestPreferredNVRMountpointRejectsUnsafeRoots(t *testing.T) {
	got := preferredNVRMountpoint([]string{"", "/", ".", "relative", "/mnt/video"})
	if got != "/mnt/video" {
		t.Fatalf("mountpoint = %q, want /mnt/video", got)
	}
}

func TestNVRStorageResponseRequiresMountedVideoDevice(t *testing.T) {
	record := state.StoragePurposeRecord{
		DevicePath:     "/dev/sdb1",
		FilesystemUUID: "video-uuid",
		Purpose:        state.StoragePurposeVideo,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	nodes := []systeminfo.BlockNode{{
		Path:        "/dev/sdb1",
		Type:        "part",
		UUID:        "video-uuid",
		Filesystem:  "ext4",
		Label:       "Video",
		SizeBytes:   1_000_000,
		FreeBytes:   600_000,
		FreeKnown:   true,
		Mountpoints: []string{"/mnt/video"},
	}}

	response := nvrStorageResponseFor(record, nodes)
	if !response.Ready || response.Mountpoint != "/mnt/video" {
		t.Fatalf("storage response = %#v", response)
	}
	if response.DevicePath != "/dev/sdb1" || response.FilesystemUUID != "video-uuid" {
		t.Fatalf("storage identity = %#v", response)
	}
}

func TestSameStorageIdentityPrefersFilesystemUUID(t *testing.T) {
	if !sameStorageIdentity("/dev/sdb1", "same", "/dev/mapper/video", "same") {
		t.Fatal("same filesystem UUID was not recognized")
	}
	if sameStorageIdentity("/dev/sdb1", "one", "/dev/sdb1", "two") {
		t.Fatal("different filesystem UUIDs matched only because device paths matched")
	}
	if !sameStorageIdentity("/dev/sdb1", "", "/dev/sdb1", "") {
		t.Fatal("device fallback identity did not match")
	}
}
