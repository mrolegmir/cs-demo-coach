package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// JobBrief coaches a weekly brief.
const JobBrief = "brief"

// Brief statuses.
const (
	BriefQueued = "queued"
	BriefReady  = "ready"
	BriefFailed = "failed"
)

// Brief is a weekly summary for a user's party.
type Brief struct {
	ID          int64
	SteamID     match.PlayerID
	PeriodStart time.Time
	PeriodEnd   time.Time
	Party       []match.PlayerID
	Status      string
	Error       string
	Input       *coach.Input
	Advice      *coach.Advice
	Coach       string
	CreatedAt   time.Time
	// RequestedBy is the user who asked for the brief and pays for it in
	// quota; 0 for scheduled briefs.
	RequestedBy match.PlayerID
}

// CreateBrief stores a brief with its coach input and queues the coach.
func (s *Store) CreateBrief(ctx context.Context, b *Brief) (int64, error) {
	input, err := json.Marshal(b.Input)
	if err != nil {
		return 0, err
	}
	language := "ru"
	if b.Input != nil {
		language = coach.Language(b.Input.Language)
	}
	party := make([]int64, len(b.Party))
	for i, p := range b.Party {
		party[i] = int64(p)
	}
	var id int64
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := checkQuota(ctx, tx, b.RequestedBy, s.limits.Coaching, false); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO briefs (steam_id, period_start, period_end, party, input)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			int64(b.SteamID), b.PeriodStart, b.PeriodEnd, party, input).Scan(&id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO jobs (kind, brief_id, requested_by, language) VALUES ($1, $2, $3, $4)`,
			JobBrief, id, nullID(b.RequestedBy), language)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("store: create brief: %w", err)
	}
	return id, nil
}

// GetBrief returns a brief with its input and advice.
func (s *Store) GetBrief(ctx context.Context, id int64) (*Brief, error) {
	b := Brief{ID: id}
	var steam int64
	var party []int64
	var input, advice []byte
	err := s.db.QueryRow(ctx, `
		SELECT steam_id, period_start, period_end, party, status, coalesce(error, ''), input, advice,
		       coalesce(coach, ''), created_at
		FROM briefs WHERE id = $1`, id).
		Scan(&steam, &b.PeriodStart, &b.PeriodEnd, &party, &b.Status, &b.Error, &input, &advice, &b.Coach, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get brief: %w", err)
	}
	b.SteamID = match.PlayerID(steam)
	for _, p := range party {
		b.Party = append(b.Party, match.PlayerID(p))
	}
	b.Input = &coach.Input{}
	if err := json.Unmarshal(input, b.Input); err != nil {
		return nil, fmt.Errorf("store: decode brief input: %w", err)
	}
	if advice != nil {
		b.Advice = &coach.Advice{}
		if err := json.Unmarshal(advice, b.Advice); err != nil {
			return nil, fmt.Errorf("store: decode brief advice: %w", err)
		}
	}
	return &b, nil
}

// ListBriefs returns a user's latest briefs, newest first, without input
// and advice.
func (s *Store) ListBriefs(ctx context.Context, id match.PlayerID, limit int) ([]Brief, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, period_start, period_end, status, created_at FROM briefs
		WHERE steam_id = $1 ORDER BY created_at DESC LIMIT $2`, int64(id), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list briefs: %w", err)
	}
	briefs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Brief, error) {
		b := Brief{SteamID: id}
		err := row.Scan(&b.ID, &b.PeriodStart, &b.PeriodEnd, &b.Status, &b.CreatedAt)
		return b, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: list briefs: %w", err)
	}
	return briefs, nil
}

// GetBriefInput returns only the coach input of a brief.
func (s *Store) GetBriefInput(ctx context.Context, id int64) (*coach.Input, error) {
	b, err := s.GetBrief(ctx, id)
	if err != nil {
		return nil, err
	}
	return b.Input, nil
}

// SaveBriefAdvice stores coach output and marks the brief ready.
func (s *Store) SaveBriefAdvice(ctx context.Context, id int64, a *coach.Advice, coachName string) error {
	advice, err := json.Marshal(a)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE briefs SET status = $2, advice = $3, coach = $4, error = NULL, ready_at = now() WHERE id = $1`,
		id, BriefReady, advice, coachName)
	if err != nil {
		return fmt.Errorf("store: save brief advice: %w", err)
	}
	return nil
}

// FailBrief marks a brief failed with a reason.
func (s *Store) FailBrief(ctx context.Context, id int64, reason string) error {
	_, err := s.db.Exec(ctx, `UPDATE briefs SET status = $2, error = $3 WHERE id = $1`, id, BriefFailed, reason)
	if err != nil {
		return fmt.Errorf("store: fail brief: %w", err)
	}
	return nil
}

// UsersToBrief returns users who appear in a match uploaded since a time and
// have no brief yet whose period reaches coveredUntil (a manual brief made
// after the weekly moment counts).
func (s *Store) UsersToBrief(ctx context.Context, matchesSince, coveredUntil time.Time) ([]match.PlayerID, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT u.steam_id FROM users u
		JOIN match_players mp ON mp.player_id = u.steam_id
		JOIN matches m ON m.id = mp.match_id
		WHERE coalesce(m.played_at, m.uploaded_at) >= $1 AND m.report IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM briefs b WHERE b.steam_id = u.steam_id AND b.period_end >= $2)
		ORDER BY u.steam_id`, matchesSince, coveredUntil)
	if err != nil {
		return nil, fmt.Errorf("store: users to brief: %w", err)
	}
	ids, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (match.PlayerID, error) {
		var id int64
		err := row.Scan(&id)
		return match.PlayerID(id), err
	})
	if err != nil {
		return nil, fmt.Errorf("store: users to brief: %w", err)
	}
	return ids, nil
}
