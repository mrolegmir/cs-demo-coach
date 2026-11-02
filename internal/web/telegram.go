package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
	"github.com/mrolegmir/cs-demo-coach/internal/telegram"
)

// TelegramStore links a user's Telegram chat for brief delivery (ADR 13).
type TelegramStore interface {
	CreateTelegramCode(ctx context.Context, id match.PlayerID, codeHash []byte, expires time.Time) error
	GetTelegramLink(ctx context.Context, id match.PlayerID) (*store.TelegramLink, error)
	DeleteTelegramLink(ctx context.Context, id match.PlayerID) error
}

func (s *Server) routeTelegram(mux *http.ServeMux) {
	if s.Auth == nil || s.TelegramBot == nil {
		return
	}
	mux.HandleFunc("POST /telegram", s.page(s.connectTelegram))
	mux.HandleFunc("POST /telegram/delete", s.page(s.disconnectTelegram))
}

// connectTelegram makes a one-time code and opens the bot with it; the bot
// gets the code back with the chat when the user presses Start.
func (s *Server) connectTelegram(w http.ResponseWriter, r *http.Request, u *store.User) {
	bot := s.TelegramBot()
	if bot == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	code, hash := telegram.NewCode()
	if err := s.Store.CreateTelegramCode(r.Context(), u.SteamID, hash, time.Now().Add(telegram.CodeTTL)); err != nil {
		s.fail(w, r, err)
		return
	}
	link := "https://t.me/" + url.PathEscape(bot) + "?start=" + url.QueryEscape(code)
	http.Redirect(w, r, link, http.StatusSeeOther)
}

func (s *Server) disconnectTelegram(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.Store.DeleteTelegramLink(r.Context(), u.SteamID); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/#telegram", http.StatusSeeOther)
}

// telegramView is the Telegram part of the home page.
type telegramView struct {
	Linked bool
	Since  time.Time
}

// telegramState returns nil when Telegram delivery is off.
func (s *Server) telegramState(ctx context.Context, u *store.User) (*telegramView, error) {
	if u == nil || s.TelegramBot == nil || s.TelegramBot() == "" {
		return nil, nil
	}
	l, err := s.Store.GetTelegramLink(ctx, u.SteamID)
	if errors.Is(err, store.ErrNotFound) {
		return &telegramView{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &telegramView{Linked: true, Since: l.Created}, nil
}
