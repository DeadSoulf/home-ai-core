package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
)

func TestCorePreservesOtherSMBAllocations(t *testing.T) {
	oldInspect, oldCapacity := inspectFolderHardQuota, readFilePoolCapacity
	t.Cleanup(func() { inspectFolderHardQuota, readFilePoolCapacity = oldInspect, oldCapacity })
	inspectFolderHardQuota = func(context.Context, string, uint32) (storage.QuotaStatus, error) {
		return storage.QuotaStatus{LimitBytes: 64 << 20, UsedBytes: 4 << 20}, nil
	}
	readFilePoolCapacity = func(string) (filedata.Capacity, error) {
		return filedata.Capacity{TotalBytes: 1 << 30, FreeBytes: 512 << 20}, nil
	}
	hard := state.NASFolderRecord{ID: "hard", PoolID: "pool", HardQuotaBytes: 64 << 20, ProjectID: 1}
	other := hard
	other.ID = "other"
	other.ProjectID = 2
	allowance, err := reservedPoolAllowance(context.Background(), []state.NASFolderRecord{hard, other}, hard, "", 400<<20)
	if err != nil || allowance != 60<<20 {
		t.Fatalf("own allocation lost: %d %v", allowance, err)
	}
	unbounded := state.NASFolderRecord{ID: "unbounded", PoolID: "pool", PoolRoot: t.TempDir(), RelativePath: "shared/nsf_target"}
	root, err := filedata.FolderRoot(unbounded.PoolRoot, unbounded.RelativePath)
	// FolderRoot requires the provisioned tree.
	if err != nil {
		if err := os.MkdirAll(filepath.Join(unbounded.PoolRoot, ".home-ai", unbounded.RelativePath), 0750); err != nil {
			t.Fatal(err)
		}
		root, err = filedata.FolderRoot(unbounded.PoolRoot, unbounded.RelativePath)
	}
	if err != nil {
		t.Fatal(err)
	}
	upload, err := filedata.CreateUpload(root, "pending", 20<<20, "", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	allowance, err = reservedPoolAllowance(context.Background(), []state.NASFolderRecord{hard, other, unbounded}, unbounded, "", 400<<20)
	if err != nil || allowance != 244<<20 {
		t.Fatalf("Core spent SMB/upload allocation: %d %v", allowance, err)
	}
	allowance, err = reservedPoolAllowance(context.Background(), []state.NASFolderRecord{hard, other, unbounded}, unbounded, upload.ID, 400<<20)
	if err != nil || allowance != 264<<20 {
		t.Fatalf("own pending reservation counted twice: %d %v", allowance, err)
	}
}
