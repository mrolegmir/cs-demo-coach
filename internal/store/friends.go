package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

type FriendContact struct {
	ID       match.PlayerID
	Name     string
	Selected bool
}

func (s *Store) Friends(ctx context.Context, owner match.PlayerID) ([]match.PlayerID, error) {
	rows, err := s.db.Query(ctx, `SELECT friend_id FROM party_friends WHERE owner_id=$1 ORDER BY friend_id`, int64(owner))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (match.PlayerID, error) {
		var id int64
		err := row.Scan(&id)
		return match.PlayerID(id), err
	})
}

func (s *Store) FriendSettings(ctx context.Context, owner match.PlayerID) ([]FriendContact, bool, error) {
	var automatic bool
	if err := s.db.QueryRow(ctx, `SELECT auto_coaching FROM users WHERE steam_id=$1`, int64(owner)).Scan(&automatic); err != nil {
		return nil, false, err
	}
	rows, err := s.db.Query(ctx, `WITH recent AS (
		SELECT match_id,side FROM match_players WHERE player_id=$1 AND side IN (1,2) ORDER BY match_id DESC LIMIT 20
	), known AS (
		SELECT DISTINCT ON (mp.player_id) mp.player_id,mp.name FROM recent r
		JOIN match_players mp ON mp.match_id=r.match_id AND mp.side=r.side
		WHERE mp.player_id<>$1 ORDER BY mp.player_id,mp.match_id DESC
	), ids AS (SELECT player_id FROM known UNION SELECT friend_id FROM party_friends WHERE owner_id=$1)
	SELECT i.player_id,coalesce(k.name,''),f.friend_id IS NOT NULL FROM ids i
	LEFT JOIN known k ON k.player_id=i.player_id LEFT JOIN party_friends f ON f.owner_id=$1 AND f.friend_id=i.player_id
	ORDER BY f.friend_id IS NOT NULL DESC,lower(coalesce(k.name,'')),i.player_id`, int64(owner))
	if err != nil {
		return nil, false, err
	}
	contacts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (FriendContact, error) {
		var c FriendContact
		var id int64
		err := row.Scan(&id, &c.Name, &c.Selected)
		c.ID = match.PlayerID(id)
		return c, err
	})
	return contacts, automatic, err
}

func (s *Store) SaveFriendSettings(ctx context.Context, owner match.PlayerID, friends []match.PlayerID, automatic bool) error {
	if len(friends) > 100 {
		return errors.New("too many friends")
	}
	for _, id := range friends {
		if id == 0 || id == owner || uint64(id) > 1<<63-1 {
			return errors.New("invalid friend")
		}
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET auto_coaching=$2 WHERE steam_id=$1`, int64(owner), automatic)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM party_friends WHERE owner_id=$1`, int64(owner)); err != nil {
			return err
		}
		for _, id := range friends {
			if _, err := tx.Exec(ctx, `INSERT INTO party_friends(owner_id,friend_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, int64(owner), int64(id)); err != nil {
				return err
			}
		}
		return nil
	})
}

// PartyFor uses starting teams, never friends on the opposing side.
func PartyFor(owner match.PlayerID, players []Player, friends []match.PlayerID) []match.PlayerID {
	var side match.Side
	for _, p := range players {
		if p.ID == owner {
			side = p.Side
		}
	}
	if side != match.SideT && side != match.SideCT {
		return nil
	}
	party := []match.PlayerID{owner}
	for _, p := range players {
		if p.ID != owner && p.Side == side && slices.Contains(friends, p.ID) {
			party = append(party, p.ID)
		}
	}
	slices.Sort(party[1:])
	return party[:min(len(party), 5)]
}

// AutoCoach queues only a newly detected, unselected match. Locking the user
// serializes automatic requests against that user's rolling quota.
func (s *Store) AutoCoach(ctx context.Context, id int64, limit int) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error { return autoCoach(ctx, tx, id, limit) })
}

func autoCoach(ctx context.Context, tx pgx.Tx, id int64, limit int) error {
	var owner int64
	var status string
	var party []int64
	err := tx.QueryRow(ctx, `SELECT coalesce(uploaded_by,0),status,coalesce(party,'{}') FROM matches WHERE id=$1 `, id).Scan(&owner, &status, &party)
	if err != nil {
		return err
	}
	if owner == 0 || status != StatusDetected || len(party) > 0 {
		return nil
	}
	if err := checkQuota(ctx, tx, match.PlayerID(owner), limit, false); errors.Is(err, ErrQuota) {
		return nil
	} else if err != nil {
		return err
	}
	var automatic bool
	if err := tx.QueryRow(ctx, `SELECT auto_coaching FROM users WHERE steam_id=$1`, owner).Scan(&automatic); err != nil {
		return err
	}
	if !automatic {
		return nil
	}
	if err := tx.QueryRow(ctx, `SELECT status,coalesce(party,'{}') FROM matches WHERE id=$1 FOR UPDATE`, id).Scan(&status, &party); err != nil {
		return err
	}
	if status != StatusDetected || len(party) > 0 {
		return nil
	}
	if err := tx.QueryRow(ctx, `SELECT array_agg(mp.player_id ORDER BY (mp.player_id=$1) DESC,mp.player_id)
			FROM match_players me JOIN match_players mp ON mp.match_id=me.match_id AND mp.side=me.side
			WHERE me.match_id=$2 AND me.player_id=$1 AND me.side IN (1,2)
			AND (mp.player_id=$1 OR EXISTS(SELECT 1 FROM party_friends f WHERE f.owner_id=$1 AND f.friend_id=mp.player_id))`, owner, id).Scan(&party); err != nil {
		return err
	}
	if len(party) == 0 {
		return nil
	}
	party = party[:min(len(party), 5)]
	if _, err := tx.Exec(ctx, `UPDATE matches SET party=$2,status=$3,error=NULL WHERE id=$1`, id, party, StatusCoaching); err != nil {
		return err
	}
	if err := enqueue(ctx, tx, JobCoach, id, match.PlayerID(owner), "ru"); err != nil {
		return fmt.Errorf("auto coach: %w", err)
	}
	return nil
}
