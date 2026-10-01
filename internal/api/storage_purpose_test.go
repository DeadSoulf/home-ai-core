package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

func TestStoragePurposeResponseSerializesEmptyMountpointsAsArray(t *testing.T) {
	record := state.StoragePurposeRecord{
		DevicePath:     "/dev/sdb1",
		FilesystemUUID: "test-uuid",
		Purpose:        state.StoragePurposeFiles,
	}
	nodes := []systeminfo.BlockNode{{
		Path:        "/dev/sdb1",
		Type:        "part",
		Filesystem:  "ext4",
		UUID:        "test-uuid",
		Mountpoints: nil,
	}}

	response := storagePurposeResponseFor(record, nodes)
	if response.Mountpoints == nil {
		t.Fatal("Mountpoints is nil, want empty slice")
	}
	if len(response.Mountpoints) != 0 {
		t.Fatalf("Mountpoints = %#v, want empty slice", response.Mountpoints)
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"mountpoints":null`) {
		t.Fatalf("storage purpose JSON contains null mountpoints: %s", data)
	}
	if !strings.Contains(string(data), `"mountpoints":[]`) {
		t.Fatalf("storage purpose JSON missing empty mountpoint array: %s", data)
	}
}
