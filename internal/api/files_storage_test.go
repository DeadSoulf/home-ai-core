package api

import "testing"

func TestMountBoundaryMatchesDevice(t *testing.T) {
	tests := []struct {
		name      string
		rootDev   uint64
		parentDev uint64
		blockDev  uint64
		want      bool
	}{
		{
			name:      "mounted block device at boundary",
			rootDev:   2049,
			parentDev: 2048,
			blockDev:  2049,
			want:      true,
		},
		{
			name:      "subdirectory on same mounted filesystem",
			rootDev:   2049,
			parentDev: 2049,
			blockDev:  2049,
			want:      false,
		},
		{
			name:      "different backing device",
			rootDev:   2050,
			parentDev: 2048,
			blockDev:  2049,
			want:      false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mountBoundaryMatchesDevice(test.rootDev, test.parentDev, test.blockDev); got != test.want {
				t.Fatalf("mountBoundaryMatchesDevice(%d, %d, %d) = %v, want %v", test.rootDev, test.parentDev, test.blockDev, got, test.want)
			}
		})
	}
}
