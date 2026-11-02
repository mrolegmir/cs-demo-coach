package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

func workerUpgradeRequired(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "worker protocol v2 required; update the worker", http.StatusConflict)
}

// RunDemoCleanup retries file deletion after durable detection completion.
// A crash after Remove but before MarkDemoDeleted is safe: not-exist succeeds.
func (s *Server) RunDemoCleanup(ctx context.Context) {
	for ctx.Err() == nil {
		if err := s.CleanupDemos(ctx); err != nil {
			s.Log.Error("demo cleanup", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

func (s *Server) CleanupDemos(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ids, err := s.Jobs.PendingDemoCleanup(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := os.Remove(s.demoPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove demo %d: %w", id, err))
			continue
		}
		if err := s.Jobs.MarkDemoDeleted(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("ack demo %d: %w", id, err))
		}
	}
	return errors.Join(errs...)
}
