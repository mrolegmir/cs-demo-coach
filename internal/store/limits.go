package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Limits are immutable per-store admission limits. Zero means unlimited.
type Limits struct{ Uploads, Coaching int }

var ErrQuota = errors.New("store: weekly quota exhausted")

// checkQuota locks the owner until the admitting transaction commits. Every
// upload, manual brief, manual coach and automatic coach uses the same lock.
func checkQuota(ctx context.Context, tx pgx.Tx, owner match.PlayerID, limit int, upload bool) error {
	if owner == 0 {
		return nil
	}
	var admin bool
	if err := tx.QueryRow(ctx, `SELECT admin FROM users WHERE steam_id=$1 FOR UPDATE`, int64(owner)).Scan(&admin); err != nil {
		return err
	}
	if admin || limit == 0 {
		return nil
	}
	query := `SELECT count(*) FROM jobs WHERE requested_by=$1 AND kind IN ('coach','brief') AND created_at >= now()-interval '7 days'`
	if upload {
		query = `SELECT count(*) FROM matches WHERE uploaded_by=$1 AND uploaded_at >= now()-interval '7 days'`
	}
	var used int
	if err := tx.QueryRow(ctx, query, int64(owner)).Scan(&used); err != nil {
		return err
	}
	if used >= limit {
		return ErrQuota
	}
	return nil
}
