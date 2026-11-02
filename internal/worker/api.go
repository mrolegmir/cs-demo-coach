package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/jobwire"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// Queue is the job queue and the match data jobs read and write. *store.Store
// implements it directly; API implements it over the API's internal
// endpoints for workers without database access.
type Queue interface {
	ClaimJob(context.Context, time.Duration) (*store.Job, error)
	FinishDetection(context.Context, *store.Job, *match.Match, detect.Report) error
	FinishAdvice(context.Context, *store.Job, *coach.Advice, string) error
	FailAttempt(context.Context, *store.Job, bool, time.Duration, string) error
	GetMatch(context.Context, int64) (*store.Match, error)
	GetAnalysis(context.Context, int64) (*store.Analysis, error)
	GetBriefInput(context.Context, int64) (*coach.Input, error)
}

// API talks to the API's internal endpoints with the shared worker token. It
// implements both Queue and Demos, so a worker needs nothing but outbound
// HTTPS.
type API struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

var (
	_ Queue = API{}
	_ Demos = API{}
)

func (a API) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	url := strings.TrimSuffix(a.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 == 2 {
		return resp, nil
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	err = fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	if resp.StatusCode == http.StatusConflict {
		err = errors.Join(store.ErrLease, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		err = errors.Join(store.ErrNotFound, err)
	}
	return nil, err
}

// callTimeout bounds a JSON call; demo downloads (Open) are bounded only by
// the job's context.
const callTimeout = 2 * time.Minute

// call sends body and decodes the response into out, if out is not nil.
func (a API) call(ctx context.Context, method, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	resp, err := a.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (a API) ClaimJob(ctx context.Context, stale time.Duration) (*store.Job, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	resp, err := a.do(ctx, http.MethodPost, "/internal/v2/jobs/claim", jobwire.ClaimRequest{StaleSeconds: int(stale.Seconds())})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, store.ErrNoJob
	}
	var j store.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (a API) FinishDetection(ctx context.Context, j *store.Job, m *match.Match, rep detect.Report) error {
	return a.call(ctx, http.MethodPut, fmt.Sprintf("/internal/v2/jobs/%d/detection", j.ID), jobwire.DetectionRequest{Attempt: j.Attempts, Match: m, Report: rep}, nil)
}

func (a API) FinishAdvice(ctx context.Context, j *store.Job, adv *coach.Advice, name string) error {
	return a.call(ctx, http.MethodPut, fmt.Sprintf("/internal/v2/jobs/%d/advice", j.ID), jobwire.AdviceRequest{Attempt: j.Attempts, Advice: adv, Coach: name}, nil)
}

func (a API) FailAttempt(ctx context.Context, j *store.Job, retry bool, after time.Duration, reason string) error {
	return a.call(ctx, http.MethodPost, fmt.Sprintf("/internal/v2/jobs/%d/failure", j.ID), jobwire.FailureRequest{Attempt: j.Attempts, Retry: retry, AfterSeconds: int(after.Seconds()), Reason: reason}, nil)
}

func (a API) GetMatch(ctx context.Context, id int64) (*store.Match, error) {
	var m store.Match
	if err := a.call(ctx, http.MethodGet, fmt.Sprintf("/internal/matches/%d", id), nil, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (a API) GetAnalysis(ctx context.Context, id int64) (*store.Analysis, error) {
	var an store.Analysis
	if err := a.call(ctx, http.MethodGet, fmt.Sprintf("/internal/matches/%d/analysis", id), nil, &an); err != nil {
		return nil, err
	}
	return &an, nil
}

func (a API) GetBriefInput(ctx context.Context, id int64) (*coach.Input, error) {
	var in coach.Input
	if err := a.call(ctx, http.MethodGet, fmt.Sprintf("/internal/briefs/%d/input", id), nil, &in); err != nil {
		return nil, err
	}
	return &in, nil
}

func (a API) Open(ctx context.Context, matchID int64) (io.ReadCloser, error) {
	resp, err := a.do(ctx, http.MethodGet, fmt.Sprintf("/internal/demos/%d", matchID), nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
