// Package watch uploads new demos from a folder, typically the CS2 replays
// folder, so a player only has to press Download in the game.
package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Uploader sends one demo file to the service.
type Uploader interface {
	Upload(ctx context.Context, path string) (Result, error)
}

// Result is the outcome of an upload.
type Result struct {
	URL       string
	Duplicate bool
}

// ErrRejected marks an upload the service will never accept (not a demo, too
// large); the watcher skips the file instead of retrying it.
var ErrRejected = errors.New("watch: rejected")

// ErrQuota means the user's weekly uploads are used up; the watcher retries
// later.
var ErrQuota = errors.New("watch: upload quota used")

// Watcher polls Dir and uploads each new .dem file once it stops growing.
type Watcher struct {
	Dir      string
	Uploader Uploader
	// StateFile remembers which files were handled, so restarts do not upload
	// them again.
	StateFile string
	// All uploads demos that were already in Dir on the first run. Without it
	// only demos that appear later are uploaded.
	All bool
	// Poll is how often Dir is scanned; Settle is how long a file must stay
	// unchanged before it counts as fully written.
	Poll, Settle time.Duration
	// QuotaWait is how long uploads pause after the weekly quota ran out.
	QuotaWait time.Duration
	Log       *slog.Logger
	Now       func() time.Time

	state state
	seen  map[string]fileInfo // size and mtime at the previous scan
	// paused holds uploads back until then, after the quota ran out.
	paused time.Time
}

type fileInfo struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
}

// same compares with time.Equal: times read back from the state file lose
// their location and monotonic reading.
func (f fileInfo) same(g fileInfo) bool {
	return f.Size == g.Size && f.ModTime.Equal(g.ModTime)
}

// state is what StateFile holds.
type state struct {
	// Since is when watching started; files older than it are skipped unless
	// All is set.
	Since time.Time           `json:"since"`
	Done  map[string]fileInfo `json:"done"`
}

// Run scans until ctx is canceled. It returns early only on errors that
// retrying cannot fix, such as a revoked token.
func (w *Watcher) Run(ctx context.Context) error {
	if err := w.load(); err != nil {
		return err
	}
	w.Log.Info("watching for demos", "dir", w.Dir)
	for {
		// A Scan cut short by ctx is a normal stop, not a failure.
		if err := w.Scan(ctx); err != nil && ctx.Err() == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(w.Poll):
		}
	}
}

// Scan looks at Dir once and uploads the files that are ready.
func (w *Watcher) Scan(ctx context.Context) error {
	if w.state.Done == nil {
		if err := w.load(); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(w.Dir)
	if err != nil {
		w.Log.Warn("cannot read the demo folder", "dir", w.Dir, "err", err)
		return nil
	}
	seen := map[string]fileInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".dem") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		cur := fileInfo{Size: info.Size(), ModTime: info.ModTime()}
		seen[e.Name()] = cur
		if !w.ready(e.Name(), cur) || w.Now().Before(w.paused) {
			continue
		}
		err = w.upload(ctx, e.Name(), cur)
		if errors.Is(err, ErrQuota) {
			w.paused = w.Now().Add(w.QuotaWait)
			w.Log.Warn("weekly upload quota used; pausing uploads", "file", e.Name(), "until", w.paused.Format(time.TimeOnly))
			continue
		}
		if err != nil {
			return err
		}
	}
	w.seen = seen
	return nil
}

// ready reports whether a file is new and fully written: the same size as at
// the last scan and not modified for Settle.
func (w *Watcher) ready(name string, cur fileInfo) bool {
	if done, ok := w.state.Done[name]; ok && done.same(cur) {
		return false
	}
	if !w.All && cur.ModTime.Before(w.state.Since) {
		return false
	}
	prev, ok := w.seen[name]
	return ok && prev.same(cur) && w.Now().Sub(cur.ModTime) >= w.Settle
}

func (w *Watcher) upload(ctx context.Context, name string, cur fileInfo) error {
	res, err := w.Uploader.Upload(ctx, filepath.Join(w.Dir, name))
	switch {
	case errors.Is(err, ErrRejected):
		w.Log.Warn("demo rejected; skipping it", "file", name, "err", err)
	case errors.Is(err, ErrQuota), errors.Is(err, ErrUnauthorized), ctx.Err() != nil:
		return err
	case err != nil:
		// Network trouble or a server error: try again at the next scan.
		w.Log.Warn("upload failed; will retry", "file", name, "err", err)
		return nil
	case res.Duplicate:
		w.Log.Info("already uploaded", "file", name, "match", res.URL)
	default:
		w.Log.Info("demo uploaded", "file", name, "match", res.URL)
	}
	w.state.Done[name] = cur
	return w.save()
}

func (w *Watcher) load() error {
	b, err := os.ReadFile(w.StateFile)
	if errors.Is(err, os.ErrNotExist) {
		w.state = state{Since: w.Now(), Done: map[string]fileInfo{}}
		return w.save()
	}
	if err != nil {
		return fmt.Errorf("watch: read state: %w", err)
	}
	if err := json.Unmarshal(b, &w.state); err != nil {
		return fmt.Errorf("watch: state file %s: %w", w.StateFile, err)
	}
	if w.state.Done == nil {
		w.state.Done = map[string]fileInfo{}
	}
	return nil
}

// save writes the state through a temporary file, so a crash never leaves a
// half-written state behind.
func (w *Watcher) save() error {
	b, err := json.MarshalIndent(w.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := w.StateFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("watch: save state: %w", err)
	}
	if err := os.Rename(tmp, w.StateFile); err != nil {
		return fmt.Errorf("watch: save state: %w", err)
	}
	return nil
}

// Handled returns the names of the files uploaded or skipped so far, sorted.
func (w *Watcher) Handled() []string {
	return slices.Sorted(maps.Keys(w.state.Done))
}
