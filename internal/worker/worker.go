// Package worker runs queued jobs: it parses and analyzes uploaded demos and
// coaches parties. A worker pulls jobs, either from Postgres directly or
// through the API's internal endpoints, so it needs no inbound connections
// and can run on a home PC.
package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/demo"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// Demos gives access to uploaded demo files.
type Demos interface {
	Open(ctx context.Context, matchID int64) (io.ReadCloser, error)
}

// Worker processes jobs until its context is canceled.
type Worker struct {
	Queue     Queue
	Demos     Demos
	Coach     coach.Coach
	CoachName string
	Log       *slog.Logger

	// Poll is how long to wait when the queue is empty.
	Poll time.Duration
	// MaxAttempts is how often a job is tried before the match fails.
	MaxAttempts int
	// Timeout bounds one job, including the demo download and LLM calls.
	Timeout time.Duration
	// Stale is how long a running job may go without finishing before
	// another worker takes it over. It must exceed Timeout.
	Stale time.Duration
	// TempDir holds demos while they are parsed; empty means os.TempDir.
	TempDir string
}

// permanent marks errors that retrying cannot fix, such as a corrupt demo.
type permanent struct{ err error }

func (p permanent) Error() string { return p.err.Error() }
func (p permanent) Unwrap() error { return p.err }

// Run processes jobs until ctx is canceled.
func (w *Worker) Run(ctx context.Context) error {
	w.Log.Info("worker started", "coach", w.CoachName)
	for ctx.Err() == nil {
		ok, err := w.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			w.Log.Error("queue", "err", err)
		}
		if ok {
			continue
		}
		select {
		case <-ctx.Done():
		case <-time.After(w.Poll):
		}
	}
	return nil
}

// RunOnce claims and runs one job. It reports whether there was a job.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, err := w.Queue.ClaimJob(ctx, w.Stale)
	if errors.Is(err, store.ErrNoJob) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	log := w.Log.With("job", job.ID, "kind", job.Kind, "match", job.MatchID, "brief", job.BriefID, "attempt", job.Attempts)

	// A job claimed more often than allowed was taken over from workers
	// that died while running it, e.g. on a demo that crashes the parser.
	if job.Attempts > w.MaxAttempts {
		err := permanent{errors.New("the worker stopped while processing this job too many times")}
		log.Error("job failed", "err", err)
		return true, w.fail(ctx, job, err)
	}

	jobCtx, cancelJob := context.WithTimeout(ctx, w.Timeout)
	defer cancelJob()
	start := time.Now()
	switch job.Kind {
	case store.JobDetect:
		err = w.detect(jobCtx, job)
	case store.JobCoach:
		err = w.coach(jobCtx, job)
	case store.JobBrief:
		err = w.brief(jobCtx, job)
	default:
		err = permanent{fmt.Errorf("unknown job kind %q", job.Kind)}
	}
	// Bookkeeping must survive shutdown: a canceled ctx would leave the job
	// "running" until it goes stale.
	book, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err == nil {
		log.Info("job done", "took", time.Since(start).Round(time.Millisecond))
		return true, nil
	}

	if _, ok := errors.AsType[permanent](err); ok || job.Attempts >= w.MaxAttempts {
		log.Error("job failed", "err", err)
		return true, w.fail(book, job, err)
	}
	backoff := time.Duration(job.Attempts*job.Attempts) * 10 * time.Second
	log.Warn("job will be retried", "err", err, "in", backoff)
	return true, w.Queue.FailAttempt(book, job, true, backoff, err.Error())
}

// fail gives up on a job. A failed detection fails the match; a failed
// coaching keeps the findings and lets the user try again; a failed brief
// is marked failed.
func (w *Worker) fail(ctx context.Context, job *store.Job, err error) error {
	return w.Queue.FailAttempt(ctx, job, false, 0, err.Error())
}

func (w *Worker) brief(ctx context.Context, job *store.Job) error {
	in, err := w.Queue.GetBriefInput(ctx, job.BriefID)
	if err != nil {
		return err
	}
	advice, err := w.Coach.Advise(ctx, in)
	if errors.Is(err, coach.ErrUngrounded) {
		return permanent{err}
	}
	if err != nil {
		return err
	}
	if err := in.Validate(advice); err != nil {
		return permanent{fmt.Errorf("%w: %w", coach.ErrUngrounded, err)}
	}
	in.Identify(advice)
	return w.Queue.FinishAdvice(ctx, job, advice, w.CoachName)
}

func (w *Worker) detect(ctx context.Context, job *store.Job) error {
	matchID := job.MatchID
	// Recover a pre-v2 result without downloading a deleted demo.
	mt, err := w.Queue.GetMatch(ctx, matchID)
	if err != nil {
		return err
	}
	if mt.Status == store.StatusDetected || mt.Status == store.StatusCoaching || mt.Status == store.StatusCoached {
		an, err := w.Queue.GetAnalysis(ctx, matchID)
		if err != nil {
			return err
		}
		return w.Queue.FinishDetection(ctx, job, an.Match, *an.Report)
	}
	// Download first, then parse: a failed download can be retried, while a
	// parse error means the demo itself is broken.
	path, err := w.download(ctx, matchID)
	if err != nil {
		return fmt.Errorf("fetch demo: %w", err)
	}
	defer func() { _ = os.Remove(path) }()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	m, err := demo.Parse(f)
	f.Close()
	if err != nil {
		return permanent{fmt.Errorf("could not read the demo: %w", err)}
	}
	m.ID = fmt.Sprint(matchID)
	return w.Queue.FinishDetection(ctx, job, m, detect.Run(m, detect.Default()))
}

func (w *Worker) download(ctx context.Context, matchID int64) (string, error) {
	src, err := w.Demos.Open(ctx, matchID)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.CreateTemp(w.TempDir, "demo-*.dem")
	if err != nil {
		return "", err
	}
	// The HTTP client reports a body shorter than its Content-Length as an
	// error, so a complete copy means a complete demo.
	_, err = io.Copy(dst, src)
	if err = errors.Join(err, dst.Close()); err != nil {
		_ = os.Remove(dst.Name())
		return "", err
	}
	return dst.Name(), nil
}

func (w *Worker) coach(ctx context.Context, job *store.Job) error {
	matchID, language := job.MatchID, job.Language
	mt, err := w.Queue.GetMatch(ctx, matchID)
	if err != nil {
		return err
	}
	an, err := w.Queue.GetAnalysis(ctx, matchID)
	if err != nil {
		return err
	}
	in := coach.NewInput(an.Match, *an.Report, mt.Party)
	in.Language = coach.Language(language)
	advice, err := w.Coach.Advise(ctx, in)
	if errors.Is(err, coach.ErrUngrounded) {
		return permanent{err}
	}
	if err != nil {
		return err
	}
	if err := in.Validate(advice); err != nil {
		return permanent{fmt.Errorf("%w: %w", coach.ErrUngrounded, err)}
	}
	in.Identify(advice)
	return w.Queue.FinishAdvice(ctx, job, advice, w.CoachName)
}
