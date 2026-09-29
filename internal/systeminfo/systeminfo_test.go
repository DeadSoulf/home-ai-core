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

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
