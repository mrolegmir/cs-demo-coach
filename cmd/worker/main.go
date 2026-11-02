// Command worker processes queued demo analyses and coaching requests.
//
// Configuration (environment):
//
//	CSDC_API_URL       API base URL (default http://localhost:8080)
//	CSDC_WORKER_TOKEN  shared secret for the API's internal endpoints (required)
//	DATABASE_URL       optional; if set, jobs are taken from Postgres directly
//	                   instead of through the API
//	CSDC_LLM           rules (default) | ollama | openai | codex
//	CSDC_LLM_MODEL, CSDC_LLM_BASE_URL, CSDC_LLM_API_KEY  LLM settings, see csdc -h
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/config"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
	"github.com/mrolegmir/cs-demo-coach/internal/worker"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("worker stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	token := os.Getenv("CSDC_WORKER_TOKEN")
	if token == "" {
		return errors.New("CSDC_WORKER_TOKEN is required")
	}
	c, name, err := coach.FromEnv()
	if err != nil {
		return err
	}

	api := worker.API{
		BaseURL: cmp.Or(os.Getenv("CSDC_API_URL"), "http://localhost:8080"),
		Token:   token,
	}
	var queue worker.Queue = api
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		limits, err := config.Quotas()
		if err != nil {
			return err
		}
		st, err := store.Open(ctx, dbURL, limits)
		if err != nil {
			return err
		}
		defer st.Close()
		queue = st
	}

	w := &worker.Worker{
		Queue:       queue,
		Demos:       api,
		Coach:       c,
		CoachName:   name,
		Log:         log,
		Poll:        2 * time.Second,
		MaxAttempts: 3,
		// A 1 GB demo over a home connection takes several minutes.
		Timeout: 30 * time.Minute,
		Stale:   45 * time.Minute,
	}
	return w.Run(ctx)
}
