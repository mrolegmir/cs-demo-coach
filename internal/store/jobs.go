package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// ErrNoJob is returned by ClaimJob when the queue is empty.
var ErrNoJob = errors.New("store: no job")

// Job is a claimed unit of work.
type Job struct {
	ID       int64
	Kind     string
	MatchID  int64 // detect and coach jobs
	BriefID  int64 // brief jobs
	Attempts int
	Language string
}

func enqueue(ctx context.Context, tx pgx.Tx, kind string, matchID int64, by match.PlayerID, language string) error {
	_, err := tx.Exec(ctx, `INSERT INTO jobs (kind, match_id, requested_by, language) VALUES ($1, $2, $3, $4)`, kind, matchID, nullID(by), language)
	return err
}

// nullID maps the zero player ID to SQL NULL.
func nullID(id match.PlayerID) *int64 {
	if id == 0 {
		return nil
	}
	v := int64(id)
	return &v
}

// ClaimJob takes the oldest runnable job and marks it running. Jobs left
// running for longer than stale (a crashed worker) become claimable again.
// Concurrent workers never claim the same job.
func (s *Store) ClaimJob(ctx context.Context, stale time.Duration) (*Job, error) {
	var j Job
	err := s.db.QueryRow(ctx, `
		UPDATE jobs SET status = 'running', locked_at = now(), attempts = attempts + 1
		WHERE id = (
			SELECT id FROM jobs
			WHERE (status = 'queued' AND run_after <= now())
			   OR (status = 'running' AND locked_at < now() - $1::interval)
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, kind, coalesce(match_id, 0), coalesce(brief_id, 0), attempts, language`, stale).
		Scan(&j.ID, &j.Kind, &j.MatchID, &j.BriefID, &j.Attempts, &j.Language)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoJob
	}
	if err != nil {
		return nil, fmt.Errorf("store: claim job: %w", err)
	}
	return &j, nil
}
