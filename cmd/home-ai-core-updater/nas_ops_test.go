package main

import "testing"

func TestValidateNASRelativePath(t *testing.T) {
	valid := []string{
		"shared/nsf_0123456789abcdef",
		"users/usr_0123456789abcdef/nsf_abcdef0123456789",
	}
	for _, value := range valid {
		got, err := validateNASRelativePath(value)
		if err != nil {
			t.Fatalf("validateNASRelativePath(%q) error = %v", value, err)
		}
		if got != value {
			t.Fatalf("validateNASRelativePath(%q) = %q", value, got)
		}
	}

	invalid := []string{
		"",
		".",
		"../etc",
		"/absolute/path",
		"shared/../escape",
		"shared/not-a-folder-id",
		"users/usr_ok/../../escape",
		"users/not-a-user/nsf_abc",
		"other/nsf_abc",
	}
	for _, value := range invalid {
		if _, err := validateNASRelativePath(value); err == nil {
			t.Fatalf("validateNASRelativePath(%q) accepted unsafe path", value)
		}
	}
}

func TestValidNASID(t *testing.T) {
	if !validNASID("nsf_0123456789abcdef", "nsf_") {
		t.Fatal("valid NAS folder ID rejected")
	}
	if validNASID("nsf_bad/path", "nsf_") {
		t.Fatal("unsafe NAS ID accepted")
	}
	if validNASID("usr_", "usr_") {
		t.Fatal("empty NAS ID suffix accepted")
	}
}


func TestParseDUUsageOutput(t *testing.T) {
	used, ok := parseDUUsageOutput([]byte("123456\t/mnt/home-ai-core/sdb1/.home-ai/shared/nsf_test\n"))
	if !ok {
		t.Fatal("expected du output to parse")
	}
	if used != 123456 {
		t.Fatalf("used = %d, want 123456", used)
	}
}

func TestParseDUUsageOutputRejectsInvalidData(t *testing.T) {
	if used, ok := parseDUUsageOutput([]byte("invalid output")); ok {
		t.Fatalf("unexpected parsed usage: %d", used)
	}
}
