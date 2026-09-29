package main

import "testing"

func TestLVMMapperNameEscapesHyphens(t *testing.T) {
	if got, want := lvmMapperName("vg-main", "data-fast"), "vg--main-data--fast"; got != want {
		t.Fatalf("lvmMapperName() = %q, want %q", got, want)
	}
}
