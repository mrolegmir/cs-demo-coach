package web

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/sharecode"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// SteamLinkStore keeps what the share-code importer reads match history
// with.
type SteamLinkStore interface {
	SetSteamLink(ctx context.Context, id match.PlayerID, authCode, lastCode string) error
	GetSteamLink(ctx context.Context, id match.PlayerID) (*store.SteamLink, error)
	DeleteSteamLink(ctx context.Context, id match.PlayerID) error
	ListShareCodes(ctx context.Context, id match.PlayerID, limit int) ([]store.ShareCode, error)
}

// authCodeRe matches a match history authentication code, e.g. ABCD-EFGHJ-KLMN.
var authCodeRe = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{5}-[A-Z0-9]{4}$`)

var linkErrors = map[string]string{
	"authcode":  "That does not look like an authentication code: it has the form ABCD-EFGHJ-KLMN.",
	"sharecode": "That does not look like a share code: it has the form CSGO-xxxxx-xxxxx-xxxxx-xxxxx-xxxxx.",
}

// saveSteamLink stores the auth code and the newest share code the importer
// starts from. Both are checked for form only; Steam checks the rest, and
// the page shows its answer after the next round.
func (s *Server) saveSteamLink(w http.ResponseWriter, r *http.Request, u *store.User) {
	auth := strings.ToUpper(strings.TrimSpace(r.FormValue("auth_code")))
	if !authCodeRe.MatchString(auth) {
		http.Redirect(w, r, "/tokens?link=authcode", http.StatusSeeOther)
		return
	}
	last, err := sharecode.Normalize(r.FormValue("share_code"))
	if err != nil {
		http.Redirect(w, r, "/tokens?link=sharecode", http.StatusSeeOther)
		return
	}
	if err := s.Store.SetSteamLink(r.Context(), u.SteamID, auth, last); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

func (s *Server) deleteSteamLink(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.Store.DeleteSteamLink(r.Context(), u.SteamID); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

// codeStatus names a share code's status for the page.
func codeStatus(status string) string {
	switch status {
	case store.CodeWaiting, store.CodeResolving:
		return "finding the demo"
	case store.CodeDownload, store.CodeDownloading:
		return "downloading"
	case store.CodeImported:
		return "imported"
	case store.CodeExpired:
		return "demo expired"
	case store.CodeOverQuota:
		return "over the weekly quota"
	}
	return "failed"
}

// maskCode shows only the last group of an auth code, so the page never
// echoes the secret.
func maskCode(code string) string {
	if i := strings.LastIndexByte(code, '-'); i >= 0 {
		return "••••-•••••" + code[i:]
	}
	return "••••"
}
