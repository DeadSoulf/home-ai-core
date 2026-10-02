package nvr

import "testing"

func TestParseSecretRef(t *testing.T) {
	valid, err := ParseSecretRef("sec_camera-01")
	if err != nil {
		t.Fatal(err)
	}
	if valid != SecretRef("sec_camera-01") {
		t.Fatalf("secret ref = %q", valid)
	}

	for _, value := range []string{
		"password",
		"sec_",
		"sec_camera/01",
		"sec_camera 01",
	} {
		if _, err := ParseSecretRef(value); err == nil {
			t.Fatalf("invalid secret reference %q was accepted", value)
		}
	}
}

func TestNVRModuleManifest(t *testing.T) {
	manifest := NewModule().Manifest()
	if manifest.ID != ModuleID || manifest.Version != ModuleVersion {
		t.Fatalf("manifest = %#v", manifest)
	}
	if len(manifest.UI.Navigation) != 1 || manifest.UI.Navigation[0].Route != "/modules/nvr" {
		t.Fatalf("navigation = %#v", manifest.UI.Navigation)
	}
	if len(manifest.Host.Packages) != 1 || manifest.Host.Packages[0] != "ffmpeg" {
		t.Fatalf("packages = %#v", manifest.Host.Packages)
	}
}
