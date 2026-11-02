// Command api serves the CS Demo Coach web UI and the internal endpoints
// workers use to fetch demos.
//
// Configuration (environment):
//
//	DATABASE_URL       Postgres connection URL (required)
//	CSDC_DEMO_DIR      where uploaded demos wait for a worker (default ./demos/uploads)
//	CSDC_WORKER_TOKEN  shared secret for workers (required)
//	CSDC_ADDR          listen address (default :8080)
//
// Steam sign-in (all pages require it when CSDC_PUBLIC_URL is set):
//
//	CSDC_PUBLIC_URL      external base URL, e.g. https://csdc.example.com
//	CSDC_ADMIN_STEAM_IDS comma-separated SteamID64s let in as admins; they
//	                     invite everyone else
//	CSDC_QUOTA_UPLOADS   uploads per user per 7 days (default 15, 0 = unlimited)
//	CSDC_QUOTA_COACHING  coaching requests per user per 7 days (default 15)
//	CSDC_BRIEF_HOUR      weekly briefs are made on Fridays at this hour (default 15)
//	CSDC_TZ              time zone for that hour (default UTC), e.g. Europe/Moscow
//	CSDC_STEAM_API_KEY   Steam Web API key; when set, users can link their
//	                     match history, new share codes are collected every
//	                     30 minutes (ADR 11), and demos the bot resolved are
//	                     downloaded from Valve (ADR 12)
//	CSDC_TELEGRAM_TOKEN  Telegram bot token from @BotFather; when set, users
//	                     can link a chat and ready briefs are sent there
//	                     (ADR 13)
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/brief"
	"github.com/mrolegmir/cs-demo-coach/internal/config"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/importer"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
	"github.com/mrolegmir/cs-demo-coach/internal/telegram"
	"github.com/mrolegmir/cs-demo-coach/internal/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL, token := os.Getenv("DATABASE_URL"), os.Getenv("CSDC_WORKER_TOKEN")
	if dbURL == "" || token == "" {
		return errors.New("DATABASE_URL and CSDC_WORKER_TOKEN are required")
	}
	demoDir := cmp.Or(os.Getenv("CSDC_DEMO_DIR"), "demos/uploads")
	if err := os.MkdirAll(demoDir, 0o750); err != nil {
		return err
	}

	limits := store.Limits{}
	if os.Getenv("CSDC_PUBLIC_URL") != "" {
		var err error
		limits, err = config.Quotas()
		if err != nil {
			return err
		}
	}
	st, err := store.Open(ctx, dbURL, limits)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := &web.Server{Store: st, Jobs: st, DemoDir: demoDir, WorkerToken: token, Log: log}
	if publicURL := os.Getenv("CSDC_PUBLIC_URL"); publicURL != "" {
		if srv.Auth, err = authConfig(ctx, st, publicURL); err != nil {
			return err
		}
		log.Info("steam sign-in enabled", "url", publicURL)
		planner, err := briefPlanner(st, log)
		if err != nil {
			return err
		}
		go planner.Run(ctx)
		log.Info("weekly briefs scheduled", "weekday", planner.Schedule.Weekday, "hour", planner.Schedule.Hour, "tz", planner.Schedule.Location)
		if key := os.Getenv("CSDC_STEAM_API_KEY"); key != "" {
			im := &importer.Importer{
				Store: st, Log: log, Every: 30 * time.Minute,
				API: &importer.Steam{Key: key, HTTP: &http.Client{Timeout: 30 * time.Second}},
			}
			go im.Run(ctx)
			srv.ShareCodes = true
			go srv.RunImports(ctx, time.Minute)
			log.Info("share-code import enabled", "every", im.Every)
		}
		if tok := os.Getenv("CSDC_TELEGRAM_TOKEN"); tok != "" {
			startTelegram(ctx, srv, st, tok, log)
		}
	} else {
		log.Warn("CSDC_PUBLIC_URL not set: running without sign-in, for local use only")
	}
	go backfill(ctx, st, log)
	go srv.RunDemoCleanup(ctx)
	h, err := srv.Handler()
	if err != nil {
		return err
	}
	httpSrv := &http.Server{
		Addr:              cmp.Or(os.Getenv("CSDC_ADDR"), ":8080"),
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()
	log.Info("api listening", "addr", httpSrv.Addr)
	select {
	case err := <-errc:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdown)
}

func authConfig(ctx context.Context, st *store.Store, publicURL string) (*web.Auth, error) {
	a := &web.Auth{PublicURL: strings.TrimSuffix(publicURL, "/")}
	var err error
	if a.UploadsPerWeek, err = envInt("CSDC_QUOTA_UPLOADS", 15); err != nil {
		return nil, err
	}
	if a.CoachingPerWeek, err = envInt("CSDC_QUOTA_COACHING", 15); err != nil {
		return nil, err
	}
	for s := range strings.SplitSeq(os.Getenv("CSDC_ADMIN_STEAM_IDS"), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		id, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("CSDC_ADMIN_STEAM_IDS: %w", err)
		}
		if err := st.EnsureAdmin(ctx, match.PlayerID(id)); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// backfill counts trade pairs and roles for matches analyzed before they
// were counted, from the parsed match the store keeps. It runs once at start
// and does nothing once every report has them.
func backfill(ctx context.Context, st *store.Store, log *slog.Logger) {
	ids, err := st.MatchesToBackfill(ctx)
	if err != nil {
		log.Error("report backfill", "err", err)
		return
	}
	done := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		an, err := st.GetAnalysis(ctx, id)
		if err == nil && an.Match != nil {
			pairs, roles := detect.Derived(an.Match)
			err = st.SetReportDerived(ctx, id, pairs, roles)
		}
		if err != nil {
			log.Warn("report backfill", "match", id, "err", err)
			continue
		}
		done++
	}
	if done > 0 {
		log.Info("reports backfilled with pairs and roles", "matches", done)
	}
}

// startTelegram starts brief delivery to Telegram. The site does not wait
// for it: the Connect button appears once the bot has reached Telegram.
func startTelegram(ctx context.Context, srv *web.Server, st *store.Store, token string, log *slog.Logger) {
	bot := &telegram.Bot{
		API:   &telegram.Client{Token: token, HTTP: &http.Client{Timeout: 70 * time.Second}},
		Store: st, PublicURL: srv.Auth.PublicURL, Log: log,
	}
	go bot.Run(ctx)
	srv.TelegramBot = bot.Name
}

func envInt(name string, def int) (int, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: want a non-negative number, got %q", name, v)
	}
	return n, nil
}

func briefPlanner(st *store.Store, log *slog.Logger) (*brief.Planner, error) {
	hour, err := envInt("CSDC_BRIEF_HOUR", 15)
	if err != nil || hour > 23 {
		return nil, fmt.Errorf("CSDC_BRIEF_HOUR: want 0-23")
	}
	loc, err := time.LoadLocation(cmp.Or(os.Getenv("CSDC_TZ"), "UTC"))
	if err != nil {
		return nil, fmt.Errorf("CSDC_TZ: %w", err)
	}
	return &brief.Planner{
		Store:    st,
		Schedule: brief.Schedule{Weekday: time.Friday, Hour: hour, Location: loc},
		Log:      log,
		Every:    10 * time.Minute,
	}, nil
}
