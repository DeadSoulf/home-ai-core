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
	result ProbeResult
	last   ProbeRequest
	calls  int
}

func (p *fakeProber) Available() bool { return true }

func (p *fakeProber) Probe(_ context.Context, request ProbeRequest) (ProbeResult, error) {
	p.calls++
	p.last = request
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
