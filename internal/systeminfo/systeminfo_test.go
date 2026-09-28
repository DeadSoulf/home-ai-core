package systeminfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPUModelFromReader(t *testing.T) {
	input := "processor : 0\nmodel name : Example CPU 9000\nprocessor : 1\n"
	if got := cpuModelFromReader(strings.NewReader(input)); got != "Example CPU 9000" {
		t.Fatalf("cpu model = %q", got)
	}
}

func TestMemoryInfoFromReader(t *testing.T) {
	input := "MemTotal:       1024 kB\nMemFree: 10 kB\nMemAvailable: 768 kB\n"
	got := memoryInfoFromReader(strings.NewReader(input))
	if got.TotalBytes != 1024*1024 {
		t.Fatalf("total bytes = %d", got.TotalBytes)
	}
	if got.AvailableBytes != 768*1024 {
		t.Fatalf("available bytes = %d", got.AvailableBytes)
	}
}

func TestBlockDevices(t *testing.T) {
	root := t.TempDir()
	makeBlockDevice(t, root, "sda", "8:0", "Test Disk", "SERIAL-1", "2048", "1", "0")
	makeBlockDevice(t, root, "loop0", "7:0", "", "", "100", "0", "0")

	got := blockDevices(root)
	if len(got) != 1 {
		t.Fatalf("device count = %d, want 1", len(got))
	}
	if got[0].Name != "sda" || got[0].SizeBytes != 2048*512 {
		t.Fatalf("unexpected device: %#v", got[0])
	}
	if !got[0].Rotational || got[0].Removable {
		t.Fatalf("unexpected flags: %#v", got[0])
	}
}

func TestGPUFromCard(t *testing.T) {
	root := t.TempDir()
	card := filepath.Join(root, "card0")
	device := filepath.Join(card, "device")
	if err := os.MkdirAll(device, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(device, "vendor"), "0x10de\n")
	writeTestFile(t, filepath.Join(device, "device"), "0x2684\n")
	writeTestFile(t, filepath.Join(device, "uevent"), "DRIVER=nvidia\nPCI_SLOT_NAME=0000:01:00.0\n")

	got, ok := gpuFromCard(card)
	if !ok {
		t.Fatal("gpu not detected")
	}
	if got.Vendor != "NVIDIA" || got.Driver != "nvidia" || got.PCIAddress != "0000:01:00.0" {
		t.Fatalf("unexpected gpu: %#v", got)
	}
}

func makeBlockDevice(t *testing.T, root, name, dev, model, serial, size, rotational, removable string) {
	t.Helper()
	base := filepath.Join(root, name)
	for _, dir := range []string{filepath.Join(base, "device"), filepath.Join(base, "queue")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(base, "dev"), dev)
	writeTestFile(t, filepath.Join(base, "device", "model"), model)
	writeTestFile(t, filepath.Join(base, "device", "serial"), serial)
	writeTestFile(t, filepath.Join(base, "size"), size)
	writeTestFile(t, filepath.Join(base, "queue", "rotational"), rotational)
	writeTestFile(t, filepath.Join(base, "removable"), removable)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
