package modules

import "testing"

func TestValidateModuleHTTPHealth(t *testing.T) {
	for _, test := range []struct {
		port int
		path string
		valid bool
	}{
		{8080, "/health", true},
		{8090, "/api/health", true},
		{0, "/health", false},
		{65536, "/health", false},
		{8080, "health", false},
		{8080, "//host/", false},
		{8080, "/../admin", false},
		{8080, "/health?x=1", false},
		{8080, "/health#frag", false},
	} {
		err := validateModuleHTTPHealth(test.port, test.path)
		if (err == nil) != test.valid {
			t.Errorf("validateModuleHTTPHealth(%d, %q) = %v; valid=%v", test.port, test.path, err, test.valid)
		}
	}
}
