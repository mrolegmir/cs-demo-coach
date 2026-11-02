// Package web serves the upload and report pages, Steam sign-in, and the
// internal endpoints workers use.
package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// Store is the part of store.Store the server needs.
type Store interface {
	CreateMatch(ctx context.Context, u store.Upload) (int64, error)
	ListMatches(ctx context.Context, u *store.User, limit int) ([]store.Match, error)
	GetMatch(ctx context.Context, id int64) (*store.Match, error)
	GetAnalysis(ctx context.Context, id int64) (*store.Analysis, error)
	RequestCoaching(ctx context.Context, id int64, party []match.PlayerID, by match.PlayerID, language string) error
	FailMatch(ctx context.Context, id int64, reason string) error
	CanView(ctx context.Context, matchID int64, u *store.User) (bool, error)
	GetUsage(ctx context.Context, id match.PlayerID, since time.Time) (store.Usage, error)
	UserStore
	TokenStore
	SteamLinkStore
	ImportStore
	TelegramStore
	HistoryStore
	BriefStore
	FriendStore
}

// MaxDemoSize is the largest accepted upload.
const MaxDemoSize = 1 << 30

// demoMagic starts every CS2 demo file.
var demoMagic = []byte("PBDEMS2\x00")

//go:embed templates/*.html
var templateFS embed.FS

// Server is the HTTP API and UI.
type Server struct {
	Store   Store
	DemoDir string
	// WorkerToken authorizes workers on /internal endpoints.
	WorkerToken string
	// Jobs, when set, exposes the job queue to remote workers.
	Jobs JobStore
	// Auth, when set, requires Steam sign-in for every page. Without it the
	// server runs open, which is meant for local use only.
	Auth *Auth
	// ShareCodes shows the Steam match history form; set it when the
	// share-code importer runs.
	ShareCodes bool
	// ImportHTTP downloads imported demos; nil means http.DefaultClient.
	ImportHTTP *http.Client
	// TelegramBot, when set, returns the bot's username once briefs can be
	// sent to Telegram, and "" until then.
	TelegramBot func() string
	Log         *slog.Logger

	pages map[string]*template.Template
}

// Viewer is embedded in every page's data so the layout can show who is
// signed in.
type Viewer struct {
	User   *store.User
	AuthOn bool
}

// Handler returns the server's routes.
func (s *Server) Handler() (http.Handler, error) {
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /language", s.setLanguage)
	mux.HandleFunc("GET /{$}", s.page(s.index))
	mux.HandleFunc("POST /matches", s.page(s.upload))
	mux.HandleFunc("GET /matches/{id}", s.page(s.match))
	mux.HandleFunc("GET /matches/{id}/status", s.page(s.matchStatus))
	mux.HandleFunc("POST /matches/{id}/coach", s.page(s.requestCoaching))
	mux.HandleFunc("GET /players/{id}", s.page(s.playerHistory))
	mux.HandleFunc("GET /briefs/{id}", s.page(s.showBrief))
	mux.HandleFunc("POST /briefs", s.page(s.requestBrief))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
	mux.HandleFunc("GET /internal/demos/{id}", s.worker(s.serveDemo))
	mux.HandleFunc("DELETE /internal/demos/{id}", s.worker(workerUpgradeRequired))
	s.routeAuth(mux)
	s.routeInternal(mux)
	s.routeImports(mux)
	s.routeTelegram(mux)
	s.routeAPI(mux)
	return mux, nil
}

func (s *Server) parseTemplates() error {
	funcs := template.FuncMap{
		"tr":         func(s string, args ...any) string { return translate("en", s, args...) },
		"lang":       func() string { return "ru" },
		"returnURL":  func() string { return "/" },
		"clock":      match.Clock,
		"pct":        func(x float64) string { return fmt.Sprintf("%.0f%%", x*100) },
		"inc":        func(i int) int { return i + 1 },
		"used":       lastUsed,
		"mask":       maskCode,
		"codeStatus": codeStatus,
	}
	s.pages = map[string]*template.Template{}
	for _, page := range []string{"index", "match", "login", "invites", "history", "brief", "tokens", "friends"} {
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/"+page+".html")
		if err != nil {
			return fmt.Errorf("web: parse %s template: %w", page, err)
		}
		s.pages[page] = t
	}
	return nil
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, data any) {
	var buf bytes.Buffer
	t, err := s.pages[page].Clone()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	language := requestLanguage(r)
	t.Funcs(template.FuncMap{
		"tr":        func(text string, args ...any) string { return translate(language, text, args...) },
		"lang":      func() string { return language },
		"returnURL": func() string { return r.URL.RequestURI() },
	})
	w.Header().Set("Content-Language", language)
	w.Header().Add("Vary", "Cookie")
	if err := t.Execute(&buf, data); err != nil {
		s.Log.Error("render", "page", page, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	s.Log.Error("request failed", "path", r.URL.Path, "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func (s *Server) viewer(u *store.User) Viewer { return Viewer{User: u, AuthOn: s.Auth != nil} }

// userID is the signed-in user's SteamID, or 0 when auth is off.
func userID(u *store.User) match.PlayerID {
	if u == nil {
		return 0
	}
	return u.SteamID
}

func (s *Server) index(w http.ResponseWriter, r *http.Request, u *store.User) {
	matches, err := s.Store.ListMatches(r.Context(), u, 50)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := struct {
		Viewer
		Matches     []matchListView
		Error       string
		BriefNotice string
		Briefs      []store.Brief
		Quota       *quotaView
		Telegram    *telegramView
	}{
		Viewer: s.viewer(u),
		Error:  uploadErrors[r.URL.Query().Get("error")], BriefNotice: briefNotices[r.URL.Query().Get("brief")],
	}
	for _, m := range matches {
		data.Matches = append(data.Matches, matchListView{Match: m, Result: resultFor(m.RoundResults, userID(u))})
	}
	if data.Quota, err = s.quota(r.Context(), u); err != nil {
		s.fail(w, r, err)
		return
	}
	if u != nil {
		if data.Briefs, err = s.Store.ListBriefs(r.Context(), u.SteamID, 5); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if data.Telegram, err = s.telegramState(r.Context(), u); err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "index", data)
}

// upload streams a demo to disk without holding it in memory, checks that it
// is a CS2 demo and queues it for analysis.
func (s *Server) upload(w http.ResponseWriter, r *http.Request, u *store.User) {
	q, err := s.quota(r.Context(), u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if q != nil && q.UploadsLeft() <= 0 {
		uploadError(w, r, "quota")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxDemoSize+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		uploadError(w, r, "nofile")
		return
	}
	var part io.ReadCloser
	var filename string
	for {
		p, err := mr.NextPart()
		if err != nil {
			uploadError(w, r, "nofile")
			return
		}
		if p.FormName() == "demo" {
			part, filename = p, filepath.Base(p.FileName())
			break
		}
	}
	defer part.Close()

	id, err := s.saveDemo(r.Context(), part, store.Upload{Filename: filename, By: userID(u)})
	switch {
	case errors.Is(err, store.ErrQuota):
		uploadError(w, r, "quota")
	case errors.Is(err, store.ErrDuplicate):
		matchRedirect(w, r, id, "duplicate")
	case errors.Is(err, errNotDemo):
		uploadError(w, r, "notdemo")
	case errors.Is(err, errTooBig):
		uploadError(w, r, "toobig")
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, fmt.Sprintf("/matches/%d", id), http.StatusSeeOther)
	}
}

var (
	errNotDemo = errors.New("not a CS2 demo")
	errTooBig  = errors.New("demo too large")
)

// saveDemo stores an uploaded demo and queues it. If the user uploaded the
// same file before, it returns that match's ID and store.ErrDuplicate.
func (s *Server) saveDemo(ctx context.Context, body io.Reader, up store.Upload) (int64, error) {
	tmp, err := os.CreateTemp(s.DemoDir, "upload-*.dem")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed

	head := make([]byte, len(demoMagic))
	n, _ := io.ReadFull(body, head)
	if !bytes.Equal(head[:n], demoMagic) {
		tmp.Close()
		return 0, errNotDemo
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), io.MultiReader(bytes.NewReader(head), body))
	err = errors.Join(err, tmp.Close())
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return 0, errTooBig
		}
		return 0, err
	}

	up.SHA256 = h.Sum(nil)
	id, err := s.Store.CreateMatch(ctx, up)
	if err != nil {
		return id, err
	}
	// The worker retries until the file is in place.
	if err := os.Rename(tmpName, s.demoPath(id)); err != nil {
		// A failed match gives up its hash, so the next try is not taken
		// for a duplicate of a match that has no demo.
		return 0, errors.Join(err, s.Store.FailMatch(context.WithoutCancel(ctx), id, "the demo could not be stored"))
	}
	s.Log.Info("demo uploaded", "match", id, "file", up.Filename, "user", up.By)
	return id, nil
}

// Errors are passed in redirects as fixed codes, so a crafted link cannot put
// arbitrary text on the page.
var (
	uploadErrors = map[string]string{
		"nofile":  "Upload a .dem file.",
		"notdemo": "That is not a CS2 demo (.dem) file.",
		"toobig":  "The demo is larger than 1 GB.",
		"quota":   "You have used this week's uploads. They reset seven days after each upload.",
	}
	matchNotices = map[string]string{
		"pick":      "Pick one to five players from this match.",
		"duplicate": "You uploaded this demo before, so here is that match.",
		"busy":      "This match is still being processed. Try again when it is done.",
		"quota":     "You have used this week's coaching requests.",
		"trained":   "Practice completed and added to your log.",
	}
)

func uploadError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/?error="+url.QueryEscape(code), http.StatusSeeOther)
}

func matchRedirect(w http.ResponseWriter, r *http.Request, id int64, notice string) {
	target := fmt.Sprintf("/matches/%d", id)
	if notice != "" {
		target += "?notice=" + url.QueryEscape(notice)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// visibleMatch loads a match the user may see; others get a 404 so match IDs
// reveal nothing.
func (s *Server) visibleMatch(w http.ResponseWriter, r *http.Request, u *store.User) (*store.Match, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	if u != nil {
		ok, err := s.Store.CanView(r.Context(), id, u)
		if err != nil {
			s.fail(w, r, err)
			return nil, false
		}
		if !ok {
			http.NotFound(w, r)
			return nil, false
		}
	}
	m, err := s.Store.GetMatch(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return m, true
}

func (s *Server) match(w http.ResponseWriter, r *http.Request, u *store.User) {
	m, ok := s.visibleMatch(w, r, u)
	if !ok {
		return
	}
	page := matchPage{Viewer: s.viewer(u), Match: m, Notice: matchNotices[r.URL.Query().Get("notice")], CanCoach: canCoach(m, u)}
	page.Diagnostics = u != nil && u.Admin && r.URL.Query().Get("diagnostics") == "1"
	page.CanCoach = page.CanCoach && (m.Status == store.StatusDetected || m.Status == store.StatusCoached)
	if m.Status == store.StatusDetected || m.Status == store.StatusCoaching || m.Status == store.StatusCoached {
		an, err := s.Store.GetAnalysis(r.Context(), m.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if u != nil && len(m.Party) == 0 {
			friends, err := s.Store.Friends(r.Context(), u.SteamID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			page.SuggestedParty = store.PartyFor(u.SteamID, m.Players, friends)
		}
		page.build(an)
		if u != nil {
			active, err := s.Store.ActivePractice(r.Context(), u.SteamID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for _, session := range active {
				if d, ok := drills.ByID(session.Drill); ok {
					page.ActivePractice = append(page.ActivePractice, activePracticeView{PracticeSession: session, Title: d.Title})
				}
			}
		}
	}
	s.render(w, r, "match", page)
}

func (s *Server) matchStatus(w http.ResponseWriter, r *http.Request, u *store.User) {
	m, ok := s.visibleMatch(w, r, u)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{m.Status})
}

// canCoach reports whether a user may (re)coach a match: its uploader or an
// admin. Others who played in it can read the report but not replace it.
func canCoach(m *store.Match, u *store.User) bool {
	return u == nil || u.Admin || (m.UploadedBy != 0 && m.UploadedBy == u.SteamID)
}

func (s *Server) requestCoaching(w http.ResponseWriter, r *http.Request, u *store.User) {
	m, ok := s.visibleMatch(w, r, u)
	if !ok {
		return
	}
	if !canCoach(m, u) {
		http.Error(w, "only the uploader can coach this match", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	var party []match.PlayerID
	values := r.PostForm["player"]
	if r.PostForm.Get("use_friends") == "1" && u != nil {
		friends, err := s.Store.Friends(r.Context(), u.SteamID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		party = store.PartyFor(u.SteamID, m.Players, friends)
		values = nil
	}
	for _, v := range values {
		p, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			http.Error(w, "bad player", http.StatusBadRequest)
			return
		}
		party = append(party, match.PlayerID(p))
	}
	slices.Sort(party)
	party = slices.Compact(party)
	inMatch := func(p match.PlayerID) bool {
		return slices.ContainsFunc(m.Players, func(pl store.Player) bool { return pl.ID == p })
	}
	if len(party) == 0 || len(party) > 5 || !all(party, inMatch) {
		matchRedirect(w, r, m.ID, "pick")
		return
	}
	q, err := s.quota(r.Context(), u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if q != nil && q.CoachingLeft() <= 0 {
		matchRedirect(w, r, m.ID, "quota")
		return
	}
	err = s.Store.RequestCoaching(r.Context(), m.ID, party, userID(u), requestLanguage(r))
	if errors.Is(err, store.ErrQuota) {
		matchRedirect(w, r, m.ID, "quota")
		return
	}
	if errors.Is(err, store.ErrNotReady) {
		matchRedirect(w, r, m.ID, "busy")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	matchRedirect(w, r, m.ID, "")
}

func all[T any](xs []T, ok func(T) bool) bool {
	return !slices.ContainsFunc(xs, func(x T) bool { return !ok(x) })
}

func (s *Server) demoPath(id int64) string {
	return filepath.Join(s.DemoDir, fmt.Sprintf("%d.dem", id))
}

// worker guards internal endpoints with the shared worker token.
func (s *Server) worker(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || s.WorkerToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.WorkerToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (s *Server) serveDemo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, s.demoPath(id))
}
