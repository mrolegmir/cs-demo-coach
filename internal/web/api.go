package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// TokenStore keeps the upload tokens users' own tools sign in with.
type TokenStore interface {
	CreateUploadToken(ctx context.Context, id match.PlayerID, tokenHash []byte, name string) error
	UploadTokenUser(ctx context.Context, tokenHash []byte) (*store.User, error)
	ListUploadTokens(ctx context.Context, id match.PlayerID) ([]store.UploadToken, error)
	DeleteUploadToken(ctx context.Context, owner match.PlayerID, tokenID int64) error
}

// UploadResponse is the body of a successful POST /api/demos.
type UploadResponse struct {
	ID  int64  `json:"id"`
	URL string `json:"url"`
	// Duplicate means the user uploaded this file before; ID is that match.
	Duplicate bool `json:"duplicate,omitzero"`
}

func (s *Server) routeAPI(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/demos", s.apiUpload)
	if s.Auth != nil {
		mux.HandleFunc("GET /tokens", s.page(s.tokens))
		mux.HandleFunc("POST /tokens", s.page(s.createToken))
		mux.HandleFunc("POST /tokens/{id}/delete", s.page(s.deleteToken))
		if s.ShareCodes {
			mux.HandleFunc("POST /steam-link", s.page(s.saveSteamLink))
			mux.HandleFunc("POST /steam-link/delete", s.page(s.deleteSteamLink))
		}
	}
}

// cs2Release bounds match times from below.
var cs2Release = time.Date(2023, 9, 27, 0, 0, 0, 0, time.UTC)

// plausiblePlayed returns t if it can be when a CS2 match was played, and
// the zero time otherwise.
func plausiblePlayed(t time.Time) time.Time {
	if t.After(cs2Release) && t.Before(time.Now().Add(time.Hour)) {
		return t
	}
	return time.Time{}
}

// apiUpload takes a demo as the raw request body, named by the name query
// parameter; played, if given, is when the match was played (RFC 3339).
// With auth on it needs an upload token as a bearer token; it is
// what csdc watch calls.
func (s *Server) apiUpload(w http.ResponseWriter, r *http.Request) {
	var u *store.User
	if s.Auth != nil {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			http.Error(w, "upload token required", http.StatusUnauthorized)
			return
		}
		var err error
		u, err = s.Store.UploadTokenUser(r.Context(), hashToken(token))
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "unknown upload token", http.StatusUnauthorized)
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	q, err := s.quota(r.Context(), u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if q != nil && q.UploadsLeft() <= 0 {
		http.Error(w, uploadErrors["quota"], http.StatusTooManyRequests)
		return
	}

	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "." || name == string(filepath.Separator) {
		name = "upload.dem"
	}
	up := store.Upload{Filename: name, By: userID(u)}
	if played, err := time.Parse(time.RFC3339, r.URL.Query().Get("played")); err == nil {
		up.Played = plausiblePlayed(played)
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxDemoSize)
	id, err := s.saveDemo(r.Context(), r.Body, up)
	dup := errors.Is(err, store.ErrDuplicate)
	switch {
	case errors.Is(err, store.ErrQuota):
		http.Error(w, uploadErrors["quota"], http.StatusTooManyRequests)
		return
	case dup:
	case errors.Is(err, errNotDemo):
		http.Error(w, uploadErrors["notdemo"], http.StatusUnsupportedMediaType)
		return
	case errors.Is(err, errTooBig):
		http.Error(w, uploadErrors["toobig"], http.StatusRequestEntityTooLarge)
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	if !dup {
		w.WriteHeader(http.StatusCreated)
	}
	writeJSON(w, UploadResponse{ID: id, URL: fmt.Sprintf("/matches/%d", id), Duplicate: dup})
}

type tokensData struct {
	Viewer
	Tokens []store.UploadToken
	// New is the token just created, shown once.
	New    string
	APIURL string

	ShareCodes bool
	Link       *store.SteamLink
	Codes      []store.ShareCode
	LinkError  string
}

func (s *Server) tokens(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.renderTokens(w, r, u, "")
}

func (s *Server) renderTokens(w http.ResponseWriter, r *http.Request, u *store.User, created string) {
	list, err := s.Store.ListUploadTokens(r.Context(), u.SteamID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := tokensData{
		Viewer: s.viewer(u), Tokens: list, New: created,
		APIURL:     strings.TrimSuffix(s.Auth.PublicURL, "/"),
		ShareCodes: s.ShareCodes, LinkError: linkErrors[r.URL.Query().Get("link")],
	}
	if s.ShareCodes {
		data.Link, err = s.Store.GetSteamLink(r.Context(), u.SteamID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.fail(w, r, err)
			return
		}
		if data.Codes, err = s.Store.ListShareCodes(r.Context(), u.SteamID, 10); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.render(w, r, "tokens", data)
}

// createToken shows the new token on the response page rather than
// redirecting, so it never appears in a URL or in history.
func (s *Server) createToken(w http.ResponseWriter, r *http.Request, u *store.User) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "csdc watch"
	}
	if rs := []rune(name); len(rs) > 60 {
		name = string(rs[:60])
	}
	token := newToken()
	if err := s.Store.CreateUploadToken(r.Context(), u.SteamID, hashToken(token), name); err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.renderTokens(w, r, u, token)
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	err = s.Store.DeleteUploadToken(r.Context(), u.SteamID, id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

// lastUsed formats an upload token's last use for the tokens page.
func lastUsed(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("Jan 2 15:04")
}
