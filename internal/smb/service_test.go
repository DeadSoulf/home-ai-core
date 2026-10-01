package smb

import "testing"

func TestHelperSharesCarryPoolCapacityPolicy(t *testing.T) {
	got := helperShares([]Share{{
		Name:           "HA_Family_12345678",
		Path:           "/mnt/home-ai-core/data/.home-ai/shared/nsf_test",
		PoolRoot:       "/mnt/home-ai-core/data",
		ReservePercent: 7,
		ReadUsers:      []string{"hai_reader", "hai_reader"},
		WriteUsers:     []string{"hai_writer"},
	}})
	if len(got) != 1 {
		t.Fatalf("share count = %d, want 1", len(got))
	}
	share := got[0]
	if share.PoolRoot != "/mnt/home-ai-core/data" || share.ReservePercent != 7 {
		t.Fatalf("pool policy was not forwarded: %#v", share)
	}
	if len(share.ReadUsers) != 1 || share.ReadUsers[0] != "hai_reader" {
		t.Fatalf("read users were not normalized: %#v", share.ReadUsers)
	}
}
