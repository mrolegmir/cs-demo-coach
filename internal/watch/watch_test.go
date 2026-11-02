package watch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

type fakeUploader struct {
	got []string
	err map[string]error
}

func (f *fakeUploader) Upload(_ context.Context, path string) (Result, error) {
	name := filepath.Base(path)
	if err := f.err[name]; err != nil {
		return Result{}, err
	}
	f.got = append(f.got, name)
	return Result{URL: "/matches/1"}, nil
}

type env struct {
	dir string
	now time.Time
	up  *fakeUploader
	w   *Watcher
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{dir: t.TempDir(), now: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), up: &fakeUploader{err: map[string]error{}}}
	e.w = e.watcher(t)
	return e
}

// watcher returns a fresh Watcher on the same folder and state file, as after
// a restart.
func (e *env) watcher(t *testing.T) *Watcher {
	t.Helper()
	return &Watcher{
		Dir: e.dir, Uploader: e.up, StateFile: filepath.Join(e.dir, "state.json"),
		Settle: 20 * time.Second, QuotaWait: time.Hour,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now: func() time.Time { return e.now },
	}
}

func (e *env) write(t *testing.T, name string, size int, mtime time.Time) {
	t.Helper()
	path := filepath.Join(e.dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// scan advances the clock and scans once.
func (e *env) scan(t *testing.T, w *Watcher, after time.Duration) {
	t.Helper()
	e.now = e.now.Add(after)
	if err := w.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestUploadsNewDemoOnceWritten(t *testing.T) {
	e := newEnv(t)
	e.write(t, "old.dem", 10, e.now.Add(-time.Hour))
	e.scan(t, e.w, 0) // first run: remembers the start time

	e.write(t, "match730_new.dem", 10, e.now.Add(time.Second))
	e.write(t, "notes.txt", 10, e.now.Add(time.Second))
	e.scan(t, e.w, 5*time.Second)
	if len(e.up.got) != 0 {
		t.Fatalf("uploaded %v while the file may still be downloading", e.up.got)
	}
	e.write(t, "match730_new.dem", 20, e.now) // still growing
	e.scan(t, e.w, 30*time.Second)
	e.scan(t, e.w, 30*time.Second)
	if want := []string{"match730_new.dem"}; !slices.Equal(e.up.got, want) {
		t.Fatalf("uploaded %v, want %v (old demos and other files skipped)", e.up.got, want)
	}

	// A restart must not upload it again.
	w := e.watcher(t)
	e.scan(t, w, time.Minute)
	e.scan(t, w, time.Minute)
	if len(e.up.got) != 1 {
		t.Fatalf("uploaded %v after a restart", e.up.got)
	}
}

func TestAllUploadsExistingDemos(t *testing.T) {
	e := newEnv(t)
	e.w.All = true
	e.write(t, "old.dem", 10, e.now.Add(-time.Hour))
	e.scan(t, e.w, 0)
	e.scan(t, e.w, time.Minute)
	if want := []string{"old.dem"}; !slices.Equal(e.up.got, want) {
		t.Fatalf("uploaded %v, want %v", e.up.got, want)
	}
}

func TestUploadErrors(t *testing.T) {
	e := newEnv(t)
	e.scan(t, e.w, 0)
	for _, name := range []string{"a.dem", "b.dem", "c.dem"} {
		e.write(t, name, 10, e.now)
	}
	e.up.err["a.dem"] = errors.New("connection reset")
	e.up.err["b.dem"] = ErrRejected
	e.scan(t, e.w, time.Minute)
	e.scan(t, e.w, time.Minute)
	if want := []string{"c.dem"}; !slices.Equal(e.up.got, want) {
		t.Fatalf("uploaded %v, want %v", e.up.got, want)
	}
	if want := []string{"b.dem", "c.dem"}; !slices.Equal(e.w.Handled(), want) {
		t.Fatalf("handled %v, want %v: a rejected demo is skipped, a failed one retried", e.w.Handled(), want)
	}

	delete(e.up.err, "a.dem")
	e.scan(t, e.w, time.Minute)
	if want := []string{"c.dem", "a.dem"}; !slices.Equal(e.up.got, want) {
		t.Fatalf("uploaded %v, want %v after the network came back", e.up.got, want)
	}

	e.write(t, "d.dem", 10, e.now)
	e.write(t, "e.dem", 10, e.now)
	e.up.err["d.dem"] = ErrQuota
	e.scan(t, e.w, time.Minute)
	e.scan(t, e.w, time.Minute)
	if slices.Contains(e.w.Handled(), "d.dem") {
		t.Fatal("a demo over the quota was marked as handled; it must be retried later")
	}
	delete(e.up.err, "d.dem")
	e.scan(t, e.w, time.Minute)
	if slices.Contains(e.up.got, "e.dem") {
		t.Fatal("uploads did not pause after the quota ran out")
	}
	e.scan(t, e.w, time.Hour)
	if want := []string{"c.dem", "a.dem", "d.dem", "e.dem"}; !slices.Equal(e.up.got, want) {
		t.Fatalf("uploaded %v after the pause, want %v", e.up.got, want)
	}

	e.write(t, "f.dem", 10, e.now)
	e.scan(t, e.w, time.Minute)
	e.now = e.now.Add(time.Minute)

	e.up.err["f.dem"] = ErrUnauthorized
	if err := e.w.Scan(t.Context()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Scan with a revoked token = %v, want ErrUnauthorized", err)
	}
}
