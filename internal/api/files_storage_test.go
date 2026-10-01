package api

import "testing"

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
