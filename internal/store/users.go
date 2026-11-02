package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// ErrInvalidInvite is returned for an unknown, used or expired invite.
var ErrInvalidInvite = errors.New("store: invalid invite")

// User is a Steam account that may use the service.
type User struct {
	SteamID match.PlayerID
	Admin   bool
	// Name is the player's most recent in-game name from an analyzed match,
	// or empty if they have not appeared in one yet.
	Name string
}

// EnsureAdmin creates the user if needed and makes them an admin.
func (s *Store) EnsureAdmin(ctx context.Context, id match.PlayerID) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO users (steam_id, admin) VALUES ($1, true)
		ON CONFLICT (steam_id) DO UPDATE SET admin = true`, int64(id))
	if err != nil {
		return fmt.Errorf("store: ensure admin: %w", err)
	}
	return nil
}

// GetUser returns a user, or ErrNotFound if the account was never let in.
func (s *Store) GetUser(ctx context.Context, id match.PlayerID) (*User, error) {
	u := User{SteamID: id}
	err := s.db.QueryRow(ctx, `
		SELECT u.admin, coalesce((
			SELECT mp.name FROM match_players mp WHERE mp.player_id = u.steam_id
			ORDER BY mp.match_id DESC LIMIT 1), '')
		FROM users u WHERE u.steam_id = $1`, int64(id)).Scan(&u.Admin, &u.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get user: %w", err)
	}
	return &u, nil
}

// RedeemInvite lets a Steam account in with an unused, unexpired invite.
// Redeeming as an existing user is a no-op that keeps the invite unused.
func (s *Store) RedeemInvite(ctx context.Context, code string, id match.PlayerID) error {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE steam_id = $1)`, int64(id)).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		var by int64
		err := tx.QueryRow(ctx, `
			SELECT created_by FROM invites
			WHERE code = $1 AND used_by IS NULL AND expires_at > now()
			FOR UPDATE`, code).Scan(&by)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidInvite
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO users (steam_id, invited_by) VALUES ($1, $2)`, int64(id), by); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE invites SET used_by = $2 WHERE code = $1`, code, int64(id))
		return err
	})
	if errors.Is(err, ErrInvalidInvite) {
		return err
	}
	if err != nil {
		return fmt.Errorf("store: redeem invite: %w", err)
	}
	return nil
}

// Invite is a one-time link to let a friend in.
type Invite struct {
	Code      string
	UsedBy    match.PlayerID
	ExpiresAt time.Time
}

// CreateInvite stores a new invite code from a user.
func (s *Store) CreateInvite(ctx context.Context, by match.PlayerID, code string, ttl time.Duration) error {
	_, err := s.db.Exec(ctx, `INSERT INTO invites (code, created_by, expires_at) VALUES ($1, $2, now() + $3::interval)`,
		code, int64(by), ttl)
	if err != nil {
		return fmt.Errorf("store: create invite: %w", err)
	}
	return nil
}

// ListInvites returns a user's invites, newest first.
func (s *Store) ListInvites(ctx context.Context, by match.PlayerID) ([]Invite, error) {
	rows, err := s.db.Query(ctx, `
		SELECT code, coalesce(used_by, 0), expires_at FROM invites
		WHERE created_by = $1 ORDER BY created_at DESC LIMIT 50`, int64(by))
	if err != nil {
		return nil, fmt.Errorf("store: list invites: %w", err)
	}
	invites, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Invite, error) {
		var inv Invite
		var used int64
		err := row.Scan(&inv.Code, &used, &inv.ExpiresAt)
		inv.UsedBy = match.PlayerID(used)
		return inv, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: list invites: %w", err)
	}
	return invites, nil
}

// CreateSession stores a session by the hash of its cookie value.
func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, id match.PlayerID, ttl time.Duration) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sessions (token_hash, steam_id, expires_at) VALUES ($1, $2, now() + $3::interval)`,
		tokenHash, int64(id), ttl)
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	// Opportunistic cleanup keeps the table small without a cron job.
	_, _ = s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return nil
}

// SessionUser returns the user of a live session, or ErrNotFound.
func (s *Store) SessionUser(ctx context.Context, tokenHash []byte) (*User, error) {
	var id int64
	err := s.db.QueryRow(ctx, `SELECT steam_id FROM sessions WHERE token_hash = $1 AND expires_at > now()`, tokenHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: session: %w", err)
	}
	return s.GetUser(ctx, match.PlayerID(id))
}

// DeleteSession ends a session.
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// Usage is how much of their quota a user spent since a point in time.
type Usage struct {
	Uploads  int
	Coaching int
}

// GetUsage counts a user's uploads and coaching requests since a time.
func (s *Store) GetUsage(ctx context.Context, id match.PlayerID, since time.Time) (Usage, error) {
	var u Usage
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM matches WHERE uploaded_by = $1 AND uploaded_at >= $2),
			(SELECT count(*) FROM jobs WHERE requested_by = $1 AND kind IN ($3, $4) AND created_at >= $2)`,
		int64(id), since, JobCoach, JobBrief).Scan(&u.Uploads, &u.Coaching)
	if err != nil {
		return Usage{}, fmt.Errorf("store: usage: %w", err)
	}
	return u, nil
}

// CanView reports whether a user may see a match: admins see everything,
// others see matches they uploaded or played in.
func (s *Store) CanView(ctx context.Context, matchID int64, u *User) (bool, error) {
	if u.Admin {
		return true, nil
	}
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM matches WHERE id = $1 AND uploaded_by = $2)
		    OR EXISTS (SELECT 1 FROM match_players WHERE match_id = $1 AND player_id = $2)`,
		matchID, int64(u.SteamID)).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("store: can view: %w", err)
	}
	return ok, nil
}

// PlayerMatch is an analyzed match a player appeared in.
type PlayerMatch struct {
	Party       []match.PlayerID
	ManualParty bool
	ID          int64
	// Played is when the match was played, or its upload time if unknown.
	Played time.Time
	Map    string
	Report detect.Report
	// Names maps every player of the match to their in-game name.
	Names map[match.PlayerID]string
	// Sides maps every player to their side in the first round, which tells
	// teammates from opponents.
	Sides map[match.PlayerID]match.Side
}

// PlayerMatches returns the analyzed matches a player appeared in, oldest
// first.
func (s *Store) PlayerMatches(ctx context.Context, id match.PlayerID) ([]PlayerMatch, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.id, coalesce(m.played_at, m.uploaded_at), coalesce(m.map, ''), m.report,coalesce(m.party,'{}'),m.party_manual
		FROM matches m JOIN match_players mp ON mp.match_id = m.id
		WHERE mp.player_id = $1 AND m.report IS NOT NULL
		ORDER BY coalesce(m.played_at, m.uploaded_at), m.id`, int64(id))
	if err != nil {
		return nil, fmt.Errorf("store: player matches: %w", err)
	}
	ms, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (PlayerMatch, error) {
		pm := PlayerMatch{Names: map[match.PlayerID]string{}, Sides: map[match.PlayerID]match.Side{}}
		var report []byte
		var party []int64
		if err := row.Scan(&pm.ID, &pm.Played, &pm.Map, &report, &party, &pm.ManualParty); err != nil {
			return pm, err
		}
		for _, id := range party {
			pm.Party = append(pm.Party, match.PlayerID(id))
		}
		return pm, json.Unmarshal(report, &pm.Report)
	})
	if err != nil {
		return nil, fmt.Errorf("store: player matches: %w", err)
	}

	byID := map[int64]*PlayerMatch{}
	ids := make([]int64, len(ms))
	for i := range ms {
		byID[ms[i].ID], ids[i] = &ms[i], ms[i].ID
	}
	rows, err = s.db.Query(ctx, `SELECT match_id, player_id, name, side FROM match_players WHERE match_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("store: player match names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var mid, pid int64
		var name string
		var side int16
		if err := rows.Scan(&mid, &pid, &name, &side); err != nil {
			return nil, fmt.Errorf("store: player match names: %w", err)
		}
		byID[mid].Names[match.PlayerID(pid)] = name
		byID[mid].Sides[match.PlayerID(pid)] = match.Side(side)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: player match names: %w", err)
	}
	return ms, nil
}

// CanViewPlayer reports whether a user may see a player's history: their
// own, any for admins, and players they share a match with.
func (s *Store) CanViewPlayer(ctx context.Context, u *User, player match.PlayerID) (bool, error) {
	if u.Admin || u.SteamID == player {
		return true, nil
	}
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM match_players a JOIN match_players b ON a.match_id = b.match_id
			WHERE a.player_id = $1 AND b.player_id = $2)`, int64(u.SteamID), int64(player)).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("store: can view player: %w", err)
	}
	return ok, nil
}

// PlayerName returns a player's most recent in-game name, or "".
func (s *Store) PlayerName(ctx context.Context, id match.PlayerID) (string, error) {
	var name string
	err := s.db.QueryRow(ctx, `SELECT name FROM match_players WHERE player_id = $1 ORDER BY match_id DESC LIMIT 1`, int64(id)).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: player name: %w", err)
	}
	return name, nil
}
