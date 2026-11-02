package web

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

type FriendStore interface {
	Friends(context.Context, match.PlayerID) ([]match.PlayerID, error)
	FriendSettings(context.Context, match.PlayerID) ([]store.FriendContact, bool, error)
	SaveFriendSettings(context.Context, match.PlayerID, []match.PlayerID, bool) error
}

type friendsPage struct {
	Viewer
	Contacts         []store.FriendContact
	Automatic, Saved bool
	Error            string
}

func (s *Server) friends(w http.ResponseWriter, r *http.Request, u *store.User) {
	contacts, automatic, err := s.Store.FriendSettings(r.Context(), u.SteamID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "friends", friendsPage{Viewer: s.viewer(u), Contacts: contacts, Automatic: automatic, Saved: r.URL.Query().Get("saved") == "1", Error: r.URL.Query().Get("error")})
}

func (s *Server) saveFriends(w http.ResponseWriter, r *http.Request, u *store.User) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	values := slices.Clone(r.PostForm["friend"])
	if extra := strings.TrimSpace(r.PostForm.Get("steam_id")); extra != "" {
		extra = strings.TrimSuffix(extra, "/")
		extra = strings.TrimPrefix(extra, "https://steamcommunity.com/profiles/")
		values = append(values, extra)
	}
	var friends []match.PlayerID
	for _, v := range values {
		id, err := strconv.ParseUint(v, 10, 64)
		// Individual public SteamID64, account ID is the low 32 bits.
		if err != nil || id <= 76561197960265728 || id > 76561197960265728+1<<32-1 || match.PlayerID(id) == u.SteamID {
			http.Redirect(w, r, "/friends?error=id", http.StatusSeeOther)
			return
		}
		friends = append(friends, match.PlayerID(id))
	}
	slices.Sort(friends)
	friends = slices.Compact(friends)
	if len(friends) > 100 {
		http.Redirect(w, r, "/friends?error=limit", http.StatusSeeOther)
		return
	}
	if err := s.Store.SaveFriendSettings(r.Context(), u.SteamID, friends, r.PostForm.Get("automatic") == "on"); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/friends?saved=1", http.StatusSeeOther)
}
