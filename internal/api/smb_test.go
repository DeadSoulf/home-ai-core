package api

import (
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

func TestSMBSystemUsernameIsStableAndSafe(t *testing.T) {
	first := smbSystemUsername("usr_0123456789abcdef")
	second := smbSystemUsername("usr_0123456789abcdef")
	if first != second {
		t.Fatalf("SMB username is not stable: %q != %q", first, second)
	}
	if len(first) > 31 || len(first) < 3 {
		t.Fatalf("SMB username length = %d: %q", len(first), first)
	}
	for _, r := range first {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		t.Fatalf("unsafe rune %q in SMB username %q", r, first)
	}
}

func TestSMBShareNameIsStableAndASCII(t *testing.T) {
	record := state.NASFolderRecord{
		ID:   "nsf_0123456789abcdef",
		Name: "Family Фото",
	}
	got := smbShareName(record)
	want := "HA_Family_89abcdef"
	if got != want {
		t.Fatalf("smbShareName() = %q, want %q", got, want)
	}
}
