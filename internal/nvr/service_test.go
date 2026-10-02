package nvr

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type fakeCredentialStore struct {
	values map[SecretRef]CameraCredential
	next   int
}

func newFakeCredentialStore() *fakeCredentialStore {
	return &fakeCredentialStore{values: map[SecretRef]CameraCredential{}}
}

func (s *fakeCredentialStore) PutCameraCredential(
	_ context.Context,
	_ string,
	credential CameraCredential,
) (SecretRef, error) {
	s.next++
	ref := SecretRef(fmt.Sprintf("sec_test_%d", s.next))
	s.values[ref] = credential
	return ref, nil
}

func (s *fakeCredentialStore) ResolveCameraCredential(
	_ context.Context,
	ref SecretRef,
) (CameraCredential, error) {
	value, ok := s.values[ref]
	if !ok {
		return CameraCredential{}, ErrSecretStoreUnavailable
	}
	return value, nil
}

func (s *fakeCredentialStore) DeleteCameraCredential(_ context.Context, ref SecretRef) error {
	delete(s.values, ref)
	return nil
}

type fakeProber struct {
	result  ProbeResult
	results map[string]ProbeResult
	last    ProbeRequest
	calls   int
}

func (p *fakeProber) Available() bool { return true }

func (p *fakeProber) Probe(_ context.Context, request ProbeRequest) (ProbeResult, error) {
	p.calls++
	p.last = request
	if result, ok := p.results[request.Address]; ok {
		return result, nil
	}
	return p.result, nil
}

func TestCameraOnboardingPersistsOnlySecretReferenceAndKeepsCredentialOnEdit(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	secrets := newFakeCredentialStore()
	prober := &fakeProber{result: ProbeResult{
		Codec:      "h264",
		Width:      1920,
		Height:     1080,
		FPS:        25,
		BitrateBPS: 4_000_000,
		HasAudio:   true,
	}}
	service := NewServiceWithDependencies(store, secrets, prober)
	service.now = func() time.Time {
		return time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)
	}

	camera, probe, err := service.CreateCamera(ctx, "", CameraInput{
		Name:          "Driveway",
		Address:       "rtsp://192.0.2.10/stream1",
		Username:      "viewer",
		Password:      "camera-secret",
		Transport:     "tcp",
		RecordingMode: "off",
		AudioEnabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.Codec != "h264" || camera.Address != "rtsp://192.0.2.10/stream1" {
		t.Fatalf("camera/probe = %#v %#v", camera, probe)
	}

	record, err := store.NVRCamera(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.CredentialRef == "" || record.CredentialRef == "camera-secret" {
		t.Fatalf("credential ref = %q", record.CredentialRef)
	}
	if record.Address != "rtsp://192.0.2.10/stream1" {
		t.Fatalf("address = %q", record.Address)
	}
	if prober.last.Credential.Password != "camera-secret" {
		t.Fatal("create probe did not receive camera credential")
	}

	_, _, err = service.UpdateCamera(ctx, camera.ID, CameraInput{
		Name:          "Driveway updated",
		Address:       "rtsp://192.0.2.10/stream2",
		Transport:     "tcp",
		RecordingMode: "continuous",
		AudioEnabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if prober.last.Credential.Username != "viewer" || prober.last.Credential.Password != "camera-secret" {
		t.Fatalf("update probe did not reuse saved credential: %#v", prober.last.Credential)
	}

	updated, err := store.NVRCamera(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CredentialRef != record.CredentialRef {
		t.Fatalf("credential ref changed unexpectedly: %q -> %q", record.CredentialRef, updated.CredentialRef)
	}
	profiles, err := store.ListNVRStreamProfiles(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].SourceURI != "rtsp://192.0.2.10/stream2" {
		t.Fatalf("profiles = %#v", profiles)
	}

	if err := service.DeleteCamera(ctx, camera.ID); err != nil {
		t.Fatal(err)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("orphaned credentials: %#v", secrets.values)
	}
	if _, err := store.NVRCamera(ctx, camera.ID); err != state.ErrNVRCameraNotFound {
		t.Fatalf("camera still present after delete: %v", err)
	}
}

func TestCameraOnboardingPersistsAndClearsSubstream(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	prober := &fakeProber{
		result: ProbeResult{Codec: "h264", Width: 1920, Height: 1080, FPS: 25},
		results: map[string]ProbeResult{
			"rtsp://192.0.2.20/main": {
				Codec: "h264", Width: 1920, Height: 1080, FPS: 25, BitrateBPS: 4_000_000,
			},
			"rtsp://192.0.2.20/sub": {
				Codec: "h264", Width: 640, Height: 360, FPS: 10, BitrateBPS: 500_000,
			},
		},
	}
	service := NewServiceWithDependencies(store, newFakeCredentialStore(), prober)

	camera, probe, err := service.CreateCamera(ctx, "", CameraInput{
		Name:             "Garage",
		Address:          "rtsp://192.0.2.20/main",
		SubstreamAddress: "rtsp://192.0.2.20/sub",
		Transport:        "tcp",
		RecordingMode:    "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.Substream == nil || probe.Substream.Width != 640 || probe.Substream.Height != 360 {
		t.Fatalf("substream probe = %#v", probe.Substream)
	}
	if camera.SubstreamAddress != "rtsp://192.0.2.20/sub" {
		t.Fatalf("camera substream = %q", camera.SubstreamAddress)
	}

	profiles, err := store.ListNVRStreamProfiles(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || profiles[0].Role != "main" || profiles[1].Role != "sub" {
		t.Fatalf("profiles = %#v", profiles)
	}
	if profiles[1].SourceURI != "rtsp://192.0.2.20/sub" || profiles[1].Width != 640 {
		t.Fatalf("sub profile = %#v", profiles[1])
	}

	updated, probe, err := service.UpdateCamera(ctx, camera.ID, CameraInput{
		Name:          "Garage",
		Address:       "rtsp://192.0.2.20/main",
		Transport:     "tcp",
		RecordingMode: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.SubstreamAddress != "rtsp://192.0.2.20/sub" || probe.Substream == nil {
		t.Fatalf("blank update did not preserve substream: camera=%#v probe=%#v", updated, probe)
	}

	updated, probe, err = service.UpdateCamera(ctx, camera.ID, CameraInput{
		Name:           "Garage",
		Address:        "rtsp://192.0.2.20/main",
		Transport:      "tcp",
		RecordingMode:  "off",
		ClearSubstream: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.SubstreamAddress != "" || probe.Substream != nil {
		t.Fatalf("substream was not cleared: camera=%#v probe=%#v", updated, probe)
	}
	profiles, err = store.ListNVRStreamProfiles(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Role != "main" {
		t.Fatalf("profiles after clear = %#v", profiles)
	}
}

func TestCameraOnboardingRejectsPasswordInsideRTSPURL(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := NewServiceWithDependencies(
		store,
		newFakeCredentialStore(),
		&fakeProber{result: ProbeResult{Codec: "h264"}},
	)
	_, _, err = service.CreateCamera(ctx, "", CameraInput{
		Name:    "Unsafe",
		Address: "rtsp://user:password@192.0.2.10/stream",
	})
	if err == nil {
		t.Fatal("credential-bearing RTSP URL was persisted")
	}
}


type fakeONVIFDiscoverer struct {
	devices []ONVIFDevice
	err     error
}

func (d fakeONVIFDiscoverer) Discover(context.Context) ([]ONVIFDevice, error) {
	return append([]ONVIFDevice(nil), d.devices...), d.err
}

type fakeONVIFClient struct {
	profiles []ONVIFProfile
	err      error
	calls    int
	address  string
	cred     CameraCredential
}

func (c *fakeONVIFClient) Profiles(
	_ context.Context,
	address string,
	credential CameraCredential,
) ([]ONVIFProfile, error) {
	c.calls++
	c.address = address
	c.cred = credential
	return append([]ONVIFProfile(nil), c.profiles...), c.err
}

func TestONVIFDiscoveryAndImportPersistsSelectedProfiles(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	secrets := newFakeCredentialStore()
	onvif := &fakeONVIFClient{profiles: []ONVIFProfile{
		{
			Token: "main-token", Name: "Main", StreamURI: "rtsp://192.168.1.40/main",
			Codec: "h264", Width: 1920, Height: 1080, FPS: 25, BitrateBPS: 4_000_000, HasAudio: true,
		},
		{
			Token: "sub-token", Name: "Sub", StreamURI: "rtsp://192.168.1.40/sub",
			Codec: "h264", Width: 640, Height: 360, FPS: 10, BitrateBPS: 500_000,
		},
	}}
	prober := &fakeProber{
		results: map[string]ProbeResult{
			"rtsp://192.168.1.40/main": {
				Codec: "h264", Width: 1920, Height: 1080, FPS: 25, BitrateBPS: 4_000_000, HasAudio: true,
			},
			"rtsp://192.168.1.40/sub": {
				Codec: "h264", Width: 640, Height: 360, FPS: 10, BitrateBPS: 500_000,
			},
		},
	}
	discoverer := fakeONVIFDiscoverer{devices: []ONVIFDevice{{
		ID: "onvif_test",
		Name: "Front Camera",
		Address: "http://192.168.1.40/onvif/device_service",
		IP: "192.168.1.40",
	}}}
	service := NewServiceWithONVIFDependencies(store, secrets, prober, discoverer, onvif)

	devices, err := service.DiscoverONVIF(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].IP != "192.168.1.40" {
		t.Fatalf("devices = %#v", devices)
	}

	profiles, err := service.ONVIFProfiles(ctx, ONVIFProfileRequest{
		Address: "http://192.168.1.40/onvif/device_service",
		Username: "viewer",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || onvif.cred.Password != "secret" {
		t.Fatalf("profiles/client = %#v %#v", profiles, onvif)
	}

	camera, probe, err := service.ImportONVIFCamera(ctx, "", ONVIFImportInput{
		Name: "Front ONVIF",
		Address: "http://192.168.1.40/onvif/device_service",
		Username: "viewer",
		Password: "secret",
		MainProfileToken: "main-token",
		SubProfileToken: "sub-token",
		Transport: "tcp",
		RecordingMode: "off",
		AudioEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if camera.SourceType != "onvif" || camera.Address != "rtsp://192.168.1.40/main" {
		t.Fatalf("camera = %#v", camera)
	}
	if camera.SubstreamAddress != "rtsp://192.168.1.40/sub" || probe.Substream == nil {
		t.Fatalf("substream camera/probe = %#v %#v", camera, probe)
	}

	record, err := store.NVRCamera(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.CredentialRef == "" || record.CredentialRef == "secret" {
		t.Fatalf("credential ref = %q", record.CredentialRef)
	}
	meta, err := store.NVRONVIFSource(ctx, camera.ID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.DeviceEndpoint != "http://192.168.1.40/onvif/device_service" ||
		meta.MainProfileToken != "main-token" ||
		meta.SubProfileToken != "sub-token" {
		t.Fatalf("ONVIF metadata = %#v", meta)
	}
	if len(secrets.values) != 1 {
		t.Fatalf("secret store = %#v", secrets.values)
	}
}

func TestONVIFImportRejectsSameMainAndSubProfile(t *testing.T) {
	ctx := context.Background()
	store, err := state.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := NewServiceWithONVIFDependencies(
		store,
		newFakeCredentialStore(),
		&fakeProber{result: ProbeResult{Codec: "h264"}},
		fakeONVIFDiscoverer{},
		&fakeONVIFClient{},
	)
	_, _, err = service.ImportONVIFCamera(ctx, "", ONVIFImportInput{
		Name: "Invalid",
		Address: "http://192.168.1.50/onvif/device_service",
		MainProfileToken: "same",
		SubProfileToken: "same",
	})
	if err == nil {
		t.Fatal("same ONVIF main/sub profile was accepted")
	}
}
