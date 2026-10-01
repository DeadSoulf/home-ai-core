package filedata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsageIncludesTrashAndUploadReservations(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	initial, err := UsageOf(root)
	if err != nil || initial.UsedBytes != 5 {
		t.Fatalf("initial %+v %v", initial, err)
	}
	session, err := CreateUpload(root, "new.txt", 100, "", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AppendUploadChunk(root, session.ID, 0, "", strings.NewReader("hello"), 8<<20, time.Now()); err != nil {
		t.Fatal(err)
	}
	usage, err := UsageOf(root)
	if err != nil || usage.UsedBytes != 5 || usage.ReservedBytes != 100 {
		t.Fatalf("reservation double count %+v %v", usage, err)
	}
	trash, err := Trash(root, "a.txt", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	usage, err = UsageOf(root)
	if err != nil || usage.UsedBytes < 5 || usage.ReservedBytes != 100 {
		t.Fatalf("trash escaped quota %+v %v", usage, err)
	}
	if _, err := PurgeTrash(root, trash.ID); err != nil {
		t.Fatal(err)
	}
	if err := CancelUpload(root, session.ID); err != nil {
		t.Fatal(err)
	}
	usage, err = UsageOf(root)
	if err != nil || usage.UsedBytes != 0 || usage.ReservedBytes != 0 {
		t.Fatalf("purge/cancel %+v %v", usage, err)
	}
}
