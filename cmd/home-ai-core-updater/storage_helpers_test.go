package main

import (
	"strings"
	"testing"
)

func TestLVMMapperNameEscapesHyphens(t *testing.T) {
	if got, want := lvmMapperName("vg-main", "data-fast"), "vg--main-data--fast"; got != want {
		t.Fatalf("lvmMapperName() = %q, want %q", got, want)
	}
}

func TestFilesystemDiagnosticCommandForExt4IsReadOnly(t *testing.T) {
	command, args := filesystemDiagnosticCommand("/dev/sdb1", "ext4")
	if command != "/usr/sbin/e2fsck" {
		t.Fatalf("command = %q, want /usr/sbin/e2fsck", command)
	}
	got := strings.Join(args, " ")
	if got != "-n /dev/sdb1" {
		t.Fatalf("args = %q, want read-only e2fsck", got)
	}
}

func TestFilesystemDiagnosticCommandRejectsUnknownFilesystem(t *testing.T) {
	command, args := filesystemDiagnosticCommand("/dev/sdb1", "unknown")
	if command != "" || args != nil {
		t.Fatalf("command = %q args = %#v, want no diagnostic command", command, args)
	}
}

func TestCompactFilesystemDiagnosticNormalizesAndBoundsOutput(t *testing.T) {
	got := compactFilesystemDiagnostic("line one\n\n line two\tline three")
	if got != "line one line two line three" {
		t.Fatalf("normalized = %q", got)
	}
	long := compactFilesystemDiagnostic(strings.Repeat("x", 4000))
	if len(long) > 3503 || !strings.HasSuffix(long, "…") {
		t.Fatalf("bounded diagnostic length = %d, suffix=%q", len(long), long[len(long)-3:])
	}
}
