package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type uiJobService struct {
	mu      sync.Mutex
	jobs    []state.JobRecord
	listErr error
}

func (s *uiJobService) List(_ context.Context, status string, _ int) ([]state.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]state.JobRecord, 0)
	for _, job := range s.jobs {
		if job.Status == status || status == "" {
			out = append(out, job)
		}
	}
	return out, s.listErr
}
func (s *uiJobService) Submit(_ context.Context, job state.JobRecord) (state.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.ID = "job-ui"
	job.Status = "queued"
	s.jobs = append(s.jobs, job)
	return job, nil
}
func (s *uiJobService) Get(_ context.Context, id string) (state.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.jobs {
		if job.ID == id {
			return job, nil
		}
	}
	return state.JobRecord{}, state.ErrJobNotFound
}
func (s *uiJobService) Cancel(ctx context.Context, id string) (state.JobRecord, error) {
	return s.Get(ctx, id)
}

type uiModuleService struct {
	fakeModules
	failure error
}

func (s uiModuleService) Runtime(context.Context) (modules.RuntimeStatus, error) {
	return modules.RuntimeStatus{Available: true, Active: true, EngineReady: true, NetworkReady: false, CheckedAt: time.Now().UTC()}, s.failure
}
func uiAPI(sec fakeSecurity, jobs *uiJobService, module uiModuleService) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New("ui-test", logger, fakeState{}, sec, jobs, nil, module, nil, realtime.New("ui-test", logger))
}
func uiRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
func TestModuleRuntimeReadPermissionsAndUnavailableStatus(t *testing.T) {
	sec := defaultFakeSecurity()
	jobs := &uiJobService{}
	response := uiRequest(uiAPI(sec, jobs, uiModuleService{}), "GET", "/api/v1/modules/runtime", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"engine_ready":true`) || !strings.Contains(response.Body.String(), `"network_ready":false`) {
		t.Fatalf("runtime response %d %s", response.Code, response.Body.String())
	}
	sec.actor.Permissions = []string{"security.self.read"}
	if got := uiRequest(uiAPI(sec, jobs, uiModuleService{}), "GET", "/api/v1/modules/runtime", ""); got.Code != 403 {
		t.Fatalf("unauthorized status %d", got.Code)
	}
	sec = defaultFakeSecurity()
	sec.authErr = security.ErrUnauthorized
	if got := uiRequest(uiAPI(sec, jobs, uiModuleService{}), "GET", "/api/v1/modules/runtime", ""); got.Code != 401 {
		t.Fatalf("anonymous status %d", got.Code)
	}
	if got := uiRequest(uiAPI(defaultFakeSecurity(), jobs, uiModuleService{failure: errors.New("offline")}), "GET", "/api/v1/modules/runtime", ""); got.Code != 503 {
		t.Fatalf("helper unavailable status %d", got.Code)
	}
}
func TestModuleOperationsExposeOnlyScopedSanitizedRecords(t *testing.T) {
	jobs := &uiJobService{jobs: []state.JobRecord{
		{ID: "job-module", Type: "module.install", Status: "running", Input: map[string]any{"module_id": "demo", "manifest_json": "private manifest"}},
		{ID: "job-other", Type: "storage.format", Status: "running", Input: map[string]any{"device": "private device"}},
		{ID: "job-done", Type: "module.install", Status: "succeeded"},
	}}
	handler := uiAPI(defaultFakeSecurity(), jobs, uiModuleService{})
	response := uiRequest(handler, "GET", "/api/v1/modules/operations", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), "job-module") || !strings.Contains(response.Body.String(), "job-done") || strings.Contains(response.Body.String(), "job-other") || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("operation scope: %s", response.Body.String())
	}
	if response := uiRequest(handler, "GET", "/api/v1/modules/operations/job-other", ""); response.Code != 404 {
		t.Fatalf("other job exposed: %d", response.Code)
	}
	if response := uiRequest(handler, "GET", "/api/v1/modules/operations/job-module", ""); response.Code != 200 || strings.Contains(response.Body.String(), "manifest_json") {
		t.Fatalf("single job scope: %s", response.Body.String())
	}
	sec := defaultFakeSecurity()
	sec.actor.Permissions = []string{"modules.read"}
	if response := uiRequest(uiAPI(sec, jobs, uiModuleService{}), "GET", "/api/v1/modules/operations", ""); response.Code != 403 {
		t.Fatalf("viewer sees management operations: %d", response.Code)
	}
}
func TestModuleInstallDeduplicatesQueueAndRequiresCSRF(t *testing.T) {
	jobs := &uiJobService{}
	handler := uiAPI(defaultFakeSecurity(), jobs, uiModuleService{})
	manifest := modules.Manifest{SchemaVersion: 2, ID: "demo", Name: "Demo", Version: "1.0.0", Core: ">=0.1.0", Runtime: modules.RuntimeSpec{Type: "docker", Docker: modules.DockerSpec{Image: "example.com/demo@sha256:" + strings.Repeat("a", 64)}}, Capabilities: modules.Capabilities{Requires: []string{"host.docker"}}, Lifecycle: []string{"install"}}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if response := uiRequest(handler, "POST", "/api/v1/modules/install", string(raw)); response.Code != 202 {
		t.Fatalf("initial install: %d %s", response.Code, response.Body.String())
	}
	response := uiRequest(handler, "POST", "/api/v1/modules/install", string(raw))
	if response.Code != 409 || !strings.Contains(response.Body.String(), "module_operation_in_progress") || len(jobs.jobs) != 1 {
		t.Fatalf("duplicate created: %d %s", response.Code, response.Body.String())
	}
	request := httptest.NewRequest("POST", "/api/v1/modules/install", strings.NewReader(string(raw)))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "test"})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("missing CSRF accepted: %d", response.Code)
	}
}
func TestConcurrentModuleSubmissionsAcceptOnlyOne(t *testing.T) {
	jobs := &uiJobService{}
	s := &server{jobs: jobs}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.submitModuleJob(context.Background(), state.JobRecord{Type: "module.install"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	accepted := 0
	for err := range errs {
		if err == nil {
			accepted++
		} else if !errors.Is(err, errModuleOperationInProgress) {
			t.Fatal(err)
		}
	}
	if accepted != 1 || len(jobs.jobs) != 1 {
		t.Fatalf("accepted=%d records=%d", accepted, len(jobs.jobs))
	}
}
func TestModuleSubmissionFailsClosedIfJobsCannotBeRead(t *testing.T) {
	jobs := &uiJobService{listErr: errors.New("database offline")}
	s := &server{jobs: jobs}
	if _, err := s.submitModuleJob(context.Background(), state.JobRecord{Type: "module.install"}); err == nil || len(jobs.jobs) != 0 {
		t.Fatal("submitted despite unknown active state")
	}
}
