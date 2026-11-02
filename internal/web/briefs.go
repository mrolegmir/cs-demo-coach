package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/brief"
	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// BriefStore is what the brief pages need from the store.
type BriefStore interface {
	brief.Store
	GetBrief(ctx context.Context, id int64) (*store.Brief, error)
	ListBriefs(ctx context.Context, id match.PlayerID, limit int) ([]store.Brief, error)
}

type briefPage struct {
	Viewer
	Brief   *store.Brief
	Notice  string
	Players []playerView
	Team    []teamAdviceView
	Notes   []string
}

// Pending reports whether the page should refresh itself.
func (p briefPage) Pending() bool { return p.Brief.Status == store.BriefQueued }

var briefNotices = map[string]string{
	"none":  "None of your party's matches from the last 7 days are analyzed yet. Upload a demo first.",
	"quota": "You have used this week's coaching requests.",
}

func (s *Server) showBrief(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := s.Store.GetBrief(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if u != nil && !u.Admin && b.SteamID != u.SteamID {
		http.NotFound(w, r)
		return
	}
	page := briefPage{Viewer: s.viewer(u), Brief: b}
	if b.Advice != nil {
		evidence := func(ids []string) []findingView {
			var out []findingView
			for _, id := range ids {
				if e, ok := b.Input.Evidence[id]; ok {
					_, fid, _ := strings.Cut(id, "-")
					out = append(out, findingView{
						ID: id, Round: e.Round, Time: e.Time, Player: e.Player,
						Kind: kindLabels[e.Kind], Place: e.Place, Detail: e.Detail,
						Link: fmt.Sprintf("/matches/%d#%s", e.Match, fid),
					})
				}
			}
			return out
		}
		places := map[string]map[detect.Kind][]coach.Place{}
		for _, ps := range b.Input.Stats {
			places[ps.Player] = map[detect.Kind][]coach.Place{}
			for _, is := range ps.Issues {
				places[ps.Player][is.Kind] = is.Places
			}
		}
		for _, pa := range b.Advice.Players {
			pv := playerView{Name: pa.Player}
			for _, f := range pa.Focus {
				pv.Focus = append(pv.Focus, newFocusView(f, evidence(f.Evidence), places[pa.Player][f.Kind]))
			}
			page.Players = append(page.Players, pv)
		}
		for _, t := range b.Advice.Team {
			page.Team = append(page.Team, teamAdviceView{Title: t.Title, Why: t.Why, Suggestion: t.Suggestion, Evidence: evidence(t.Evidence)})
		}
		page.Notes = b.Advice.Notes
	}
	s.render(w, r, "brief", page)
}

func (s *Server) requestBrief(w http.ResponseWriter, r *http.Request, u *store.User) {
	if u == nil {
		// Briefs belong to accounts; without sign-in there is nobody to brief.
		http.NotFound(w, r)
		return
	}
	q, err := s.quota(r.Context(), u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if q != nil && q.CoachingLeft() <= 0 {
		http.Redirect(w, r, "/?brief=quota", http.StatusSeeOther)
		return
	}
	id, err := brief.Request(r.Context(), s.Store, u.SteamID, time.Now(), u.SteamID, requestLanguage(r))
	if errors.Is(err, store.ErrQuota) {
		http.Redirect(w, r, "/?brief=quota", http.StatusSeeOther)
		return
	}
	if errors.Is(err, brief.ErrNoMatches) {
		http.Redirect(w, r, "/?brief=none", http.StatusSeeOther)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/briefs/%d", id), http.StatusSeeOther)
}
