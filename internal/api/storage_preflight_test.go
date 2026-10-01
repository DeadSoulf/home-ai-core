package api

import "testing"

func TestStorageExecutionModePreflightIsAlwaysDryRun(t *testing.T) {
	operation, dryRun := storageExecutionMode("preflight", false)
	if operation != "partition.delete_all" {
		t.Fatalf("operation = %q, want partition.delete_all", operation)
	}
	if !dryRun {
		t.Fatal("preflight must always force dry-run")
	}
}

func TestStorageExecutionModePreservesNormalOperation(t *testing.T) {
	operation, dryRun := storageExecutionMode("format", false)
	if operation != "format" || dryRun {
		t.Fatalf("mode = (%q, %v), want (format, false)", operation, dryRun)
	}
}
