package cameras

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestInstallWebSDKAndReadAssets(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	files := map[string]string{
		"WebSDK V3.3.1/demo/jquery-1.7.1.min.js":                 "window.jQuery={};",
		"WebSDK V3.3.1/demo/codebase/webVideoCtrl.js":            "window.WebVideoCtrl={version:'3.3.1'};",
		"WebSDK V3.3.1/demo/codebase/jsVideoPlugin-1.0.0.min.js": "window.JSVideoPlugin=function(){};",
		"WebSDK V3.3.1/demo/codebase/HCWebSDKPlugin.exe":         "MZ-test-plugin",
	}
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	service := &Service{
		runtime:     &fakeSDKRuntime{},
		runtimeRoot: t.TempDir(),
		webSDKRoot:  t.TempDir(),
	}
	result, err := service.InstallWebSDK(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Installed || !result.Status.Available || result.Version != "3.3.1" {
		t.Fatalf("install result = %#v", result)
	}
	status := service.Status()
	if !status.WebSDK.Available || status.WebSDK.Version != "3.3.1" {
		t.Fatalf("status = %#v", status.WebSDK)
	}

	asset, err := service.WebSDKAsset("webVideoCtrl.js")
	if err != nil {
		t.Fatal(err)
	}
	if asset.ContentType != "application/javascript; charset=utf-8" || !bytes.Contains(asset.Data, []byte("WebVideoCtrl")) {
		t.Fatalf("asset = %#v", asset)
	}
	plugin, err := service.WebSDKAsset("HCWebSDKPlugin.exe")
	if err != nil {
		t.Fatal(err)
	}
	if !plugin.Download || !bytes.HasPrefix(plugin.Data, []byte("MZ")) {
		t.Fatalf("plugin = %#v", plugin)
	}
}

func TestInstallWebSDKRejectsIncompleteArchive(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("demo/codebase/webVideoCtrl.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("test")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	service := &Service{runtime: &fakeSDKRuntime{}, webSDKRoot: t.TempDir()}
	if _, err := service.InstallWebSDK(bytes.NewReader(archive.Bytes()), int64(archive.Len())); err == nil {
		t.Fatal("expected incomplete WebSDK archive to be rejected")
	}
}
