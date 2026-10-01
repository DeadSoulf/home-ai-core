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

func TestMountTargetPresentNormalizesPaths(t *testing.T) {
	if !mountTargetPresent([]string{"/mnt/home-ai-core/sdb1"}, "/mnt/home-ai-core/sdb1/") {
		t.Fatal("expected normalized mount target to match")
	}
	if mountTargetPresent([]string{"/mnt/other"}, "/mnt/home-ai-core/sdb1") {
		t.Fatal("unexpected mount target match")
	}
}

func TestHostMountCommandArgsTargetsPID1MountNamespace(t *testing.T) {
	got := strings.Join(hostMountCommandArgs("/usr/bin/mount", "-o", "usrquota", "--", "/dev/sdb1", "/mnt/home-ai-core/sdb1"), " ")
	want := "--mount=/proc/1/ns/mnt -- /usr/bin/mount -o usrquota -- /dev/sdb1 /mnt/home-ai-core/sdb1"
	if got != want {
		t.Fatalf("hostMountCommandArgs() = %q, want %q", got, want)
	}
}

func TestHostFindmntCommandArgsTargetsPID1MountNamespace(t *testing.T) {
	got := strings.Join(hostMountCommandArgs("/usr/bin/findmnt", "-rn", "-S", "/dev/sdb1", "-o", "TARGET"), " ")
	want := "--mount=/proc/1/ns/mnt -- /usr/bin/findmnt -rn -S /dev/sdb1 -o TARGET"
	if got != want {
		t.Fatalf("host findmnt args = %q, want %q", got, want)
	}
}

func TestHostFilesystemInspectionTargetsPID1MountNamespace(t *testing.T) {
	got := strings.Join(hostMountCommandArgs(
		"/usr/bin/lsblk",
		"--json",
		"--bytes",
		"--output", "PATH,TYPE,FSTYPE,FSAVAIL,MOUNTPOINTS",
	), " ")
	want := "--mount=/proc/1/ns/mnt -- /usr/bin/lsblk --json --bytes --output PATH,TYPE,FSTYPE,FSAVAIL,MOUNTPOINTS"
	if got != want {
		t.Fatalf("host filesystem inspection args = %q, want %q", got, want)
	}
}

func TestParseDFCapacityOutput(t *testing.T) {
	output := []byte("  1B-blocks       Avail\n999653638144 753428987904\n")
	total, free, ok := parseDFCapacityOutput(output)
	if !ok {
		t.Fatal("expected df capacity output to parse")
	}
	if total != 999653638144 || free != 753428987904 {
		t.Fatalf("capacity = %d/%d, want 999653638144/753428987904", total, free)
	}
}

func TestParseDFCapacityOutputRejectsInvalidData(t *testing.T) {
	if total, free, ok := parseDFCapacityOutput([]byte("Size Avail\n- -\n")); ok {
		t.Fatalf("unexpected parsed capacity: %d/%d", total, free)
	}
}


func TestSummarizeDestructivePlanIncludesReadOnlyActions(t *testing.T) {
	got := summarizeDestructivePlan("format as ext4", "/dev/sdb1", destructiveUsagePlan{
		Mounts:       []string{"/dev/sdb1@/mnt/home-ai-core/sdb1"},
		ActiveSwaps:  []string{"/dev/sdb2"},
		VolumeGroups: []string{"vg-data"},
	})
	for _, want := range []string{
		"dry-run OK",
		"system disk check passed",
		"would unmount /dev/sdb1@/mnt/home-ai-core/sdb1",
		"would disable swap /dev/sdb2",
		"would deactivate LVM vg-data",
		"no changes made",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plan %q does not contain %q", got, want)
		}
	}
}
