package web

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"html/template"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/history"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// HistoryStore is what the progress page needs from the store.
type HistoryStore interface {
	PlayerMatches(ctx context.Context, id match.PlayerID) ([]store.PlayerMatch, error)
	CanViewPlayer(ctx context.Context, u *store.User, player match.PlayerID) (bool, error)
	PlayerName(ctx context.Context, id match.PlayerID) (string, error)
	ListPractice(ctx context.Context, id match.PlayerID) ([]store.Practice, error)
	AddPractice(ctx context.Context, id match.PlayerID, p store.Practice) (int64, error)
	DeletePractice(ctx context.Context, owner match.PlayerID, id int64) error
	StartPractice(ctx context.Context, owner match.PlayerID, drill string, minutes int) error
	ActivePractice(ctx context.Context, owner match.PlayerID) ([]store.PracticeSession, error)
	FinishPractice(ctx context.Context, owner match.PlayerID, id int64, minutes int) error
}

var statusText = map[history.Status]string{
	history.Active:        "Habit: work on this",
	history.Improving:     "Getting better",
	history.Resolved:      "Fixed",
	history.OneOff:        "Bad games, not a habit",
	history.Fine:          "Fine",
	history.NotEnoughData: "Not enough data yet",
}

type historyPage struct {
	Viewer
	Name    string
	SteamID match.PlayerID
	Matches int
	Issues  []historyIssue
	// Own is set when players look at their own page; only they see and
	// edit their practice log.
	Own            bool
	Practice       []practiceView
	ActivePractice []activePracticeView
	Kinds          []kindOption
	Drills         []drillGroup
	// Drill is the drill preselected in the form, from a drill card's link.
	Drill string
	Today string
	Error string
	// Window is how many matches "recent" and the trend cover.
	Window int
	// Partners is how the player and their regular teammates trade each
	// other, within TradeWindow.
	Partners    []partnerView
	TradeWindow time.Duration
	// Role is the player's role across their matches.
	Role *roleView
}

// roleView is the role line on the progress page.
type roleView struct {
	Labels                        []string
	OpeningRate, LobbyOpeningRate float64
	OpeningWinRate                float64
	Flashes10, LobbyFlashes10     float64
	AWPShare                      float64
}

var roleLabels = map[string]string{
	history.Entry:   "Entry",
	history.Support: "Support",
	history.AWPer:   "AWPer",
}

// partnerView is one teammate on the progress page.
type partnerView struct {
	Link, Name                               string
	Covered, CoveredOf, Covering, CoveringOf int
	CoveredRate, CoveringRate                float64
}

// minPartnerDeaths is how many deaths near each other a teammate needs to
// be listed: fewer say nothing.
const minPartnerDeaths = 4

// drillGroup is the library drills for one mistake, for the form.
type drillGroup struct {
	Label  string
	Drills []drills.Drill
}

type practiceView struct {
	ID      int64
	On      time.Time
	Label   string
	Drill   string // the library drill's title, if any
	Note    string
	Minutes int
}

type kindOption struct {
	Kind  detect.Kind
	Label string
}

// practiceVerdicts explain history.Effect verdicts on the progress page.
var practiceVerdicts = map[string]string{
	history.Working:    "working",
	history.NotYet:     "no change yet",
	history.TooEarly:   "too early to tell",
	history.NoBaseline: "no matches before it to compare with",
}

type historyIssue struct {
	Label, Status, StatusText string
	Rate, LobbyRate           float64
	RecentRate, RecentLobby   float64
	Count, Chances            int
	Points                    []historyPoint
	// Trend is the rolling rate against the lobby as an inline chart.
	Trend template.HTML
	// Where lists the places the mistake happened most, e.g. "B site 4".
	Where []string
	// Practice summarizes logged practice for this issue; empty without it.
	Practice string
}

type historyPoint struct {
	MatchID        int64
	Played         time.Time
	Count, Chances int
	StandOut       bool
}

func (s *Server) playerHistory(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	player := match.PlayerID(id)
	if u != nil {
		ok, err := s.Store.CanViewPlayer(r.Context(), u, player)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
	}
	pms, err := s.Store.PlayerMatches(r.Context(), player)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	name, err := s.Store.PlayerName(r.Context(), player)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	var ms []history.Match
	for _, pm := range pms {
		ms = append(ms, history.FromReport(pm.ID, pm.Played, pm.Map, player, pm.Report))
	}
	page := historyPage{
		Viewer: s.viewer(u), Name: name, SteamID: player, Matches: len(ms),
		Own:         u != nil && u.SteamID == player,
		Today:       time.Now().Format(time.DateOnly),
		Drill:       r.URL.Query().Get("drill"),
		TradeWindow: detect.DefaultTrade().Window,
		Error:       practiceErrors[r.URL.Query().Get("error")],
	}
	var log []store.Practice
	if page.Own {
		active, err := s.Store.ActivePractice(r.Context(), player)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, session := range active {
			if d, ok := drills.ByID(session.Drill); ok {
				page.ActivePractice = append(page.ActivePractice, activePracticeView{PracticeSession: session, Title: d.Title})
			}
		}
		if log, err = s.Store.ListPractice(r.Context(), player); err != nil {
			s.fail(w, r, err)
			return
		}
		for _, p := range slices.Backward(log) {
			v := practiceView{
				ID: p.ID, On: p.On, Label: cmp.Or(kindLabels[p.Kind], "General"), Note: p.Note, Minutes: p.Minutes,
			}
			if d, ok := drills.ByID(p.Drill); ok {
				v.Drill = d.Title
			}
			page.Practice = append(page.Practice, v)
		}
		for _, k := range slices.Sorted(maps.Keys(kindLabels)) {
			page.Kinds = append(page.Kinds, kindOption{Kind: k, Label: kindLabels[k]})
			page.Drills = append(page.Drills, drillGroup{Label: kindLabels[k], Drills: drills.ForKind(k)})
		}
	}
	names := map[match.PlayerID]string{}
	for _, pm := range pms {
		maps.Copy(names, pm.Names)
	}
	for _, p := range history.Partners(player, ms) {
		if p.Total() < minPartnerDeaths {
			continue
		}
		// Listing a teammate tells who the player plays with; a viewer who
		// could not open that teammate's page does not learn it here.
		if u != nil && u.SteamID != p.ID {
			ok, err := s.Store.CanViewPlayer(r.Context(), u, p.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			if !ok {
				continue
			}
		}
		page.Partners = append(page.Partners, partnerView{
			Link: fmt.Sprintf("/players/%d", p.ID), Name: cmp.Or(names[p.ID], "a teammate"),
			Covered: p.Covered.Count, CoveredOf: p.Covered.Chances, CoveredRate: p.Covered.Rate(),
			Covering: p.Covering.Count, CoveringOf: p.Covering.Chances, CoveringRate: p.Covering.Rate(),
		})
	}
	if rs := history.Roles(ms); rs.Player.Rounds > 0 {
		rv := &roleView{
			OpeningRate: rs.OpeningRate(), LobbyOpeningRate: rs.LobbyOpeningRate(), OpeningWinRate: rs.OpeningWinRate(),
			Flashes10: rs.FlashesPer10(), LobbyFlashes10: rs.LobbyFlashesPer10(), AWPShare: rs.AWPShare(),
		}
		for _, l := range rs.Labels {
			rv.Labels = append(rv.Labels, roleLabels[l])
		}
		page.Role = rv
	}
	rules := history.Default()
	page.Window = rules.RecentMatches
	for _, is := range rules.Build(ms) {
		hi := historyIssue{
			Label:       kindLabels[is.Kind],
			Status:      string(is.Status),
			StatusText:  statusText[is.Status],
			Rate:        is.Total.Rate(),
			LobbyRate:   is.Lobby.Rate(),
			RecentRate:  is.Recent.Rate(),
			RecentLobby: is.RecentLobby.Rate(),
			Count:       is.Total.Count,
			Chances:     is.Total.Chances,
		}
		var since time.Time
		if p, ok := is.Practiced(store.PracticeDays(log, is.Kind)); ok {
			hi.Practice, since = practiceSummary(p, requestLanguage(r)), p.Since
		}
		top, _ := coach.TopPlaces(is.Places, len(mapsOf(ms)) > 1)
		hi.Where = whereText(top)
		hi.Trend = trendSVG(is.Trend(rules.RecentMatches), rules.RecentMatches, since)
		for _, p := range is.PerMatch {
			hi.Points = append(hi.Points, historyPoint{
				MatchID: p.MatchID, Played: p.Played,
				Count: p.Counts.Count, Chances: p.Counts.Chances, StandOut: p.StandOut,
			})
		}
		page.Issues = append(page.Issues, hi)
	}
	s.render(w, r, "history", page)
}

// practiceSummary describes the practice logged for an issue and whether it
// is paying off.
func practiceSummary(p history.Practice, language string) string {
	if language == "ru" {
		text := translate(language, "Sessions: %d since %s", p.Sessions, p.Since.Format("02.01.2006"))
		if p.Before.Chances > 0 && p.After.Chances > 0 {
			text += translate(language, ": %.0f%% before, %.0f%% in %d matches since", p.Before.Rate()*100, p.After.Rate()*100, p.MatchesAfter)
		}
		return text + " · " + translate(language, practiceVerdicts[p.Verdict()])
	}
	text := fmt.Sprintf("%d %s since %s", p.Sessions, plural(p.Sessions, "session", "sessions"), p.Since.Format("Jan 2"))
	if p.Before.Chances > 0 && p.After.Chances > 0 {
		text += fmt.Sprintf(": %.0f%% before, %.0f%% in %d %s since", p.Before.Rate()*100, p.After.Rate()*100,
			p.MatchesAfter, plural(p.MatchesAfter, "match", "matches"))
	}
	return text + " · " + practiceVerdicts[p.Verdict()]
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

var practiceErrors = map[string]string{
	"note": "Write what you practiced (up to 300 characters).",
	"date": "Pick a date in the last year, not in the future.",
}

const maxNoteLen = 300

// practiceForm sends a drill card's "log it" link to the practice form on
// the player's own progress page, with the drill selected.
func (s *Server) practiceForm(w http.ResponseWriter, r *http.Request, u *store.User) {
	if u == nil {
		http.NotFound(w, r)
		return
	}
	to := fmt.Sprintf("/players/%d", u.SteamID)
	if d, ok := drills.ByID(r.URL.Query().Get("drill")); ok {
		to += "?drill=" + url.QueryEscape(d.ID)
	}
	http.Redirect(w, r, to+"#practice", http.StatusSeeOther)
}

// addPractice adds an entry to the signed-in player's own practice log. A
// library drill sets the mistake, and fills in the note and minutes when
// they are left empty.
func (s *Server) addPractice(w http.ResponseWriter, r *http.Request, u *store.User) {
	back := fmt.Sprintf("/players/%d", u.SteamID)
	drill, isDrill := drills.ByID(r.FormValue("drill"))
	note := strings.TrimSpace(r.FormValue("note"))
	if note == "" && isDrill {
		note = drill.Title
	}
	if note == "" || utf8.RuneCountInString(note) > maxNoteLen {
		http.Redirect(w, r, back+"?error=note#practice", http.StatusSeeOther)
		return
	}
	on, err := time.Parse(time.DateOnly, r.FormValue("on"))
	now := time.Now()
	if err != nil || on.After(now.Add(24*time.Hour)) || on.Before(now.AddDate(-1, 0, 0)) {
		http.Redirect(w, r, back+"?error=date#practice", http.StatusSeeOther)
		return
	}
	kind := detect.Kind(r.FormValue("kind"))
	if _, ok := kindLabels[kind]; !ok {
		kind = ""
	}
	minutes, err := strconv.Atoi(r.FormValue("minutes"))
	if err != nil {
		// Left empty: the drill's time, or the form's usual 20 minutes.
		minutes = 20
		if isDrill {
			minutes = drill.Minutes
		}
	}
	p := store.Practice{Kind: kind, Note: note, Minutes: min(max(minutes, 0), 600), On: on}
	if isDrill {
		p.Kind, p.Drill = drill.Kind, drill.ID
	}
	if _, err := s.Store.AddPractice(r.Context(), u.SteamID, p); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, back+"#practice", http.StatusSeeOther)
}

func (s *Server) deletePractice(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.Store.DeletePractice(r.Context(), u.SteamID, id); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/players/%d#practice", u.SteamID), http.StatusSeeOther)
}

// mapsOf returns the maps a player's matches were played on.
func mapsOf(ms []history.Match) map[string]bool {
	maps := map[string]bool{}
	for _, m := range ms {
		maps[m.Map] = true
	}
	return maps
}
