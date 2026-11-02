package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

type activePracticeView struct {
	store.PracticeSession
	Title string
}

func (s *Server) startMatchPractice(w http.ResponseWriter, r *http.Request, u *store.User) {
	m, ok := s.visibleMatch(w, r, u)
	if !ok {
		return
	}
	d, ok := drills.ByID(r.FormValue("drill"))
	minutes, err := strconv.Atoi(r.FormValue("minutes"))
	if !ok || err != nil || minutes < 0 || minutes > 600 {
		http.Error(w, "invalid practice", http.StatusBadRequest)
		return
	}
	if err := s.Store.StartPractice(r.Context(), u.SteamID, d.ID, minutes); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/matches/"+strconv.FormatInt(m.ID, 10)+"#training", http.StatusSeeOther)
}

func (s *Server) finishMatchPractice(w http.ResponseWriter, r *http.Request, u *store.User) {
	back := "/players/" + strconv.FormatUint(uint64(u.SteamID), 10) + "#practice"
	if r.PathValue("id") != "" {
		m, ok := s.visibleMatch(w, r, u)
		if !ok {
			return
		}
		back = "/matches/" + strconv.FormatInt(m.ID, 10) + "?notice=trained#training"
	}
	id, err := strconv.ParseInt(r.PathValue("session"), 10, 64)
	minutes, minErr := strconv.Atoi(r.FormValue("minutes"))
	if err != nil || minErr != nil || minutes < 0 || minutes > 600 {
		http.Error(w, "invalid practice", http.StatusBadRequest)
		return
	}
	if err := s.Store.FinishPractice(r.Context(), u.SteamID, id, minutes); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
