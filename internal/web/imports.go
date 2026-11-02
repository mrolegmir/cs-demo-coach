package web

import (
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// ImportStore moves share codes through the bot and the downloader.
type ImportStore interface {
	ClaimShareCode(ctx context.Context, from, to string, retryAfter time.Duration) (*store.CodeJob, error)
	ResolveShareCode(ctx context.Context, id int64, demoURL string, played time.Time) error
	RetryShareCode(ctx context.Context, id int64, from, to, reason string) error
	ReleaseShareCode(ctx context.Context, id int64, from, to string) error
	FinishShareCode(ctx context.Context, id int64, from, status string, matchID int64, reason string) error
}

// Wire types of the bot endpoints.
type (
	// CodeClaim is a share code for the bot to resolve.
	CodeClaim struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
	}
	// CodeResolved is the bot's answer: where the demo is and when the match
	// was played (Unix seconds, 0 if unknown).
	CodeResolved struct {
		DemoURL string `json:"demo_url"`
		Played  int64  `json:"played,omitzero"`
	}
	// CodeFailed reports a code the bot could not resolve. Expired means the
	// Game Coordinator no longer has the demo; Permanent, any other error a
	// retry would not fix.
	CodeFailed struct {
		Reason    string `json:"reason"`
		Expired   bool   `json:"expired,omitzero"`
		Permanent bool   `json:"permanent,omitzero"`
	}
)

// How long a claimed or failed code waits before it is tried again. A
// download claim outlives the download timeout, so a running download is
// never taken over.
const (
	codeRetry       = 10 * time.Minute
	downloadRetry   = 30 * time.Minute
	downloadTimeout = 20 * time.Minute
)

const reasonExpired = "The demo is no longer available from Valve."

func (s *Server) routeImports(mux *http.ServeMux) {
	if !s.ShareCodes {
		return
	}
	mux.HandleFunc("POST /internal/sharecodes/claim", s.worker(s.claimCode))
	mux.HandleFunc("POST /internal/sharecodes/{id}/resolve", s.worker(s.jobAction(func(r *http.Request, id int64) error {
		var req CodeResolved
		if err := decode(r, &req); err != nil {
			return err
		}
		if !replayURL(req.DemoURL) {
			return errBadRequest
		}
		var played time.Time
		if req.Played > 0 {
			played = plausiblePlayed(time.Unix(req.Played, 0))
		}
		return s.Store.ResolveShareCode(r.Context(), id, req.DemoURL, played)
	})))
	mux.HandleFunc("POST /internal/sharecodes/{id}/fail", s.worker(s.jobAction(func(r *http.Request, id int64) error {
		var req CodeFailed
		if err := decode(r, &req); err != nil {
			return err
		}
		// Only a code the bot holds can be failed: a late report must not
		// undo a code that was taken over and imported meanwhile.
		switch {
		case req.Expired:
			return s.Store.FinishShareCode(r.Context(), id, store.CodeResolving, store.CodeExpired, 0, reasonExpired)
		case req.Permanent:
			return s.Store.FinishShareCode(r.Context(), id, store.CodeResolving, store.CodeFailed, 0, req.Reason)
		}
		return s.Store.RetryShareCode(r.Context(), id, store.CodeResolving, store.CodeWaiting, req.Reason)
	})))
}

func (s *Server) claimCode(w http.ResponseWriter, r *http.Request) {
	j, err := s.Store.ClaimShareCode(r.Context(), store.CodeWaiting, store.CodeResolving, codeRetry)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, CodeClaim{ID: j.ID, Code: j.Code})
}

// replayURL reports whether u points at Valve's replay servers, the only
// place the api downloads demos from. The URL comes from the bot, and the api
// must not fetch arbitrary addresses for anyone who holds the worker token.
func replayURL(u string) bool {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.User != nil || p.Port() != "" {
		return false
	}
	return strings.HasSuffix(p.Hostname(), ".valve.net")
}

// RunImports downloads resolved share codes until ctx is canceled. Each demo
// goes through the same path as an upload: magic check, dedupe, quota and the
// detect job.
func (s *Server) RunImports(ctx context.Context, every time.Duration) {
	for {
		s.importDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// importDue imports every resolved code that is due.
func (s *Server) importDue(ctx context.Context) {
	for ctx.Err() == nil {
		j, err := s.Store.ClaimShareCode(ctx, store.CodeDownload, store.CodeDownloading, downloadRetry)
		if errors.Is(err, store.ErrNotFound) {
			return
		}
		if err != nil {
			s.Log.Error("share codes: claim download", "err", err)
			return
		}
		s.importCode(ctx, j)
	}
}

// errExpired marks a demo Valve no longer serves.
var errExpired = errors.New("demo expired")

func (s *Server) importCode(ctx context.Context, j *store.CodeJob) {
	log := s.Log.With("code", j.ID, "user", j.SteamID)
	// The bookkeeping must land even when ctx is canceled by a shutdown.
	book := context.WithoutCancel(ctx)
	finish := func(status string, matchID int64, reason string) {
		if err := s.Store.FinishShareCode(book, j.ID, store.CodeDownloading, status, matchID, reason); err != nil {
			log.Error("share codes: finish", "err", err)
		}
	}
	retry := func(err error) {
		if ctx.Err() != nil {
			// Shutting down: hand the code back without using an attempt.
			if err := s.Store.ReleaseShareCode(book, j.ID, store.CodeDownloading, store.CodeDownload); err != nil {
				log.Error("share codes: release", "err", err)
			}
			return
		}
		log.Warn("share codes: import failed, will retry", "err", err)
		if err := s.Store.RetryShareCode(book, j.ID, store.CodeDownloading, store.CodeDownload, "Download failed."); err != nil {
			log.Error("share codes: retry", "err", err)
		}
	}
	u, err := s.Store.GetUser(ctx, j.SteamID)
	if err != nil {
		retry(err)
		return
	}
	q, err := s.quota(ctx, u)
	if err != nil {
		retry(err)
		return
	}
	if q != nil && q.UploadsLeft() <= 0 {
		finish(store.CodeOverQuota, 0, "This week's uploads were used up. Download the match in CS2 later to analyze it.")
		return
	}

	id, err := s.downloadDemo(ctx, j.DemoURL, store.Upload{Filename: j.Code + ".dem", By: j.SteamID, Played: j.Played})
	switch {
	case err == nil, errors.Is(err, store.ErrDuplicate):
		log.Info("share code imported", "match", id, "duplicate", err != nil)
		finish(store.CodeImported, id, "")
	case errors.Is(err, store.ErrQuota):
		finish(store.CodeOverQuota, 0, "This week's uploads were used up.")
	case errors.Is(err, errExpired):
		finish(store.CodeExpired, 0, reasonExpired)
	case errors.Is(err, errNotDemo), errors.Is(err, errTooBig):
		finish(store.CodeFailed, 0, "Valve sent something that is not a usable demo.")
	default:
		retry(err)
	}
}

// downloadDemo fetches a demo from Valve's replay servers, unpacking .bz2 on
// the fly, and saves it like an upload.
func (s *Server) downloadDemo(ctx context.Context, demoURL string, up store.Upload) (int64, error) {
	if !replayURL(demoURL) {
		return 0, errNotDemo
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, demoURL, nil)
	if err != nil {
		return 0, err
	}
	client := http.Client{}
	if s.ImportHTTP != nil {
		client = *s.ImportHTTP
	}
	// Every hop must stay on Valve's replay servers, or a redirect would
	// get around replayURL.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !replayURL(req.URL.String()) {
			return errNotDemo
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return 0, errExpired
	case resp.StatusCode != http.StatusOK:
		return 0, fmt.Errorf("download: %s", resp.Status)
	}
	var body io.Reader = resp.Body
	if strings.HasSuffix(req.URL.Path, ".bz2") {
		body = bzip2.NewReader(resp.Body)
	}
	return s.saveDemo(ctx, http.MaxBytesReader(nil, io.NopCloser(body), MaxDemoSize), up)
}
