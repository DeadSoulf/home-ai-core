package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"unsafe"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func TestProjectQuotaKernelABI(t *testing.T) {
	if unsafe.Sizeof(projectQuota{}) != 72 || unsafe.Sizeof(projectAttributes{}) != 28 {
		t.Fatal("project quota structures do not match the Linux kernel ABI")
	}
}

// Opt-in only: the caller supplies a disposable ext4/XFS loop filesystem.
// This test never creates, formats or mounts a device itself.
func TestNASQuotaKernelEnforcement(t *testing.T) {
	root := os.Getenv("HOME_AI_QUOTA_TEST_ROOT")
	if root == "" {
		t.Skip("requires an explicitly prepared disposable filesystem")
	}
	if os.Geteuid() != 0 {
		t.Fatal("kernel quota test requires root")
	}
	if !filepath.IsAbs(root) || filepath.Dir(root) != nasMountRoot || !stringsHasQuotaTestPrefix(filepath.Base(root)) {
		t.Fatal("quota test root must be a dedicated home-ai-quota-test.* mount")
	}
	ctx := context.Background()
	const nasUID, nasGID = 65534, 65534
	relative := "shared/nsf_kernel"
	request := updaterhelper.Request{RootPath: root, RelativePath: relative, ProjectID: 1000001, QuotaBytes: 4 << 20}
	request.Operation = "storage.nas.prepare_pool"
	if _, err := performNASOperation(ctx, request, nasUID, nasGID); err != nil {
		t.Fatal(err)
	}
	request.Operation = "storage.nas.prepare_folder"
	if _, err := performNASOperation(ctx, request, nasUID, nasGID); err != nil {
		t.Fatal(err)
	}
	if _, err := setNASQuota(ctx, request, nasUID, nasGID); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(root, ".home-ai", relative)
	write := func(name string, mebibytes int) error {
		command := exec.CommandContext(ctx, "/usr/bin/dd", "if=/dev/zero", "of="+filepath.Join(folder, name), "bs=1M", "count="+strconv.Itoa(mebibytes), "conv=fsync", "status=none")
		command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: nasUID, Gid: nasGID}}
		return command.Run()
	}
	if err := write("small", 1); err != nil {
		t.Fatal(err)
	}
	if err := write("too-large", 8); err == nil {
		t.Fatalf("kernel allowed oversized project write: %v", err)
	}
	quota, err := inspectNASQuota(ctx, request)
	if err != nil || quota.BlockHardLimit != 4096 || quota.CurrentSpace > 4<<20 {
		t.Fatalf("quota readback: %+v %v", quota, err)
	}
	if err := os.Remove(filepath.Join(folder, "too-large")); err != nil {
		t.Fatal(err)
	}
	request.QuotaBytes = 0
	if _, err := setNASQuota(ctx, request, nasUID, nasGID); err != nil {
		t.Fatal(err)
	}
	if err := write("unlimited", 8); err != nil {
		t.Fatal("removing cap did not restore writes", err)
	}
}

func stringsHasQuotaTestPrefix(value string) bool {
	const prefix = "home-ai-quota-test."
	return len(value) > len(prefix) && value[:len(prefix)] == prefix
}
