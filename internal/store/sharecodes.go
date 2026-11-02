package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// SteamLink is what the share-code importer reads a user's match history
// with.
type SteamLink struct {
	SteamID  match.PlayerID
	AuthCode string
	LastCode string
	Checked  time.Time // zero if never checked
	// Error is why Steam rejected the link; the importer skips it until the
	// user saves it again.
	Error string
}

// ShareCode is a match the importer found.
type ShareCode struct {
	Code    string
	Status  string
	MatchID int64 // 0 until the demo is imported
	Reason  string
	Found   time.Time
}

// SetSteamLink saves a user's auth code and newest share code, and clears
// any earlier error so the importer checks it on its next round. The newest
// code is also stored as a found match: the importer only finds codes after
// it.
func (s *Store) SetSteamLink(ctx context.Context, id match.PlayerID, authCode, lastCode string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: set steam link: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO steam_links (steam_id, auth_code, last_code) VALUES ($1, $2, $3)
		ON CONFLICT (steam_id) DO UPDATE
		SET auth_code = excluded.auth_code, last_code = excluded.last_code, error = '', checked_at = NULL`,
		int64(id), authCode, lastCode); err != nil {
		return fmt.Errorf("store: set steam link: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO share_codes (steam_id, code) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		int64(id), lastCode); err != nil {
		return fmt.Errorf("store: set steam link: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: set steam link: %w", err)
	}
	return nil
}

// GetSteamLink returns a user's link, or ErrNotFound.
func (s *Store) GetSteamLink(ctx context.Context, id match.PlayerID) (*SteamLink, error) {
	rows, err := s.db.Query(ctx, `
		SELECT steam_id, auth_code, last_code, checked_at, error FROM steam_links WHERE steam_id = $1`, int64(id))
	if err != nil {
		return nil, fmt.Errorf("store: get steam link: %w", err)
	}
	l, err := pgx.CollectExactlyOneRow(rows, scanSteamLink)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get steam link: %w", err)
	}
	return &l, nil
}

// DeleteSteamLink stops importing a user's matches. Codes already found are
// kept.
func (s *Store) DeleteSteamLink(ctx context.Context, id match.PlayerID) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM steam_links WHERE steam_id = $1`, int64(id)); err != nil {
		return fmt.Errorf("store: delete steam link: %w", err)
	}
	return nil
}

// SteamLinksToCheck returns the links without an error that were not checked
// since before, oldest first.
func (s *Store) SteamLinksToCheck(ctx context.Context, before time.Time) ([]SteamLink, error) {
	rows, err := s.db.Query(ctx, `
		SELECT steam_id, auth_code, last_code, checked_at, error FROM steam_links
		WHERE error = '' AND (checked_at IS NULL OR checked_at < $1)
		ORDER BY checked_at NULLS FIRST, steam_id`, before)
	if err != nil {
		return nil, fmt.Errorf("store: steam links to check: %w", err)
	}
	links, err := pgx.CollectRows(rows, scanSteamLink)
	if err != nil {
		return nil, fmt.Errorf("store: steam links to check: %w", err)
	}
	return links, nil
}

func scanSteamLink(row pgx.CollectableRow) (SteamLink, error) {
	var l SteamLink
	var id int64
	var checked *time.Time
	err := row.Scan(&id, &l.AuthCode, &l.LastCode, &checked, &l.Error)
	l.SteamID = match.PlayerID(id)
	if checked != nil {
		l.Checked = *checked
	}
	return l, err
}

// RecordShareCodes stores the codes found in one check, in order, and moves
// the link's newest code to the last of them. A non-empty linkErr disables
// the link until the user saves it again. If the user changed the link while
// it was being checked, the check is dropped (stored is false): it read
// history from the old code.
func (s *Store) RecordShareCodes(ctx context.Context, link SteamLink, codes []string, linkErr string) (stored bool, err error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("store: record share codes: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	last := link.LastCode
	if len(codes) > 0 {
		last = codes[len(codes)-1]
	}
	tag, err := tx.Exec(ctx, `
		UPDATE steam_links SET last_code = $4, error = $5, checked_at = now()
		WHERE steam_id = $1 AND auth_code = $2 AND last_code = $3`,
		int64(link.SteamID), link.AuthCode, link.LastCode, last, linkErr)
	if err != nil {
		return false, fmt.Errorf("store: record share codes: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	for _, c := range codes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO share_codes (steam_id, code) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			int64(link.SteamID), c); err != nil {
			return false, fmt.Errorf("store: record share codes: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("store: record share codes: %w", err)
	}
	return true, nil
}

// ListShareCodes returns a user's found codes, newest first.
func (s *Store) ListShareCodes(ctx context.Context, id match.PlayerID, limit int) ([]ShareCode, error) {
	rows, err := s.db.Query(ctx, `
		SELECT code, status, match_id, reason, created_at FROM share_codes
		WHERE steam_id = $1 ORDER BY id DESC LIMIT $2`, int64(id), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list share codes: %w", err)
	}
	codes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ShareCode, error) {
		var c ShareCode
		var matchID *int64
		err := row.Scan(&c.Code, &c.Status, &matchID, &c.Reason, &c.Found)
		if matchID != nil {
			c.MatchID = *matchID
		}
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: list share codes: %w", err)
	}
	return codes, nil
}

// Share code statuses. A code moves waiting → resolving (the bot asks the
// Game Coordinator) → download → downloading (the api fetches the demo) →
// imported, or ends as failed, expired or over_quota.
const (
	CodeWaiting     = "waiting"
	CodeResolving   = "resolving"
	CodeDownload    = "download"
	CodeDownloading = "downloading"
	CodeImported    = "imported"
	CodeFailed      = "failed"
	CodeExpired     = "expired"
	CodeOverQuota   = "over_quota"
)

// MaxCodeAttempts is how often a code is tried, across resolving and
// downloading, before it fails.
const MaxCodeAttempts = 5

// CodeJob is a share code claimed for resolving or downloading.
type CodeJob struct {
	ID      int64
	SteamID match.PlayerID
	Code    string
	DemoURL string
	Played  time.Time // zero if unknown
}

// ClaimShareCode claims the oldest code in status from, moving it to to. A
// code that was claimed less than retryAfter ago is skipped: either someone
// works on it, or it failed and waits for a retry. Due codes that are out of
// attempts fail first, keeping their last reason. It returns ErrNotFound
// when nothing is due.
func (s *Store) ClaimShareCode(ctx context.Context, from, to string, retryAfter time.Duration) (*CodeJob, error) {
	due := `status IN ($1, $2) AND (claimed_at IS NULL OR claimed_at < now() - make_interval(secs => $3))`
	if _, err := s.db.Exec(ctx, `
		UPDATE share_codes SET status = $4,
			reason = 'Gave up after several attempts' || CASE WHEN reason <> '' THEN ': ' || reason ELSE '.' END
		WHERE `+due+` AND attempts >= $5`,
		from, to, retryAfter.Seconds(), CodeFailed, MaxCodeAttempts); err != nil {
		return nil, fmt.Errorf("store: claim share code: %w", err)
	}
	var j CodeJob
	var id int64
	var played *time.Time
	err := s.db.QueryRow(ctx, `
		UPDATE share_codes SET status = $2, claimed_at = now(), attempts = attempts + 1
		WHERE id = (
			SELECT id FROM share_codes WHERE `+due+`
			ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id, steam_id, code, demo_url, played_at`,
		from, to, retryAfter.Seconds()).Scan(&j.ID, &id, &j.Code, &j.DemoURL, &played)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: claim share code: %w", err)
	}
	j.SteamID = match.PlayerID(id)
	if played != nil {
		j.Played = *played
	}
	return &j, nil
}

// ResolveShareCode stores the demo URL the bot found and hands the code to
// the downloader with fresh attempts. It returns ErrNotFound unless the code
// is being resolved.
func (s *Store) ResolveShareCode(ctx context.Context, id int64, demoURL string, played time.Time) error {
	var p *time.Time
	if !played.IsZero() {
		p = &played
	}
	return s.moveCode(ctx, `status = $3, demo_url = $4, played_at = $5, attempts = 0, claimed_at = NULL, reason = ''`,
		id, CodeResolving, CodeDownload, demoURL, p)
}

// RetryShareCode puts a code that is in status from back to status to, to be
// claimed again after the claim's retry delay. It returns ErrNotFound if the
// code moved on meanwhile.
func (s *Store) RetryShareCode(ctx context.Context, id int64, from, to, reason string) error {
	return s.moveCode(ctx, `status = $3, reason = $4`, id, from, to, reason)
}

// ReleaseShareCode hands a claimed code back right away without using up an
// attempt, e.g. when the api shuts down in the middle of a download.
func (s *Store) ReleaseShareCode(ctx context.Context, id int64, from, to string) error {
	return s.moveCode(ctx, `status = $3, claimed_at = NULL, attempts = greatest(attempts - 1, 0)`, id, from, to)
}

// FinishShareCode moves a code that is in status from to a final status,
// with the imported match (or 0) and a reason shown to the user. It returns
// ErrNotFound if the code moved on meanwhile.
func (s *Store) FinishShareCode(ctx context.Context, id int64, from, status string, matchID int64, reason string) error {
	var m *int64
	if matchID != 0 {
		m = &matchID
	}
	return s.moveCode(ctx, `status = $3, match_id = $4, reason = $5`, id, from, status, m, reason)
}

// moveCode applies set to code id if it is in status from ($2); set's own
// arguments start at $3.
func (s *Store) moveCode(ctx context.Context, set string, id int64, from string, args ...any) error {
	tag, err := s.db.Exec(ctx, `UPDATE share_codes SET `+set+` WHERE id = $1 AND status = $2`,
		append([]any{id, from}, args...)...)
	if err != nil {
		return fmt.Errorf("store: update share code: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
