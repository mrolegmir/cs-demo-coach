package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/jobwire"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// JobStore is what workers need from the store. The API exposes it under
// /internal so workers only need HTTPS and a token, not database access.
type JobStore interface {
	ClaimJob(context.Context, time.Duration) (*store.Job, error)
	FinishDetection(context.Context, *store.Job, *match.Match, detect.Report) error
	FinishAdvice(context.Context, *store.Job, *coach.Advice, string) error
	FailAttempt(context.Context, *store.Job, bool, time.Duration, string) error
	GetMatch(context.Context, int64) (*store.Match, error)
	GetAnalysis(context.Context, int64) (*store.Analysis, error)
	GetBriefInput(context.Context, int64) (*coach.Input, error)
	PendingDemoCleanup(context.Context) ([]int64, error)
	MarkDemoDeleted(context.Context, int64) error
}

// maxInternalBody bounds worker request bodies, including sampled mechanics
// evidence. Real-demo regression tests guard the uncompressed detection payload.
const maxInternalBody = 32 << 20

func (s *Server) routeInternal(mux *http.ServeMux) {
	if s.Jobs == nil {
		return
	}
	// Legacy writes have no attempt identity; never allow them to bypass v2.
	for _, pattern := range []string{"POST /internal/jobs/claim", "POST /internal/jobs/{id}/complete", "POST /internal/jobs/{id}/retry", "POST /internal/jobs/{id}/fail", "POST /internal/matches/{id}/fail", "POST /internal/matches/{id}/fail-coaching", "PUT /internal/matches/{id}/detection", "PUT /internal/matches/{id}/advice", "PUT /internal/briefs/{id}/advice", "POST /internal/briefs/{id}/fail"} {
		mux.HandleFunc(pattern, s.worker(workerUpgradeRequired))
	}
	mux.HandleFunc("POST /internal/v2/jobs/claim", s.worker(s.claimJob))
	mux.HandleFunc("PUT /internal/v2/jobs/{id}/detection", s.worker(s.jobAction(func(r *http.Request, id int64) error {
		var req jobwire.DetectionRequest
		if err := decode(r, &req); err != nil {
			return err
		}
		if req.Match == nil || req.Attempt < 1 {
			return errBadRequest
		}
		return s.Jobs.FinishDetection(r.Context(), &store.Job{ID: id, Attempts: req.Attempt}, req.Match, req.Report)
	})))
	mux.HandleFunc("PUT /internal/v2/jobs/{id}/advice", s.worker(s.jobAction(func(r *http.Request, id int64) error {
		var req jobwire.AdviceRequest
		if err := decode(r, &req); err != nil {
			return err
		}
		if req.Advice == nil || req.Attempt < 1 {
			return errBadRequest
		}
		return s.Jobs.FinishAdvice(r.Context(), &store.Job{ID: id, Attempts: req.Attempt}, req.Advice, req.Coach)
	})))
	mux.HandleFunc("POST /internal/v2/jobs/{id}/failure", s.worker(s.jobAction(func(r *http.Request, id int64) error {
		var req jobwire.FailureRequest
		if err := decode(r, &req); err != nil {
			return err
		}
		if req.Attempt < 1 || req.AfterSeconds < 0 || req.AfterSeconds > 86400 {
			return errBadRequest
		}
		return s.Jobs.FailAttempt(r.Context(), &store.Job{ID: id, Attempts: req.Attempt}, req.Retry, time.Duration(req.AfterSeconds)*time.Second, req.Reason)
	})))
	mux.HandleFunc("GET /internal/briefs/{id}/input", s.worker(func(w http.ResponseWriter, r *http.Request) {
		s.internalGet(w, r, func(ctx context.Context, id int64) (any, error) { return s.Jobs.GetBriefInput(ctx, id) })
	}))
	mux.HandleFunc("GET /internal/matches/{id}", s.worker(func(w http.ResponseWriter, r *http.Request) {
		s.internalGet(w, r, func(ctx context.Context, id int64) (any, error) { return s.Jobs.GetMatch(ctx, id) })
	}))
	mux.HandleFunc("GET /internal/matches/{id}/analysis", s.worker(func(w http.ResponseWriter, r *http.Request) {
		s.internalGet(w, r, func(ctx context.Context, id int64) (any, error) { return s.Jobs.GetAnalysis(ctx, id) })
	}))
}

var errBadRequest = errors.New("bad request")

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxInternalBody)).Decode(v); err != nil {
		return errBadRequest
	}
	return nil
}

func (s *Server) claimJob(w http.ResponseWriter, r *http.Request) {
	var req jobwire.ClaimRequest
	if err := decode(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	job, err := s.Jobs.ClaimJob(r.Context(), time.Duration(req.StaleSeconds)*time.Second)
	if errors.Is(err, store.ErrNoJob) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, job)
}

// jobAction adapts a handler that acts on the {id} path value and has no
// response body.
func (s *Server) jobAction(do func(r *http.Request, id int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := do(r, id); err != nil {
			s.internalError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) internalGet(w http.ResponseWriter, r *http.Request, get func(context.Context, int64) (any, error)) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	v, err := get(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, v)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrLease):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, errBadRequest):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, store.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		s.Log.Error("internal request failed", "path", r.URL.Path, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
