package main

import "testing"

func TestValidateBlankDiskLayout(t *testing.T) {
	cases := []struct {
		name   string
		output string
		valid  bool
	}{
		{"blank SATA", "/dev/sdb disk\n", true},
		{"blank NVMe", "/dev/nvme1n1 disk\n", true},
		{"partition table", "/dev/sdb disk gpt\n", false},
		{"filesystem", "/dev/sdb disk ext4\n", false},
		{"child partition", "/dev/sdb disk\n/dev/sdb1 part\n", false},
		{"different target", "/dev/sdc disk\n", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBlankDiskLayout(tc.output, "/dev/sdb")
			if tc.name == "blank NVMe" {
				err = validateBlankDiskLayout(tc.output, "/dev/nvme1n1")
			}
			if (err == nil) != tc.valid {
				t.Fatalf("validation error = %v, want valid=%v", err, tc.valid)
			}
		})
	}
}
