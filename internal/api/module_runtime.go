package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type moduleRuntimeReader interface {
	Runtime(context.Context) (modules.RuntimeStatus, error)
}

func (s *server) moduleRuntime(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	service, ok := s.modules.(moduleRuntimeReader)
	if !ok {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_runtime_unavailable", "runtime inspection unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Second)
	defer cancel()
	result, err := service.Runtime(ctx)
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "module_runtime_unavailable", "runtime inspection unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtime": result})
}

var errModuleOperationInProgress = errors.New("module operation already in progress")

func isModuleJob(kind string) bool {
	return kind == "module.install" || kind == "module.control" || kind == "module.remove"
}

// The runtime serializes changes globally. Reject duplicate/conflicting queue
// submissions, including simultaneous tabs, rather than failing a second job
// after the first has already begun changing the host.
func (s *server) activeModuleJobs(ctx context.Context) ([]state.JobRecord, error) {
	if s.jobs == nil {
		return nil, errors.New("job service unavailable")
	}
	out := make([]state.JobRecord, 0)
	for _, status := range []string{"queued", "running", "cancel_requested"} {
		records, err := s.jobs.List(ctx, status, 500)
		if err != nil {
			return nil, err
		}
		if len(records) >= 500 {
			return nil, errors.New("active job list is incomplete; cannot safely schedule a module operation")
		}
		for _, job := range records {
			if job.Status == status && isModuleJob(job.Type) {
				out = append(out, moduleJobView(job))
			}
		}
	}
	return out, nil
}

func (s *server) submitModuleJob(ctx context.Context, record state.JobRecord) (state.JobRecord, error) {
	if !s.moduleSubmitMu.TryLock() {
		return state.JobRecord{}, errModuleOperationInProgress
	}
	defer s.moduleSubmitMu.Unlock()
	active, err := s.activeModuleJobs(ctx)
	if err != nil {
		return state.JobRecord{}, err
	}
	if len(active) != 0 {
		return state.JobRecord{}, errModuleOperationInProgress
	}
	return s.jobs.Submit(ctx, record)
}

func writeModuleSubmitError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errModuleOperationInProgress) {
		writeAPIError(w, r, http.StatusConflict, "module_operation_in_progress", "another module operation is already active", nil)
		return
	}
	writeAPIError(w, r, http.StatusInternalServerError, "module_job_failed", "failed to queue module operation", nil)
}

// Module managers may observe only module operations here, not arbitrary jobs
// and not the raw stored manifest. General job browsing still needs jobs.read.
func moduleJobView(record state.JobRecord) state.JobRecord {
	input := map[string]any{}
	for _, key := range []string{"module_id", "operation"} {
		if value, ok := record.Input[key].(string); ok {
			input[key] = value
		}
	}
	record.Input = input
	result := map[string]any{}
	for _, key := range []string{"module_id", "status", "version"} {
		if value, ok := record.Result[key].(string); ok {
			result[key] = value
		}
	}
	record.Result = result
	return record
}

func (s *server) moduleOperations(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	jobs, err := s.activeModuleJobs(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "jobs_unavailable", "module operations unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *server) moduleOperation(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if s.jobs == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "jobs_unavailable", "module operation unavailable", nil)
		return
	}
	job, err := s.jobs.Get(r.Context(), r.PathValue("jobID"))
	if errors.Is(err, state.ErrJobNotFound) || err == nil && !isModuleJob(job.Type) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "jobs_unavailable", "module operation unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": moduleJobView(job)})
}
