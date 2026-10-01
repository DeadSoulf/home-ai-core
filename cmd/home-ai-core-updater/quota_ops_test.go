package main

import (
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

func TestHasUserQuotaMountOption(t *testing.T) {
	for _, options := range []string{
		"rw,uquota,attr2",
		"rw,usrquota",
		"rw,quota",
		"RW,UQUOTA",
	} {
		if !hasUserQuotaMountOption(options) {
			t.Fatalf("quota mount option not detected in %q", options)
		}
	}
	for _, options := range []string{"rw", "rw,noquota", "defaults"} {
		if hasUserQuotaMountOption(options) {
			t.Fatalf("false quota mount option detected in %q", options)
		}
	}
}

func TestParseExt4QuotaFeature(t *testing.T) {
	enabled, err := parseExt4QuotaFeature(
		"Filesystem volume name: data\nFilesystem features: has_journal ext_attr resize_inode quota metadata_csum\n",
	)
	if err != nil || !enabled {
		t.Fatalf("quota feature = %v, err = %v", enabled, err)
	}

	enabled, err = parseExt4QuotaFeature(
		"Filesystem features: has_journal ext_attr resize_inode metadata_csum\n",
	)
	if err != nil {
		t.Fatalf("feature parse error = %v", err)
	}
	if enabled {
		t.Fatal("quota feature detected when absent")
	}

	if _, err := parseExt4QuotaFeature("Filesystem volume name: data\n"); err == nil {
		t.Fatal("missing filesystem feature line was accepted")
	}
}

func TestQuotaSafeHardLimitUsesOnlySpaceAboveReserve(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	const kibPerGiB = uint64(1024 * 1024)

	limit, err := quotaSafeHardLimitKiBForCapacity(
		100*gib,
		20*gib,
		50*kibPerGiB,
		5,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Existing Home-AI usage is 50 GiB. There is 20 GiB globally free, but
	// 5 GiB must remain untouched, so Home-AI may grow by only 15 GiB.
	want := uint64(65 * kibPerGiB)
	if limit != want {
		t.Fatalf("hard limit = %d KiB, want %d", limit, want)
	}

	limit, err = quotaSafeHardLimitKiBForCapacity(
		100*gib,
		4*gib,
		50*kibPerGiB,
		5,
	)
	if err != nil {
		t.Fatal(err)
	}
	if limit != 50*kibPerGiB {
		t.Fatalf("reserve-reached hard limit = %d, want current usage", limit)
	}

	limit, err = quotaSafeHardLimitKiBForCapacity(100*gib, 20*gib, 50*kibPerGiB, 0)
	if err != nil || limit != 0 {
		t.Fatalf("disabled reserve limit = %d, err = %v", limit, err)
	}

	if _, err := quotaSafeHardLimitKiBForCapacity(100*gib, 20*gib, 0, 51); err == nil {
		t.Fatal("invalid reserve percentage was accepted")
	}
}

func TestQuotaSafeHardLimitNeverUsesUnlimitedZeroAtReserve(t *testing.T) {
	limit, err := quotaSafeHardLimitKiBForCapacity(1024*1024, 0, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if limit != 1 {
		t.Fatalf("empty reserve-reached limit = %d, want 1", limit)
	}
}

func TestParseUserQuotaCSV(t *testing.T) {
	data := strings.Join([]string{
		"User,SpaceStatus,FileStatus,SpaceUsed,SpaceSoftLimit,SpaceHardLimit,FileUsed,FileSoftLimit,FileHardLimit",
		"998,ok,ok,10,0,0,2,0,0",
		"999,ok,ok,12345,60000,70000,10,0,0",
	}, "\n")

	stat, found, err := parseUserQuotaCSV(data, 999)
	if err != nil {
		t.Fatal(err)
	}
	if !found || stat.UsedKiB != 12345 || stat.HardLimitKiB != 70000 {
		t.Fatalf("parsed quota = %#v found=%v", stat, found)
	}

	_, found, err = parseUserQuotaCSV(data, 1000)
	if err != nil || found {
		t.Fatalf("missing uid result found=%v err=%v", found, err)
	}

	_, _, err = parseUserQuotaCSV("999,ok,ok,bad,0,70000", 999)
	if err == nil {
		t.Fatal("invalid usage was accepted")
	}
}

func TestSMBPoolQuotaPoliciesCollapsePoolsAndRejectConflicts(t *testing.T) {
	shares := []updaterhelper.SMBShareRequest{
		{
			Name:           "A",
			Path:           "/mnt/home-ai-core/data/.home-ai/shared/nsf_a",
			PoolRoot:       "/mnt/home-ai-core/data",
			ReservePercent: 5,
		},
		{
			Name:           "B",
			Path:           "/mnt/home-ai-core/data/.home-ai/users/usr_a/nsf_b",
			PoolRoot:       "/mnt/home-ai-core/data",
			ReservePercent: 5,
		},
		{
			Name:           "C",
			Path:           "/mnt/home-ai-core/archive/.home-ai/shared/nsf_c",
			PoolRoot:       "/mnt/home-ai-core/archive",
			ReservePercent: 10,
		},
	}
	policies, err := smbPoolQuotaPolicies(shares)
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 2 {
		t.Fatalf("policy count = %d, want 2: %#v", len(policies), policies)
	}
	if policies[0].RootPath != "/mnt/home-ai-core/archive" ||
		policies[1].RootPath != "/mnt/home-ai-core/data" {
		t.Fatalf("policies are not deterministic: %#v", policies)
	}

	conflicting := append([]updaterhelper.SMBShareRequest(nil), shares[:2]...)
	conflicting[1].ReservePercent = 7
	if _, err := smbPoolQuotaPolicies(conflicting); err == nil {
		t.Fatal("conflicting pool reserve policies were accepted")
	}

	outside := []updaterhelper.SMBShareRequest{{
		Name:           "Bad",
		Path:           "/mnt/home-ai-core/other/.home-ai/shared/nsf_bad",
		PoolRoot:       "/mnt/home-ai-core/data",
		ReservePercent: 5,
	}}
	if _, err := smbPoolQuotaPolicies(outside); err == nil {
		t.Fatal("share outside its pool root was accepted")
	}

	invalid := []updaterhelper.SMBShareRequest{{
		Name:           "Bad",
		Path:           "/mnt/home-ai-core/data/.home-ai/shared/nsf_bad",
		PoolRoot:       "/mnt/home-ai-core/data",
		ReservePercent: 51,
	}}
	if _, err := smbPoolQuotaPolicies(invalid); err == nil {
		t.Fatal("invalid reserve percentage was accepted")
	}
}
