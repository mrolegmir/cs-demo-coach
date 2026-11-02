package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// UploadToken is a token a user's own tools upload demos with. Only its hash
// is stored, so the token itself is shown once, when it is created.
type UploadToken struct {
	ID       int64
	Name     string
	Created  time.Time
	LastUsed time.Time // zero if never used
}

// CreateUploadToken stores a new upload token by its hash.
func (s *Store) CreateUploadToken(ctx context.Context, id match.PlayerID, tokenHash []byte, name string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO upload_tokens (token_hash, steam_id, name) VALUES ($1, $2, $3)`,
		tokenHash, int64(id), name)
	if err != nil {
		return fmt.Errorf("store: create upload token: %w", err)
	}
	return nil
}

// UploadTokenUser returns the owner of an upload token, or ErrNotFound, and
// records that the token was used.
func (s *Store) UploadTokenUser(ctx context.Context, tokenHash []byte) (*User, error) {
	var id int64
	err := s.db.QueryRow(ctx, `
		UPDATE upload_tokens SET last_used_at = now() WHERE token_hash = $1 RETURNING steam_id`,
		tokenHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: upload token: %w", err)
	}
	return s.GetUser(ctx, match.PlayerID(id))
}

// ListUploadTokens returns a user's upload tokens, newest first.
func (s *Store) ListUploadTokens(ctx context.Context, id match.PlayerID) ([]UploadToken, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, created_at, last_used_at FROM upload_tokens
		WHERE steam_id = $1 ORDER BY created_at DESC`, int64(id))
	if err != nil {
		return nil, fmt.Errorf("store: list upload tokens: %w", err)
	}
	tokens, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (UploadToken, error) {
		var t UploadToken
		var used *time.Time
		err := row.Scan(&t.ID, &t.Name, &t.Created, &used)
		if used != nil {
			t.LastUsed = *used
		}
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: list upload tokens: %w", err)
	}
	return tokens, nil
}

// DeleteUploadToken revokes one of a user's tokens. Tokens of other users are
// left alone and reported as ErrNotFound.
func (s *Store) DeleteUploadToken(ctx context.Context, owner match.PlayerID, tokenID int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM upload_tokens WHERE id = $1 AND steam_id = $2`, tokenID, int64(owner))
	if err != nil {
		return fmt.Errorf("store: delete upload token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
