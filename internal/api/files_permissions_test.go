package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestFileFolderRootRepairsPermissionDeniedOnce(t *testing.T) {
	originalResolve, originalPrepare := resolveFileFolderRoot, prepareFileFolder
	t.Cleanup(func() {
		resolveFileFolderRoot, prepareFileFolder = originalResolve, originalPrepare
	})
	folder := state.NASFolderRecord{
		PoolRoot:     "/mnt/home-ai-core/sdb1",
		RelativePath: "users/usr_test/nsf_test",
	}
	expectedRoot := "/mnt/home-ai-core/sdb1/.home-ai/users/usr_test/nsf_test"
	resolveCalls, repairCalls := 0, 0
	resolveFileFolderRoot = func(pool, relative string) (string, error) {
		resolveCalls++
		if pool != folder.PoolRoot || relative != folder.RelativePath {
			t.Fatalf("unexpected folder identity %q %q", pool, relative)
		}
		if resolveCalls == 1 {
			return "", fmt.Errorf("inspect logical folder root: %w", os.ErrPermission)
		}
		return expectedRoot, nil
	}
	prepareFileFolder = func(_ context.Context, pool, relative string) error {
		repairCalls++
		if pool != folder.PoolRoot || relative != folder.RelativePath {
			t.Fatalf("repair targeted another folder: %q %q", pool, relative)
		}
		return nil
	}

	root, err := fileFolderRootWithRepair(context.Background(), folder)
	if err != nil || root != expectedRoot {
		t.Fatalf("fileFolderRootWithRepair() = %q, %v", root, err)
	}
	if resolveCalls != 2 || repairCalls != 1 {
		t.Fatalf("resolve=%d repair=%d, want 2 and 1", resolveCalls, repairCalls)
	}
}

func TestFileFolderRootDoesNotRepairOtherErrors(t *testing.T) {
	originalResolve, originalPrepare := resolveFileFolderRoot, prepareFileFolder
	t.Cleanup(func() {
		resolveFileFolderRoot, prepareFileFolder = originalResolve, originalPrepare
	})
	folder := state.NASFolderRecord{PoolRoot: "/mnt/home-ai-core/sdb1", RelativePath: "shared/nsf_test"}
	repairCalls := 0
	resolveFileFolderRoot = func(_, _ string) (string, error) {
		return "", fmt.Errorf("inspect logical folder root: %w", os.ErrNotExist)
	}
	prepareFileFolder = func(_ context.Context, _, _ string) error {
		repairCalls++
		return nil
	}

	if _, err := fileFolderRootWithRepair(context.Background(), folder); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want original not-exist error, got %v", err)
	}
	if repairCalls != 0 {
		t.Fatalf("unexpected filesystem mutation on not-exist: %d calls", repairCalls)
	}
}

func TestFileFolderRootReportsRepairFailure(t *testing.T) {
	originalResolve, originalPrepare := resolveFileFolderRoot, prepareFileFolder
	t.Cleanup(func() {
		resolveFileFolderRoot, prepareFileFolder = originalResolve, originalPrepare
	})
	folder := state.NASFolderRecord{PoolRoot: "/mnt/home-ai-core/sdb1", RelativePath: "users/usr_test/nsf_test"}
	resolveCalls := 0
	resolveFileFolderRoot = func(_, _ string) (string, error) {
		resolveCalls++
		return "", os.ErrPermission
	}
	repairError := errors.New("NAS helper refused directory repair")
	prepareFileFolder = func(_ context.Context, _, _ string) error { return repairError }
	if _, err := fileFolderRootWithRepair(context.Background(), folder); !errors.Is(err, repairError) {
		t.Fatalf("want helper failure, got %v", err)
	}
	if resolveCalls != 1 {
		t.Fatalf("unexpected retry when helper fails: %d calls", resolveCalls)
	}
}

func TestFileFolderRootHealthyPathDoesNotMutate(t *testing.T) {
	originalResolve, originalPrepare := resolveFileFolderRoot, prepareFileFolder
	t.Cleanup(func() {
		resolveFileFolderRoot, prepareFileFolder = originalResolve, originalPrepare
	})
	folder := state.NASFolderRecord{PoolRoot: "/mnt/home-ai-core/sdb1", RelativePath: "shared/nsf_test"}
	resolveFileFolderRoot = func(_, _ string) (string, error) {
		return "/mnt/home-ai-core/sdb1/.home-ai/shared/nsf_test", nil
	}
	prepareFileFolder = func(_ context.Context, _, _ string) error {
		t.Fatal("healthy folder must not trigger repair")
		return nil
	}
	if _, err := fileFolderRootWithRepair(context.Background(), folder); err != nil {
		t.Fatal(err)
	}
}
