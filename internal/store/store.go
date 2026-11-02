// Package store keeps matches, findings and the job queue in Postgres.
package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Match statuses.
const (
	StatusUploaded = "uploaded" // waiting for detection
	StatusDetected = "detected" // findings ready, party not chosen yet
	StatusCoaching = "coaching" // waiting for the coach
	StatusCoached  = "coached"  // advice ready
	StatusFailed   = "failed"
)

// Job kinds.
const (
	JobDetect = "detect"
	JobCoach  = "coach"
)

// ErrNotFound is returned when a match does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrDuplicate is returned with the existing match's ID when a user uploads
// a demo they already uploaded.
var ErrDuplicate = errors.New("store: duplicate demo")

// ErrNotReady is returned when a match is not in a state that allows the
// requested change, e.g. coaching before detection finished.
var ErrNotReady = errors.New("store: match not ready")

// Store is a Postgres-backed store.
type Store struct {
	db     *pgxpool.Pool
	limits Limits
}

// Open connects to Postgres and applies pending migrations.
func Open(ctx context.Context, url string, limits Limits) (*Store, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, limits: limits}, nil
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Closing this *sql.DB leaves the pool open.
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close() //nolint:errcheck // nothing to flush
	dir, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, dir)
	if err != nil {
		return fmt.Errorf("store: migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// Close closes the connection pool.
func (s *Store) Close() { s.db.Close() }

// Match is a match as listed and shown in the UI.
type Match struct {
	ID         int64
	Filename   string
	UploadedAt time.Time
	Status     string
	Error      string
	Map        string
	Rounds     int
	Party      []match.PlayerID
	Coach      string
	// UploadedBy is the uploading user, or 0 for uploads without accounts.
	UploadedBy match.PlayerID
	Players    []Player
	// RoundResults is the compact round history used by the match list.
	RoundResults []match.Round
}

// Player is a participant of a stored match.
type Player struct {
	ID   match.PlayerID
	Name string
	// Side is the side in the first round.
	Side match.Side
}

// Upload is a new demo.
type Upload struct {
	Filename string
	// By is the uploading user, or 0 for uploads without accounts (CLI,
	// tests).
	By match.PlayerID
	// SHA256 is the file's hash, or nil.
	SHA256 []byte
	// Played is when the match was played, or zero if unknown.
	Played time.Time
}

// CreateMatch records an uploaded demo and queues it for detection. If the
// user already uploaded a file with the same hash, it returns that match's ID
// and ErrDuplicate, unless that match failed: then the old match lets go of
// the hash so the demo can be analyzed again.
func (s *Store) CreateMatch(ctx context.Context, u Upload) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Serialize deduplication with admission, including simultaneous uploads.
		if err := checkQuota(ctx, tx, u.By, 0, true); err != nil {
			return err
		}
		if u.SHA256 != nil {
			var status string
			err := tx.QueryRow(ctx, `SELECT id, status FROM matches WHERE uploaded_by IS NOT DISTINCT FROM $1 AND sha256 = $2`,
				nullID(u.By), u.SHA256).Scan(&id, &status)
			switch {
			case err == nil && status != StatusFailed:
				return ErrDuplicate
			case err == nil:
				if _, err := tx.Exec(ctx, `UPDATE matches SET sha256 = NULL WHERE id = $1`, id); err != nil {
					return err
				}
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
		}
		if err := checkQuota(ctx, tx, u.By, s.limits.Uploads, true); err != nil {
			return err
		}
		var played *time.Time
		if !u.Played.IsZero() {
			played = &u.Played
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO matches (filename, uploaded_by, sha256, played_at) VALUES ($1, $2, $3, $4) RETURNING id`,
			u.Filename, nullID(u.By), u.SHA256, played).Scan(&id)
		if err != nil {
			return err
		}
		return enqueue(ctx, tx, JobDetect, id, u.By, "ru")
	})
	if errors.Is(err, ErrDuplicate) {
		return id, err
	}
	if err != nil {
		return 0, fmt.Errorf("store: create match: %w", err)
	}
	return id, nil
}

// ListMatches returns the most recent matches a user may see (see CanView),
// newest first, without players. A nil user sees everything.
func (s *Store) ListMatches(ctx context.Context, u *User, limit int) ([]Match, error) {
	all, viewer := true, int64(0)
	if u != nil {
		all, viewer = u.Admin, int64(u.SteamID)
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, filename, uploaded_at, status, coalesce(error, ''), coalesce(map, ''), coalesce(rounds, 0),
		       coalesce(party, '{}'), coalesce(coach, ''), coalesce(uploaded_by, 0),
		       (SELECT jsonb_agg(jsonb_build_object('winner', r->'winner', 'players', r->'players'))
		        FROM jsonb_array_elements(m.data->'rounds') r)
		FROM matches m
		WHERE $2 OR m.uploaded_by = $3
		   OR EXISTS (SELECT 1 FROM match_players mp WHERE mp.match_id = m.id AND mp.player_id = $3)
		ORDER BY id DESC LIMIT $1`, limit, all, viewer)
	if err != nil {
		return nil, fmt.Errorf("store: list matches: %w", err)
	}
	ms, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Match, error) {
		var rounds []byte
		m, err := scanMatchFields(row, &rounds)
		if err == nil && len(rounds) > 0 {
			err = json.Unmarshal(rounds, &m.RoundResults)
		}
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: list matches: %w", err)
	}
	return ms, nil
}

// GetMatch returns a match with its players.
func (s *Store) GetMatch(ctx context.Context, id int64) (*Match, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, filename, uploaded_at, status, coalesce(error, ''), coalesce(map, ''), coalesce(rounds, 0),
		       coalesce(party, '{}'), coalesce(coach, ''), coalesce(uploaded_by, 0)
		FROM matches WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("store: get match: %w", err)
	}
	m, err := pgx.CollectExactlyOneRow(rows, scanMatch)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get match: %w", err)
	}

	rows, err = s.db.Query(ctx, `SELECT player_id, name, side FROM match_players WHERE match_id = $1 ORDER BY side, name`, id)
	if err != nil {
		return nil, fmt.Errorf("store: get players: %w", err)
	}
	m.Players, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Player, error) {
		var p Player
		var pid int64
		var side int16
		err := row.Scan(&pid, &p.Name, &side)
		p.ID, p.Side = match.PlayerID(pid), match.Side(side)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: get players: %w", err)
	}
	return &m, nil
}

func scanMatch(row pgx.CollectableRow) (Match, error) {
	return scanMatchFields(row)
}

func scanMatchFields(row pgx.CollectableRow, extra ...any) (Match, error) {
	var m Match
	var party []int64
	var by int64
	fields := []any{&m.ID, &m.Filename, &m.UploadedAt, &m.Status, &m.Error, &m.Map, &m.Rounds, &party, &m.Coach, &by}
	err := row.Scan(append(fields, extra...)...)
	m.UploadedBy = match.PlayerID(by)
	for _, p := range party {
		m.Party = append(m.Party, match.PlayerID(p))
	}
	return m, err
}

// Analysis is everything computed for a match. Advice is nil until the match
// has been coached.
type Analysis struct {
	Match  *match.Match
	Report *detect.Report
	Advice *coach.Advice
}

// GetAnalysis loads the parsed match, findings and advice of a match.
func (s *Store) GetAnalysis(ctx context.Context, id int64) (*Analysis, error) {
	var data, report, advice []byte
	err := s.db.QueryRow(ctx, `SELECT data, report, advice FROM matches WHERE id = $1`, id).Scan(&data, &report, &advice)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get analysis: %w", err)
	}
	if data == nil || report == nil {
		return nil, fmt.Errorf("store: match %d has not been analyzed", id)
	}
	a := &Analysis{Match: &match.Match{}, Report: &detect.Report{}}
	if err := json.Unmarshal(data, a.Match); err != nil {
		return nil, fmt.Errorf("store: decode match: %w", err)
	}
	if err := json.Unmarshal(report, a.Report); err != nil {
		return nil, fmt.Errorf("store: decode report: %w", err)
	}
	if advice != nil {
		a.Advice = &coach.Advice{}
		if err := json.Unmarshal(advice, a.Advice); err != nil {
			return nil, fmt.Errorf("store: decode advice: %w", err)
		}
	}
	return a, nil
}

// SaveDetection stores the parsed match and its findings and marks the match
// detected.
func (s *Store) SaveDetection(ctx context.Context, id int64, m *match.Match, rep detect.Report) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error { return saveDetection(ctx, tx, id, m, rep) })
}

func saveDetection(ctx context.Context, tx pgx.Tx, id int64, m *match.Match, rep detect.Report) error {
	if rep.Pairs == nil || rep.Roles == nil {
		// A worker from before pairs or roles sends none; the match is here.
		rep.Pairs, rep.Roles = detect.Derived(m)
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	report, err := json.Marshal(rep)
	if err != nil {
		return err
	}

	var current string
	if err := tx.QueryRow(ctx, `SELECT status FROM matches WHERE id=$1 FOR UPDATE`, id).Scan(&current); err != nil {
		return err
	}
	if current == StatusCoaching || current == StatusCoached {
		return nil
	}
	_, err = tx.Exec(ctx, `
			UPDATE matches SET status = $2, error = NULL, map = $3, rounds = $4, data = $5, report = $6
			WHERE id = $1`, id, StatusDetected, m.Map, len(m.Rounds), data, report)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM match_players WHERE match_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM findings WHERE match_id = $1`, id); err != nil {
		return err
	}
	var players [][]any
	for _, p := range m.Players {
		players = append(players, []any{id, int64(p.ID), p.Name, int16(m.StartSide(p.ID))})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"match_players"},
		[]string{"match_id", "player_id", "name", "side"}, pgx.CopyFromRows(players)); err != nil {
		return err
	}
	var findings [][]any
	for _, f := range rep.Findings {
		findings = append(findings, []any{id, f.ID, string(f.Kind), int64(f.Player), f.Round, f.Tick, f.Detail})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"findings"},
		[]string{"match_id", "finding_id", "kind", "player_id", "round", "tick", "detail"}, pgx.CopyFromRows(findings))
	return err
}

// RequestCoaching sets the party of a detected match and queues the coach on
// behalf of user by (0 for none). Language is snapshotted on the queued job.
func (s *Store) RequestCoaching(ctx context.Context, id int64, party []match.PlayerID, by match.PlayerID, language string) error {
	ids := make([]int64, len(party))
	for i, p := range party {
		ids[i] = int64(p)
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := checkQuota(ctx, tx, by, s.limits.Coaching, false); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE matches SET party = $2, party_manual = true, status = $3, advice = NULL, coach = NULL, error = NULL
			WHERE id = $1 AND status IN ($4, $5)`, id, ids, StatusCoaching, StatusDetected, StatusCoached)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotReady
		}
		return enqueue(ctx, tx, JobCoach, id, by, coach.Language(language))
	})
	if err != nil {
		return fmt.Errorf("store: request coaching: %w", err)
	}
	return nil
}

// SaveAdvice stores coach output and marks the match coached.
func (s *Store) SaveAdvice(ctx context.Context, id int64, a *coach.Advice, coachName string) error {
	advice, err := json.Marshal(a)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE matches SET status = $2, advice = $3, coach = $4, error = NULL WHERE id = $1`,
		id, StatusCoached, advice, coachName)
	if err != nil {
		return fmt.Errorf("store: save advice: %w", err)
	}
	return nil
}

// FailCoaching records why coaching failed and returns the match to
// detected, so its findings stay visible and coaching can be retried.
func (s *Store) FailCoaching(ctx context.Context, id int64, reason string) error {
	_, err := s.db.Exec(ctx, `UPDATE matches SET status = $2, error = $3 WHERE id = $1 AND status = $4`,
		id, StatusDetected, reason, StatusCoaching)
	if err != nil {
		return fmt.Errorf("store: fail coaching: %w", err)
	}
	return nil
}

// FailMatch marks a match failed with a user-visible reason.
func (s *Store) FailMatch(ctx context.Context, id int64, reason string) error {
	_, err := s.db.Exec(ctx, `UPDATE matches SET status = $2, error = $3 WHERE id = $1`, id, StatusFailed, reason)
	if err != nil {
		return fmt.Errorf("store: fail match: %w", err)
	}
	return nil
}

// Ping checks the database connection.
func (s *Store) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

// missingDerived matches a report made before pairs or roles were counted.
const missingDerived = `(coalesce(jsonb_typeof(report->'pairs'), 'null') = 'null'
	OR coalesce(jsonb_typeof(report->'roles'), 'null') = 'null')`

// MatchesToBackfill returns analyzed matches whose report was made before
// trade pairs or roles were counted.
func (s *Store) MatchesToBackfill(ctx context.Context) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM matches
		WHERE report IS NOT NULL AND data IS NOT NULL AND `+missingDerived+`
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: matches to backfill: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("store: matches to backfill: %w", err)
	}
	return ids, nil
}

// SetReportDerived adds pairs and roles to a stored report that lacks them,
// so a newer detection saved meanwhile is left alone.
func (s *Store) SetReportDerived(ctx context.Context, id int64, pairs []detect.Pair, roles []detect.Role) error {
	b, err := json.Marshal(struct {
		Pairs []detect.Pair `json:"pairs"`
		Roles []detect.Role `json:"roles"`
	}{pairs, roles})
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE matches SET report = report || $2::jsonb
		WHERE id = $1 AND `+missingDerived, id, b); err != nil {
		return fmt.Errorf("store: set report derived: %w", err)
	}
	return nil
}
