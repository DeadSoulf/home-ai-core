package coreupdate

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{"0.1.5-dev", "0.1.4-dev", 1},
		{"0.2.0-dev", "0.1.99-dev", 1},
		{"1.0.0", "1.0.0-dev", 1},
		{"0.1.4-dev", "0.1.5-dev", -1},
		{"0.1.5-dev", "0.1.5-dev", 0},
	}
	for _, test := range tests {
		got := compareVersions(test.left, test.right)
		if got < 0 {
			got = -1
		} else if got > 0 {
			got = 1
		}
		if got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}
