package brief

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// Period is how far back a brief looks for new matches.
const Period = 7 * 24 * time.Hour

// ErrNoMatches is returned when nobody in the party played in the period.
var ErrNoMatches = errors.New("brief: no matches in the period")

// Store is what briefs need from the store.
type Store interface {
	Friends(ctx context.Context, owner match.PlayerID) ([]match.PlayerID, error)
	PlayerMatches(ctx context.Context, id match.PlayerID) ([]store.PlayerMatch, error)
	ListPractice(ctx context.Context, id match.PlayerID) ([]store.Practice, error)
	CreateBrief(ctx context.Context, b *store.Brief) (int64, error)
	UsersToBrief(ctx context.Context, matchesSince, coveredUntil time.Time) ([]match.PlayerID, error)
}

// Request builds and queues a brief for owner's party over [to-Period, to).
// by is the user who asked (charged against their quota), or 0 for the
// scheduled brief. Language is snapshotted in the input for the worker.
//
// Party members are judged only on matches they played with the owner: the
// owner may not see a teammate's other matches, so the brief must not quote
// them.
func Request(ctx context.Context, st Store, owner match.PlayerID, to time.Time, by match.PlayerID, language string) (int64, error) {
	from := to.Add(-Period)
	ownerMatches, err := st.PlayerMatches(ctx, owner)
	if err != nil {
		return 0, err
	}
	shared := map[int64]store.PlayerMatch{}
	for _, pm := range ownerMatches {
		shared[pm.ID] = pm
	}
	friends, err := st.Friends(ctx, owner)
	if err != nil {
		return 0, err
	}
	party := Party(owner, ownerMatches, from, to, friends)
	ownerPractice, err := st.ListPractice(ctx, owner)
	if err != nil {
		return 0, err
	}
	members := []Member{{ID: owner, Matches: ownerMatches, Practice: ownerPractice, ShareNotes: true}}
	for _, p := range party[1:] {
		ms, err := st.PlayerMatches(ctx, p)
		if err != nil {
			return 0, err
		}
		ms = slices.DeleteFunc(ms, func(pm store.PlayerMatch) bool {
			own := shared[pm.ID]
			side := own.Sides[owner]
			return side == match.SideNone || pm.Sides[p] != side || (own.ManualParty && !slices.Contains(own.Party, p))
		})
		log, err := st.ListPractice(ctx, p)
		if err != nil {
			return 0, err
		}
		members = append(members, Member{ID: p, Matches: ms, Practice: log})
	}
	in, ok := Input(members, from, to)
	if !ok {
		return 0, ErrNoMatches
	}
	in.Language = coach.Language(language)
	return st.CreateBrief(ctx, &store.Brief{
		SteamID: owner, PeriodStart: from, PeriodEnd: to, Party: party, Input: in, RequestedBy: by,
	})
}

// Schedule is the weekly moment briefs are made, e.g. Friday 15:00.
type Schedule struct {
	Weekday  time.Weekday
	Hour     int
	Location *time.Location
}

// Last returns the most recent scheduled moment at or before now.
func (s Schedule) Last(now time.Time) time.Time {
	now = now.In(s.Location)
	slot := time.Date(now.Year(), now.Month(), now.Day(), s.Hour, 0, 0, 0, s.Location)
	slot = slot.AddDate(0, 0, -int((7+now.Weekday()-s.Weekday)%7))
	if slot.After(now) {
		slot = slot.AddDate(0, 0, -7)
	}
	return slot
}

// Planner makes the weekly briefs.
type Planner struct {
	Store    Store
	Schedule Schedule
	Log      *slog.Logger
	// Every is how often to check whether briefs are due.
	Every time.Duration
}

// Run checks for due briefs until ctx is canceled.
func (p *Planner) Run(ctx context.Context) {
	for {
		if err := p.RunOnce(ctx, time.Now()); err != nil && ctx.Err() == nil {
			p.Log.Error("weekly briefs", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.Every):
		}
	}
}

// RunOnce creates the briefs due at now: one per user who appeared in a
// match during the week before the last scheduled moment and has no brief
// covering it yet.
func (p *Planner) RunOnce(ctx context.Context, now time.Time) error {
	slot := p.Schedule.Last(now)
	users, err := p.Store.UsersToBrief(ctx, slot.Add(-Period), slot)
	if err != nil {
		return err
	}
	var errs []error
	for _, u := range users {
		id, err := Request(ctx, p.Store, u, slot, 0, "ru")
		switch {
		case errors.Is(err, ErrNoMatches):
		case err != nil:
			errs = append(errs, err)
		default:
			p.Log.Info("weekly brief queued", "user", u, "brief", id)
		}
	}
	return errors.Join(errs...)
}
