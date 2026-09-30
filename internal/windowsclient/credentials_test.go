package windowsclient

import "testing"

func TestCredentialTargetUsesNormalizedServerAndUsername(t *testing.T) {
	first, err := CredentialTarget("HTTP://Example.COM:80/", "alice")
	if err != nil {
		t.Fatal(err)
	}
	second, err := CredentialTarget("http://example.com", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == "" {
		t.Fatalf("credential targets differ: %q vs %q", first, second)
	}
	if _, err := CredentialTarget("http://example.com", " alice "); err == nil {
		t.Fatal("accepted username with surrounding whitespace")
	}
}
