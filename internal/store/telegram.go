package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// TelegramLink is the Telegram chat a user's briefs go to.
type TelegramLink struct {
	SteamID match.PlayerID
	ChatID  int64
	Created time.Time
}

// CreateTelegramCode stores a one-time code that links a chat to a user,
// replacing the user's earlier codes.
func (s *Store) CreateTelegramCode(ctx context.Context, id match.PlayerID, codeHash []byte, expires time.Time) error {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM telegram_codes WHERE steam_id = $1 OR expires_at < now()`, int64(id)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO telegram_codes (code_hash, steam_id, expires_at) VALUES ($1, $2, $3)`,
			codeHash, int64(id), expires)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: create telegram code: %w", err)
	}
	return nil
}

// UseTelegramCode links a chat to the owner of an unexpired code and spends
// the code. A chat belongs to one user at a time, so linking it again moves
// it. It returns ErrNotFound for an unknown or expired code.
func (s *Store) UseTelegramCode(ctx context.Context, codeHash []byte, chatID int64) (match.PlayerID, error) {
	var id int64
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `DELETE FROM telegram_codes WHERE code_hash = $1 AND expires_at > now() RETURNING steam_id`,
			codeHash).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM telegram_links WHERE chat_id = $1 AND steam_id <> $2`, chatID, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO telegram_links (steam_id, chat_id) VALUES ($1, $2)
			ON CONFLICT (steam_id) DO UPDATE SET chat_id = excluded.chat_id, created_at = now()`, id, chatID)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("store: use telegram code: %w", err)
	}
	return match.PlayerID(id), nil
}

// GetTelegramLink returns a user's Telegram link, or ErrNotFound.
func (s *Store) GetTelegramLink(ctx context.Context, id match.PlayerID) (*TelegramLink, error) {
	l := TelegramLink{SteamID: id}
	err := s.db.QueryRow(ctx, `SELECT chat_id, created_at FROM telegram_links WHERE steam_id = $1`, int64(id)).
		Scan(&l.ChatID, &l.Created)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get telegram link: %w", err)
	}
	return &l, nil
}

// DeleteTelegramLink unlinks a user's chat.
func (s *Store) DeleteTelegramLink(ctx context.Context, id match.PlayerID) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM telegram_links WHERE steam_id = $1`, int64(id)); err != nil {
		return fmt.Errorf("store: delete telegram link: %w", err)
	}
	return nil
}

// DeleteTelegramChat unlinks a chat, for /stop or when the user blocked the
// bot. It reports whether the chat was linked.
func (s *Store) DeleteTelegramChat(ctx context.Context, chatID int64) (bool, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM telegram_links WHERE chat_id = $1`, chatID)
	if err != nil {
		return false, fmt.Errorf("store: delete telegram chat: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// BriefNotice is a ready brief waiting to be sent to its owner's chat. The
// brief has its period and advice, not its input.
type BriefNotice struct {
	Brief  Brief
	ChatID int64
}

// BriefsToNotify returns briefs that became ready since a time, were not
// sent yet, and became ready after their owner linked a chat, so linking a
// chat does not send old briefs.
func (s *Store) BriefsToNotify(ctx context.Context, since time.Time) ([]BriefNotice, error) {
	rows, err := s.db.Query(ctx, `
		SELECT b.id, b.steam_id, b.period_start, b.period_end, b.advice, l.chat_id FROM briefs b
		JOIN telegram_links l ON l.steam_id = b.steam_id
		WHERE b.status = $1 AND b.notified_at IS NULL AND b.ready_at >= $2 AND b.ready_at >= l.created_at
		ORDER BY b.id`, BriefReady, since)
	if err != nil {
		return nil, fmt.Errorf("store: briefs to notify: %w", err)
	}
	ns, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BriefNotice, error) {
		var n BriefNotice
		var steam int64
		err := row.Scan(&n.Brief.ID, &steam, &n.Brief.PeriodStart, &n.Brief.PeriodEnd, &n.Brief.Advice, &n.ChatID)
		n.Brief.SteamID = match.PlayerID(steam)
		return n, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: briefs to notify: %w", err)
	}
	return ns, nil
}

// MarkBriefNotified records that a brief was sent.
func (s *Store) MarkBriefNotified(ctx context.Context, id int64) error {
	if _, err := s.db.Exec(ctx, `UPDATE briefs SET notified_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("store: mark brief notified: %w", err)
	}
	return nil
}
