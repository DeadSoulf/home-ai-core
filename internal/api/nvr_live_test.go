package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func TestNVRVersionGateForArchiveSafeCameraRemoval(t *testing.T) {
	for _, tc := range []struct { version string; supported bool }{
		{"1.1.2",false}, {"1.1.1",false}, {"1.0.9",false},
		{"1.1.3",true}, {"1.1.4",true}, {"1.2.0",true},
		{"2.0.0",true}, {"unknown",false}, {"1.1.3-malformed",true},
	} {
		if got:=nvrSupportsSavedCameraActions(tc.version);got!=tc.supported {
			t.Errorf("version %q -> %v expected %v",tc.version,got,tc.supported)
		}
	}
}

func TestNVRLiveStartUsesSavedCameraWithoutCredentialsInBrowser(t *testing.T) {
	s:=nvrTestUpstream(t, `{"ok":true,"key":"saved_cam1","playlist_url":"/output/hls/saved_cam1/index.m3u8"}`)
	// Fixture ships 1.1.2; only the new tested module permits mutation.
	mod:=nvrTestService(true)
	mod.items[0].Manifest.Version="1.1.3"
	s.modules=mod
	request:=httptest.NewRequest(http.MethodPost,"/api/v1/nvr/cameras/cam1/live/start",strings.NewReader("{}"))
	request.SetPathValue("cameraID","cam1")
	request.SetPathValue("action","start")
	response:=httptest.NewRecorder()
	s.security=defaultFakeSecurity()
	s.nvrSavedLive(response,request,defaultFakeSecurity().actor,authBearer)
	if response.Code!=http.StatusOK ||
		!strings.Contains(response.Body.String(),"/api/v1/nvr/output/hls/saved_cam1/index.m3u8") {
		t.Fatalf("unsafe HLS response: %d %s",response.Code,response.Body.String())
	}
}

func TestNVRDeleteCameraRetainsArchiveAndRejectsOldImage(t *testing.T) {
	s:=nvrTestUpstream(t,`{"ok":true,"deleted":true}`)
	s.security=defaultFakeSecurity()
	req:=httptest.NewRequest(http.MethodDelete,"/api/v1/nvr/cameras/cam1",nil)
	req.SetPathValue("cameraID","cam1")
	rec:=httptest.NewRecorder()
	s.nvrDeleteCamera(rec,req,defaultFakeSecurity().actor,authBearer)
	if rec.Code!=http.StatusConflict {
		t.Fatalf("old NVR delete must be rejected, got %d %s",rec.Code,rec.Body.String())
	}
	module:=nvrTestService(true)
	module.items[0].Manifest.Version="1.1.3"
	s.modules=module
	rec=httptest.NewRecorder()
	s.nvrDeleteCamera(rec,req,defaultFakeSecurity().actor,authBearer)
	if rec.Code!=http.StatusOK || !strings.Contains(rec.Body.String(),`"recordings_preserved":true`) {
		t.Fatalf("safe deletion failed: %d %s",rec.Code,rec.Body.String())
	}
}

func TestNVRRoutesEnforceManagerPermissionsAndCSRF(t *testing.T) {
	sec:=defaultFakeSecurity()
	sec.actor.Permissions=[]string{"modules.read"}
	mod:=nvrTestService(true)
	mod.items[0].Manifest.Version="1.1.3"
	readOnly:=uiAPI(sec,&uiJobService{},uiModuleService{fakeModules:mod})
	for _,tc:=range []struct{ method,url string }{
		{"POST","/api/v1/nvr/cameras/cam1/live/start"},
		{"DELETE","/api/v1/nvr/cameras/cam1"},
	} {
		req:=httptest.NewRequest(tc.method,tc.url,strings.NewReader("{}"))
		req.Header.Set("Authorization","Bearer test")
		rec:=httptest.NewRecorder()
		readOnly.ServeHTTP(rec,req)
		if rec.Code!=http.StatusForbidden { t.Fatalf("readonly actor allowed %s %s: %d",tc.method,tc.url,rec.Code) }
	}
	s:=nvrTestUpstream(t,`{"ok":true,"key":"saved_cam1","playlist_url":"/output/hls/saved_cam1/index.m3u8"}`)
	s.modules=mod
	s.security=defaultFakeSecurity()
	for _,tc:=range []struct{ action string; delete bool }{
		{"start",false},{"",true},
	}{
		req:=httptest.NewRequest(http.MethodPost,"/api/v1/nvr/cameras/cam1/live/start",nil)
		req.SetPathValue("cameraID","cam1")
		req.SetPathValue("action",tc.action)
		rec:=httptest.NewRecorder()
		if tc.delete{s.nvrDeleteCamera(rec,req,security.Actor{},authCookie)}else{s.nvrSavedLive(rec,req,security.Actor{},authCookie)}
		if rec.Code!=403 {t.Fatalf("CSRF bypassed: %d",rec.Code)}
	}
}

func TestNVRLiveRejectsHostSuppliedPlaylistURL(t *testing.T) {
	s:=nvrTestUpstream(t,`{"ok":true,"key":"saved_cam1","playlist_url":"http://169.254.169.254/latest/meta-data"}`)
	mod:=nvrTestService(true)
	mod.items[0].Manifest.Version="1.1.3"
	s.modules=mod
	s.security=defaultFakeSecurity()
	req:=httptest.NewRequest("POST","/api/v1/nvr/cameras/cam1/live/start",nil)
	req.SetPathValue("cameraID","cam1")
	req.SetPathValue("action","start")
	rec:=httptest.NewRecorder()
	s.nvrSavedLive(rec,req,defaultFakeSecurity().actor,authBearer)
	if rec.Code!=http.StatusBadGateway {t.Fatalf("unsafe URL accepted %d",rec.Code)}
}

