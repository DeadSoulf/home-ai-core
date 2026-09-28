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

func TestGPUInventoryUsesPCIIDsAndWorksWithoutDriver(t *testing.T) {
	root := t.TempDir()
	device := filepath.Join(root, "0000:01:00.0")
	if err := os.MkdirAll(device, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(device, "class"), "0x030000\n")
	writeTestFile(t, filepath.Join(device, "vendor"), "0x10de\n")
	writeTestFile(t, filepath.Join(device, "device"), "0x2684\n")
	writeTestFile(t, filepath.Join(device, "modalias"), "pci:v000010DEd00002684sv00000000sd00000000bc03sc00i00\n")

	idsPath := filepath.Join(t.TempDir(), "pci.ids")
	writeTestFile(t, idsPath, "10de  NVIDIA Corporation\n\t2684  AD102 [GeForce RTX 4090]\n")

	got := gpus(root, []string{idsPath})
	if len(got) != 1 {
		t.Fatalf("gpu count = %d, want 1", len(got))
	}
	if got[0].Vendor != "NVIDIA Corporation" || got[0].Model != "AD102 [GeForce RTX 4090]" {
		t.Fatalf("unexpected gpu identity: %#v", got[0])
	}
	if got[0].PCIAddress != "0000:01:00.0" {
		t.Fatalf("unexpected PCI address: %#v", got[0])
	}
	if got[0].Driver != "" {
		t.Fatalf("driver should be empty for an unbound device: %#v", got[0])
	}
	if got[0].Modalias == "" {
		t.Fatalf("modalias was not collected: %#v", got[0])
	}
}

func TestGPUInventoryReadsBoundDriver(t *testing.T) {
	root := t.TempDir()
	device := filepath.Join(root, "0000:02:00.0")
	if err := os.MkdirAll(device, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(device, "class"), "0x030200\n")
	writeTestFile(t, filepath.Join(device, "vendor"), "0x1002\n")
	writeTestFile(t, filepath.Join(device, "device"), "0x744c\n")
	writeTestFile(t, filepath.Join(device, "uevent"), "DRIVER=amdgpu\n")

	got := gpus(root, nil)
	if len(got) != 1 || got[0].Driver != "amdgpu" {
		t.Fatalf("unexpected gpu driver state: %#v", got)
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
