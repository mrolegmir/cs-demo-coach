package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

var ErrLease = errors.New("store: job attempt no longer owns the lease")

// withAttempt derives the target from the locked job, never from client input.
// A successful commit may lose its HTTP response: repeating it is a no-op.
func (s *Store) withAttempt(ctx context.Context, claim *Job, completing bool, apply func(pgx.Tx, *Job) error) error {
	if claim == nil || claim.Attempts < 1 {
		return ErrLease
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var j Job
		var status string
		err := tx.QueryRow(ctx, `SELECT id,kind,coalesce(match_id,0),coalesce(brief_id,0),attempts,status FROM jobs WHERE id=$1 FOR UPDATE`, claim.ID).Scan(&j.ID, &j.Kind, &j.MatchID, &j.BriefID, &j.Attempts, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLease
		}
		if err != nil {
			return err
		}
		if j.Attempts != claim.Attempts {
			return ErrLease
		}
		if status == "done" && completing {
			return nil
		}
		if status != "running" {
			return ErrLease
		}
		if err := apply(tx, &j); err != nil {
			return err
		}
		if completing {
			_, err = tx.Exec(ctx, `UPDATE jobs SET status='done',error=NULL WHERE id=$1`, j.ID)
		}
		return err
	})
}

// FinishDetection commits the report, optional automatic coaching, and job
// completion together. Direct and HTTP workers use this same operation.
func (s *Store) FinishDetection(ctx context.Context, claim *Job, m *match.Match, rep detect.Report) error {
	if m == nil {
		return errors.New("store: missing parsed match")
	}
	return s.withAttempt(ctx, claim, true, func(tx pgx.Tx, j *Job) error {
		if j.Kind != JobDetect {
			return ErrLease
		}
		var owner int64
		if err := tx.QueryRow(ctx, `SELECT coalesce(uploaded_by,0) FROM matches WHERE id=$1`, j.MatchID).Scan(&owner); err != nil {
			return err
		}
		// Owner before match, as in manual admission and automatic coaching.
		if err := checkQuota(ctx, tx, match.PlayerID(owner), 0, false); err != nil {
			return err
		}
		if err := saveDetection(ctx, tx, j.MatchID, m, rep); err != nil {
			return err
		}
		return autoCoach(ctx, tx, j.MatchID, s.limits.Coaching)
	})
}

// FinishAdvice saves the result only for the current job generation.
func (s *Store) FinishAdvice(ctx context.Context, claim *Job, a *coach.Advice, name string) error {
	if a == nil {
		return errors.New("store: missing advice")
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return s.withAttempt(ctx, claim, true, func(tx pgx.Tx, j *Job) error {
		switch j.Kind {
		case JobCoach:
			_, err = tx.Exec(ctx, `UPDATE matches SET status=$2,advice=$3,coach=$4,error=NULL WHERE id=$1`, j.MatchID, StatusCoached, b, name)
		case JobBrief:
			_, err = tx.Exec(ctx, `UPDATE briefs SET status=$2,advice=$3,coach=$4,error=NULL,ready_at=now() WHERE id=$1`, j.BriefID, BriefReady, b, name)
		default:
			return ErrLease
		}
		return err
	})
}

// FailAttempt atomically retries or terminates the job and its target.
func (s *Store) FailAttempt(ctx context.Context, claim *Job, retry bool, after time.Duration, reason string) error {
	return s.withAttempt(ctx, claim, false, func(tx pgx.Tx, j *Job) error {
		if retry {
			_, err := tx.Exec(ctx, `UPDATE jobs SET status='queued',run_after=now()+$2::interval,error=$3 WHERE id=$1`, j.ID, after, reason)
			return err
		}
		var err error
		switch j.Kind {
		case JobDetect:
			// A pre-v2 job may already have saved a report before crashing.
			_, err = tx.Exec(ctx, `UPDATE matches SET status=$2,error=$3 WHERE id=$1 AND report IS NULL`, j.MatchID, StatusFailed, reason)
		case JobCoach:
			_, err = tx.Exec(ctx, `UPDATE matches SET status=$2,error=$3 WHERE id=$1 AND status=$4`, j.MatchID, StatusDetected, reason, StatusCoaching)
		case JobBrief:
			_, err = tx.Exec(ctx, `UPDATE briefs SET status=$2,error=$3 WHERE id=$1`, j.BriefID, BriefFailed, reason)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE jobs SET status='failed',error=$2 WHERE id=$1`, j.ID, reason)
		return err
	})
}

// PendingDemoCleanup lists files whose detection is durably complete.
func (s *Store) PendingDemoCleanup(ctx context.Context) ([]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT match_id FROM jobs WHERE kind='detect' AND status='done' AND NOT demo_deleted ORDER BY match_id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

func (s *Store) MarkDemoDeleted(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `UPDATE jobs SET demo_deleted=true WHERE match_id=$1 AND kind='detect' AND status='done'`, id)
	return err
}
