package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/auth"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// UserStore is the account part of the store.
type UserStore interface {
	GetUser(ctx context.Context, id match.PlayerID) (*store.User, error)
	RedeemInvite(ctx context.Context, code string, id match.PlayerID) error
	CreateInvite(ctx context.Context, by match.PlayerID, code string, ttl time.Duration) error
	ListInvites(ctx context.Context, by match.PlayerID) ([]store.Invite, error)
	CreateSession(ctx context.Context, tokenHash []byte, id match.PlayerID, ttl time.Duration) error
	SessionUser(ctx context.Context, tokenHash []byte) (*store.User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
}

// Auth configures Steam sign-in.
type Auth struct {
	Steam auth.Steam
	// PublicURL is the site's external base URL, e.g. https://csdc.example.com.
	// Steam redirects back to it, and POSTs must come from it.
	PublicURL string
	// Weekly limits per user; zero means unlimited.
	UploadsPerWeek  int
	CoachingPerWeek int
}

const (
	sessionCookie = "csdc_session"
	inviteCookie  = "csdc_invite"
	// loginCookie holds the state of a login in progress. It must match the
	// state in Steam's callback, so a callback URL only works in the browser
	// that started the login, and only once.
	loginCookie = "csdc_login"
	sessionTTL  = 30 * 24 * time.Hour
	inviteTTL   = 7 * 24 * time.Hour
	quotaWindow = 7 * 24 * time.Hour
)

func (s *Server) routeAuth(mux *http.ServeMux) {
	if s.Auth == nil {
		return
	}
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("GET /auth/steam", s.startLogin)
	mux.HandleFunc("GET /auth/steam/callback", s.finishLogin)
	mux.HandleFunc("POST /logout", s.page(s.logout))
	mux.HandleFunc("GET /friends", s.page(s.friends))
	mux.HandleFunc("POST /friends", s.page(s.saveFriends))
	mux.HandleFunc("GET /invite/{code}", s.acceptInvite)
	mux.HandleFunc("GET /invites", s.page(s.invites))
	mux.HandleFunc("POST /invites", s.page(s.createInvite))
	mux.HandleFunc("GET /practice", s.page(s.practiceForm))
	mux.HandleFunc("POST /practice", s.page(s.addPractice))
	mux.HandleFunc("POST /matches/{id}/practice/start", s.page(s.startMatchPractice))
	mux.HandleFunc("POST /matches/{id}/practice/{session}/finish", s.page(s.finishMatchPractice))
	mux.HandleFunc("POST /practice/{session}/finish", s.page(s.finishMatchPractice))
	mux.HandleFunc("POST /practice/{id}/delete", s.page(s.deletePractice))
}

// page wraps a handler that needs the signed-in user. With auth on it sends
// anonymous visitors to the login page and rejects cross-site POSTs; with
// auth off the user is nil.
func (s *Server) page(h func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Auth == nil {
			h(w, r, nil)
			return
		}
		if r.Method == http.MethodPost && !s.sameOrigin(r) {
			http.Error(w, "cross-site request", http.StatusForbidden)
			return
		}
		u, err := s.currentUser(r)
		if errors.Is(err, store.ErrNotFound) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		h(w, r, u)
	}
}

// sameOrigin checks the Origin (or Referer) of a state-changing request. The
// session cookie is SameSite=Lax as well; this is the second line of defense.
func (s *Server) sameOrigin(r *http.Request) bool {
	want, err := url.Parse(s.Auth.PublicURL)
	if err != nil {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	got, err := url.Parse(origin)
	return err == nil && got.Scheme == want.Scheme && got.Host == want.Host
}

func (s *Server) currentUser(r *http.Request) (*store.User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, store.ErrNotFound
	}
	return s.Store.SessionUser(r.Context(), hashToken(c.Value))
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) cookie(name, value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.Auth.PublicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) callbackURL(state string) string {
	return strings.TrimSuffix(s.Auth.PublicURL, "/") + "/auth/steam/callback?state=" + url.QueryEscape(state)
}

type loginData struct {
	Viewer
	Message string
}

var loginMessages = map[string]string{
	"invite": "This is an invite-only beta. Ask a friend who uses CS Demo Coach for an invite link, then open it and sign in.",
	"failed": "Steam could not confirm the sign-in. Please try again.",
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login", loginData{Viewer: s.viewer(nil), Message: loginMessages[r.URL.Query().Get("m")]})
}

func (s *Server) startLogin(w http.ResponseWriter, r *http.Request) {
	state := newToken()
	http.SetCookie(w, s.cookie(loginCookie, state, 10*time.Minute))
	http.Redirect(w, r, s.Auth.Steam.LoginURL(s.Auth.PublicURL, s.callbackURL(state)), http.StatusSeeOther)
}

func (s *Server) finishLogin(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.cookie(loginCookie, "", -time.Second))
	state := r.URL.Query().Get("state")
	c, err := r.Cookie(loginCookie)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		s.Log.Warn("steam login rejected", "err", "login state does not match this browser")
		http.Redirect(w, r, "/login?m=failed", http.StatusSeeOther)
		return
	}
	id, err := s.Auth.Steam.Verify(r.Context(), s.callbackURL(state), r.URL.Query(), time.Now())
	if err != nil {
		s.Log.Warn("steam login rejected", "err", err)
		http.Redirect(w, r, "/login?m=failed", http.StatusSeeOther)
		return
	}
	if _, err := s.Store.GetUser(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		c, cerr := r.Cookie(inviteCookie)
		if cerr != nil {
			http.Redirect(w, r, "/login?m=invite", http.StatusSeeOther)
			return
		}
		if err := s.Store.RedeemInvite(r.Context(), c.Value, id); errors.Is(err, store.ErrInvalidInvite) {
			http.Redirect(w, r, "/login?m=invite", http.StatusSeeOther)
			return
		} else if err != nil {
			s.fail(w, r, err)
			return
		}
		s.Log.Info("user joined", "user", id)
	} else if err != nil {
		s.fail(w, r, err)
		return
	}

	token := newToken()
	if err := s.Store.CreateSession(r.Context(), hashToken(token), id, sessionTTL); err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, s.cookie(sessionCookie, token, sessionTTL))
	http.SetCookie(w, s.cookie(inviteCookie, "", -time.Second))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.Store.DeleteSession(r.Context(), hashToken(c.Value)); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	http.SetCookie(w, s.cookie(sessionCookie, "", -time.Second))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// acceptInvite remembers the invite for the Steam round trip.
func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.cookie(inviteCookie, r.PathValue("code"), 30*time.Minute))
	http.Redirect(w, r, "/auth/steam", http.StatusSeeOther)
}

type invitesData struct {
	Viewer
	Invites []inviteView
}

type inviteView struct {
	Link    string
	Used    bool
	Expired bool
	Expires time.Time
}

func (s *Server) invites(w http.ResponseWriter, r *http.Request, u *store.User) {
	list, err := s.Store.ListInvites(r.Context(), u.SteamID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := invitesData{Viewer: s.viewer(u)}
	for _, inv := range list {
		data.Invites = append(data.Invites, inviteView{
			Link:    strings.TrimSuffix(s.Auth.PublicURL, "/") + "/invite/" + inv.Code,
			Used:    inv.UsedBy != 0,
			Expired: time.Now().After(inv.ExpiresAt),
			Expires: inv.ExpiresAt,
		})
	}
	s.render(w, r, "invites", data)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.Store.CreateInvite(r.Context(), u.SteamID, newToken()[:22], inviteTTL); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/invites", http.StatusSeeOther)
}

// quotaView is a user's remaining weekly quota; nil means unlimited.
type quotaView struct {
	Uploads, UploadsLimit   int
	Coaching, CoachingLimit int
}

func (q *quotaView) UploadsLeft() int  { return left(q.Uploads, q.UploadsLimit) }
func (q *quotaView) CoachingLeft() int { return left(q.Coaching, q.CoachingLimit) }

func left(used, limit int) int {
	if limit == 0 {
		return 1 << 30
	}
	return max(0, limit-used)
}

func (s *Server) quota(ctx context.Context, u *store.User) (*quotaView, error) {
	if s.Auth == nil || u == nil || u.Admin {
		return nil, nil
	}
	usage, err := s.Store.GetUsage(ctx, u.SteamID, time.Now().Add(-quotaWindow))
	if err != nil {
		return nil, err
	}
	return &quotaView{
		Uploads: usage.Uploads, UploadsLimit: s.Auth.UploadsPerWeek,
		Coaching: usage.Coaching, CoachingLimit: s.Auth.CoachingPerWeek,
	}, nil
}
