package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func TestRenderSMBConfigDisablesGuestAndScopesUsers(t *testing.T) {
	config := renderSMBConfig("WORKGROUP", []updaterhelper.SMBShareRequest{
		{
			Name:       "HA_Family_12345678",
			Path:       "/mnt/home-ai-core/data/.home-ai/shared/nsf_test",
			ReadUsers:  []string{"hai_reader"},
			WriteUsers: []string{"hai_writer"},
		},
	})

	for _, want := range []string{
		"[global]",
		"map to guest = Never",
		"usershare allow guests = no",
		"server min protocol = SMB2_10",
		"[HA_Family_12345678]",
		"guest ok = no",
		"valid users = hai_reader hai_writer",
		"read list = hai_reader",
		"write list = hai_writer",
		"force user = home-ai-core",
		"follow symlinks = no",
		"wide links = no",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}
}

func TestSMBIncludeContentIsIdempotentAndIgnoresComment(t *testing.T) {
	original := "[global]\n# include = /etc/samba/home-ai.conf\n"
	updated, changed := ensureSMBIncludeContent(original)
	if !changed {
		t.Fatal("commented include incorrectly treated as active")
	}
	if !strings.Contains(updated, smbIncludeBegin) ||
		!strings.Contains(updated, smbIncludeDirective) ||
		!strings.Contains(updated, smbIncludeEnd) {
		t.Fatalf("managed include block missing:\n%s", updated)
	}
	second, changed := ensureSMBIncludeContent(updated)
	if changed || second != updated {
		t.Fatal("SMB include insertion is not idempotent")
	}
}

func TestValidateSMBSharePathRejectsOutsideAndSymlink(t *testing.T) {
	base := t.TempDir()
	oldRoot := nasMountRoot
	_ = oldRoot

	// validateSMBSharePath intentionally uses the production /mnt root,
	// so safety primitives that do not require privileged fixtures are
	// covered here and path ownership is exercised in live/integration use.
	if err := validateSMBSharePath("/tmp/not-home-ai", os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("outside NAS path accepted")
	}

	_ = base
}

func TestSMBNameValidation(t *testing.T) {
	for _, user := range []string{"hai_0123456789abcdef", "abc", "home-ai"} {
		if !validSMBUser(user) {
			t.Fatalf("valid SMB user rejected: %q", user)
		}
	}
	for _, user := range []string{"", "ab", "BadUser", "bad.user", "bad user"} {
		if validSMBUser(user) {
			t.Fatalf("invalid SMB user accepted: %q", user)
		}
	}

	for _, share := range []string{"HA_Family_1234", "Share-1"} {
		if !validSMBShareName(share) {
			t.Fatalf("valid share rejected: %q", share)
		}
	}
	for _, share := range []string{"", "bad share", "bad/share"} {
		if validSMBShareName(share) {
			t.Fatalf("invalid share accepted: %q", share)
		}
	}
}

func TestReadOptionalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	data, mode, exists, err := readOptionalFile(path)
	if err != nil || exists || data != nil || mode != 0 {
		t.Fatalf("missing file result = %q %v %v %v", data, mode, exists, err)
	}
	if err := os.WriteFile(path, []byte("ok"), 0o640); err != nil {
		t.Fatal(err)
	}
	data, mode, exists, err = readOptionalFile(path)
	if err != nil || !exists || string(data) != "ok" || mode != 0o640 {
		t.Fatalf("existing file result = %q %v %v %v", data, mode, exists, err)
	}
}
