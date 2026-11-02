// Package auth signs users in with Steam (OpenID 2.0). A Steam login gives a
// SteamID64, the same ID players have in demos, so accounts link to their
// matches without any setup. No Steam Web API key is needed.
package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// SteamEndpoint is Steam's OpenID provider.
const SteamEndpoint = "https://steamcommunity.com/openid/login"

const (
	openIDNS         = "http://specs.openid.net/auth/2.0"
	identifierSelect = "http://specs.openid.net/auth/2.0/identifier_select"
)

var claimedID = regexp.MustCompile(`^https://steamcommunity\.com/openid/id/(\d{17})$`)

// ErrNotVerified is returned when Steam does not confirm a login.
var ErrNotVerified = errors.New("auth: steam login not verified")

// Steam talks to Steam's OpenID provider.
type Steam struct {
	// Endpoint defaults to SteamEndpoint; tests point it at a fake.
	Endpoint string
	Client   *http.Client
}

func (s Steam) endpoint() string {
	if s.Endpoint == "" {
		return SteamEndpoint
	}
	return s.Endpoint
}

// LoginURL is where to send the browser to sign in. Steam redirects back to
// returnTo, which must lie within realm.
func (s Steam) LoginURL(realm, returnTo string) string {
	q := url.Values{
		"openid.ns":         {openIDNS},
		"openid.mode":       {"checkid_setup"},
		"openid.return_to":  {returnTo},
		"openid.realm":      {realm},
		"openid.identity":   {identifierSelect},
		"openid.claimed_id": {identifierSelect},
	}
	return s.endpoint() + "?" + q.Encode()
}

// MaxNonceAge is how old a positive assertion may be. Together with the
// per-login state in return_to it keeps captured callback URLs from being
// replayed.
const MaxNonceAge = 5 * time.Minute

// Verify checks the parameters Steam redirected back with and returns the
// signed-in SteamID. returnTo must be the exact URL the login was started
// with. Steam itself confirms the signature (check_authentication), so no
// keys or association state are needed here.
func (s Steam) Verify(ctx context.Context, returnTo string, q url.Values, now time.Time) (match.PlayerID, error) {
	// Every OpenID parameter must appear once: with duplicates, what we check
	// here and what Steam verifies could be different values.
	for k, v := range q {
		if strings.HasPrefix(k, "openid.") && len(v) != 1 {
			return 0, fmt.Errorf("%w: repeated parameter %s", ErrNotVerified, k)
		}
	}
	if q.Get("openid.mode") != "id_res" {
		return 0, ErrNotVerified
	}
	if q.Get("openid.return_to") != returnTo || q.Get("openid.op_endpoint") != s.endpoint() {
		return 0, fmt.Errorf("%w: unexpected return_to or endpoint", ErrNotVerified)
	}
	m := claimedID.FindStringSubmatch(q.Get("openid.claimed_id"))
	if m == nil || q.Get("openid.identity") != q.Get("openid.claimed_id") {
		return 0, fmt.Errorf("%w: unexpected claimed_id", ErrNotVerified)
	}
	id, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrNotVerified, err)
	}
	// The nonce starts with its UTC timestamp, e.g. 2026-10-29T12:00:00ZaBc.
	nonce := q.Get("openid.response_nonce")
	if len(nonce) < 20 {
		return 0, fmt.Errorf("%w: missing nonce", ErrNotVerified)
	}
	issued, err := time.Parse(time.RFC3339, nonce[:20])
	if err != nil || now.Sub(issued) > MaxNonceAge || issued.Sub(now) > time.Minute {
		return 0, fmt.Errorf("%w: stale or invalid nonce", ErrNotVerified)
	}

	check := url.Values{}
	for k := range q {
		if strings.HasPrefix(k, "openid.") {
			check.Set(k, q.Get(k))
		}
	}
	check.Set("openid.mode", "check_authentication")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint(), strings.NewReader(check.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("auth: steam: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return 0, fmt.Errorf("auth: steam: %w", err)
	}
	// The reply is key:value lines; is_valid:true confirms the signature.
	for line := range strings.SplitSeq(string(body), "\n") {
		if strings.TrimSpace(line) == "is_valid:true" {
			return match.PlayerID(id), nil
		}
	}
	return 0, ErrNotVerified
}
