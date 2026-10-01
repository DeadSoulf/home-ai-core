package api

import (
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func TestMountInfoDeviceForPath(t *testing.T) {
	data := []byte(
		"34 23 8:17 / /mnt/home-ai-core/sdb1 rw,relatime - ext4 /dev/sdb1 rw\n" +
			"35 23 8:18 / /mnt/home-ai-core/other rw,relatime - ext4 /dev/sdb2 rw\n",
	)

	major, minor, ok := mountInfoDeviceForPath(data, "/mnt/home-ai-core/sdb1")
	if !ok {
		t.Fatal("mount point was not found")
	}
	if major != 8 || minor != 17 {
		t.Fatalf("device = %d:%d, want 8:17", major, minor)
	}
}

func TestMountInfoDeviceForPathRequiresExactMountPoint(t *testing.T) {
	data := []byte("34 23 8:17 / /mnt/home-ai-core/sdb1 rw,relatime - ext4 /dev/sdb1 rw\n")

	if _, _, ok := mountInfoDeviceForPath(data, "/mnt/home-ai-core/sdb1/data"); ok {
		t.Fatal("subdirectory must not be accepted as a mount point")
	}
}

func TestMountInfoDeviceForPathDecodesEscapes(t *testing.T) {
	data := []byte("34 23 8:17 / /mnt/home-ai-core/data\\040disk rw,relatime - ext4 /dev/sdb1 rw\n")

	major, minor, ok := mountInfoDeviceForPath(data, "/mnt/home-ai-core/data disk")
	if !ok || major != 8 || minor != 17 {
		t.Fatalf("escaped mount point = %d:%d ok=%v, want 8:17 true", major, minor, ok)
	}
}

func TestFilePoolCapacityFallsBackToBackingStorageInventory(t *testing.T) {
	record := state.NASPoolRecord{
		StorageDevicePath:     "/dev/sdb1",
		StorageFilesystemUUID: "fs-test-uuid",
	}
	nodes := []systeminfo.BlockNode{
		{
			Path: "/dev/sdb",
			Type: "disk",
			Children: []systeminfo.BlockNode{
				{
					Path:       "/dev/sdb1",
					Type:       "part",
					Filesystem: "ext4",
					UUID:       "fs-test-uuid",
					SizeBytes:  1_000,
					FreeKnown:  false,
				},
			},
		},
	}
	inspection := storage.Inspection{
		Filesystems: []updaterhelper.FilesystemStat{
			{
				Device:     "/dev/sdb1",
				Filesystem: "ext4",
				FreeBytes:  640,
				FreeKnown:  true,
			},
		},
	}

	capacity, ok := filePoolCapacityFromInventory(record, nodes, inspection)
	if !ok {
		t.Fatal("expected backing-storage capacity fallback to succeed")
	}
	if capacity.TotalBytes != 1_000 || capacity.FreeBytes != 640 {
		t.Fatalf("capacity = %#v, want total=1000 free=640", capacity)
	}
}

func TestFilePoolCapacityPrefersHelperFilesystemTotals(t *testing.T) {
	record := state.NASPoolRecord{
		StorageDevicePath:     "/dev/sdb1",
		StorageFilesystemUUID: "fs-test-uuid",
	}
	nodes := []systeminfo.BlockNode{
		{
			Path:       "/dev/sdb1",
			Type:       "part",
			Filesystem: "ext4",
			UUID:       "fs-test-uuid",
			SizeBytes:  1_000,
			FreeBytes:  100,
			FreeKnown:  true,
		},
	}
	inspection := storage.Inspection{
		Filesystems: []updaterhelper.FilesystemStat{
			{
				Device:     "/dev/sdb1",
				Filesystem: "ext4",
				TotalBytes: 960,
				TotalKnown: true,
				FreeBytes:  640,
				FreeKnown:  true,
			},
		},
	}

	capacity, ok := filePoolCapacityFromInventory(record, nodes, inspection)
	if !ok {
		t.Fatal("expected helper filesystem capacity to be used")
	}
	if capacity.TotalBytes != 960 || capacity.FreeBytes != 640 {
		t.Fatalf("capacity = %#v, want total=960 free=640", capacity)
	}
}

func TestFilePoolCapacityFallbackRequiresKnownFreeSpace(t *testing.T) {
	record := state.NASPoolRecord{
		StorageDevicePath:     "/dev/sdb1",
		StorageFilesystemUUID: "fs-test-uuid",
	}
	nodes := []systeminfo.BlockNode{
		{
			Path:       "/dev/sdb1",
			Type:       "part",
			Filesystem: "ext4",
			UUID:       "fs-test-uuid",
			SizeBytes:  1_000,
			FreeKnown:  false,
		},
	}

	if capacity, ok := filePoolCapacityFromInventory(record, nodes, storage.Inspection{}); ok {
		t.Fatalf("unexpected capacity fallback success: %#v", capacity)
	}
}
