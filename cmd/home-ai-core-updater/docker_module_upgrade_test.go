package main

import "testing"

func TestValidateModuleHealthCheck(t *testing.T) {
	for _, tc := range []struct {
		port int
		path string
		ok   bool
	}{
		{8080, "/health", true},
		{8090, "/api/health", true},
		{0, "/health", false},
		{65536, "/health", false},
		{8080, "", false},
		{8080, "health", false},
		{8080, "//attacker.test/", false},
		{8080, "/../admin", false},
		{8080, "/health?target=x", false},
		{8080, "/health#fragment", false},
		{8080, "/health\nheader", false},
	} {
		err := validateModuleHealthCheck(tc.port, tc.path)
		if (err == nil) != tc.ok {
			t.Errorf("validateModuleHealthCheck(%d, %q) = %v, want valid=%v", tc.port, tc.path, err, tc.ok)
		}
	}
}
