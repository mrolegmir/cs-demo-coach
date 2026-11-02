package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Practice is one entry of a player's practice log.
type Practice struct {
	ID int64
	// Kind is the mistake the practice targets, or empty for general
	// practice.
	Kind detect.Kind
	// Drill is the library drill practiced (see package drills), or empty.
	Drill   string
	Note    string
	Minutes int
	// On is the day of the practice, at midnight UTC.
	On time.Time
}

// PracticeDays returns the days of the entries for one kind of mistake.
func PracticeDays(log []Practice, kind detect.Kind) []time.Time {
	var days []time.Time
	for _, p := range log {
		if p.Kind == kind {
			days = append(days, p.On)
		}
	}
	return days
}

// AddPractice adds an entry to a user's practice log.
func (s *Store) AddPractice(ctx context.Context, id match.PlayerID, p Practice) (int64, error) {
	var pid int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO practice (steam_id, kind, drill, note, minutes, practiced_on) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		int64(id), string(p.Kind), p.Drill, p.Note, p.Minutes, p.On).Scan(&pid)
	if err != nil {
		return 0, fmt.Errorf("store: add practice: %w", err)
	}
	return pid, nil
}

// ListPractice returns a user's practice log, oldest first.
func (s *Store) ListPractice(ctx context.Context, id match.PlayerID) ([]Practice, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, kind, drill, note, minutes, practiced_on FROM practice
		WHERE steam_id = $1 ORDER BY practiced_on, id`, int64(id))
	if err != nil {
		return nil, fmt.Errorf("store: list practice: %w", err)
	}
	log, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Practice, error) {
		var p Practice
		var kind string
		err := row.Scan(&p.ID, &kind, &p.Drill, &p.Note, &p.Minutes, &p.On)
		p.Kind = detect.Kind(kind)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: list practice: %w", err)
	}
	return log, nil
}

// DeletePractice removes an entry from a user's own log. Entries of other
// users are left alone and reported as ErrNotFound.
func (s *Store) DeletePractice(ctx context.Context, owner match.PlayerID, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM practice WHERE id = $1 AND steam_id = $2`, id, int64(owner))
	if err != nil {
		return fmt.Errorf("store: delete practice: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
