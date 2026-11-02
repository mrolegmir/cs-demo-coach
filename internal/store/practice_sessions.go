package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

type PracticeSession struct {
	ID        int64
	Drill     string
	Minutes   int
	StartedAt time.Time
}

func (s *Store) StartPractice(ctx context.Context, owner match.PlayerID, drill string, minutes int) error {
	_, err := s.db.Exec(ctx, `INSERT INTO practice_sessions (steam_id, drill, minutes) VALUES ($1, $2, $3)
		ON CONFLICT (steam_id, drill) DO NOTHING`, int64(owner), drill, minutes)
	if err != nil {
		return fmt.Errorf("store: start practice: %w", err)
	}
	return nil
}

func (s *Store) ActivePractice(ctx context.Context, owner match.PlayerID) ([]PracticeSession, error) {
	rows, err := s.db.Query(ctx, `SELECT id, drill, minutes, started_at FROM practice_sessions WHERE steam_id=$1 ORDER BY started_at, id`, int64(owner))
	if err != nil {
		return nil, fmt.Errorf("store: active practice: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PracticeSession, error) {
		var p PracticeSession
		err := row.Scan(&p.ID, &p.Drill, &p.Minutes, &p.StartedAt)
		return p, err
	})
}

// FinishPractice moves one owned session into the completed log atomically.
// Repeated finish requests cannot create duplicate completed entries.
func (s *Store) FinishPractice(ctx context.Context, owner match.PlayerID, id int64, minutes int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var drillID string
	if err := tx.QueryRow(ctx, `DELETE FROM practice_sessions WHERE id=$1 AND steam_id=$2 RETURNING drill`, id, int64(owner)).Scan(&drillID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("store: finish practice: %w", err)
	}
	d, ok := drills.ByID(drillID)
	if !ok {
		return fmt.Errorf("store: unknown session drill %q", drillID)
	}
	note := d.Title
	_, err = tx.Exec(ctx, `INSERT INTO practice (steam_id, kind, drill, note, minutes, practiced_on) VALUES ($1,$2,$3,$4,$5,$6)`, int64(owner), string(d.Kind), d.ID, note, minutes, time.Now().UTC().Format(time.DateOnly))
	if err != nil {
		return fmt.Errorf("store: complete practice: %w", err)
	}
	return tx.Commit(ctx)
}
