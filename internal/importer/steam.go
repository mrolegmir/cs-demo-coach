package importer

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// DefaultAPI is the Steam Web API.
const DefaultAPI = "https://api.steampowered.com"

var (
	// ErrForbidden is Steam's 403: the auth code is wrong or revoked, or the
	// API key is. KeyValid tells them apart.
	ErrForbidden = errors.New("steam: access denied")
	// ErrUnknownCode is Steam's 412: the known code is not one of the
	// player's matches.
	ErrUnknownCode = errors.New("steam: share code not in the player's history")
)

// Steam calls the Steam Web API.
type Steam struct {
	Key  string
	Base string       // default DefaultAPI
	HTTP *http.Client // default http.DefaultClient
}

// NextCode returns the share code of the match after known in the player's
// history, or "" when known is the newest.
func (s *Steam) NextCode(ctx context.Context, player match.PlayerID, authCode, known string) (string, error) {
	q := url.Values{
		"steamid":    {strconv.FormatUint(uint64(player), 10)},
		"steamidkey": {authCode},
		"knowncode":  {known},
	}
	var body struct {
		Result struct {
			NextCode string `json:"nextcode"`
		} `json:"result"`
	}
	status, err := s.get(ctx, "/ICSGOPlayers_730/GetNextMatchSharingCode/v1", q, &body)
	switch {
	case err != nil:
		return "", err
	case status == http.StatusAccepted || body.Result.NextCode == "n/a":
		return "", nil
	case body.Result.NextCode == "":
		return "", errors.New("steam: GetNextMatchSharingCode: empty response")
	}
	return body.Result.NextCode, nil
}

// KeyValid reports whether Steam accepts the API key, with a call that
// needs nothing but the key.
func (s *Steam) KeyValid(ctx context.Context, player match.PlayerID) (bool, error) {
	q := url.Values{"steamids": {strconv.FormatUint(uint64(player), 10)}}
	_, err := s.get(ctx, "/ISteamUser/GetPlayerSummaries/v2", q, &struct{}{})
	if errors.Is(err, ErrForbidden) {
		return false, nil
	}
	return err == nil, err
}

// get calls a Web API method and decodes a 200 or 202 response into out.
// Errors never include the URL: it carries the key and the auth code.
func (s *Steam) get(ctx context.Context, method string, q url.Values, out any) (int, error) {
	q.Set("key", s.Key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cmp.Or(s.Base, DefaultAPI)+method+"?"+q.Encode(), nil)
	if err != nil {
		return 0, fmt.Errorf("steam: %s: bad request", method)
	}
	resp, err := cmp.Or(s.HTTP, http.DefaultClient).Do(req)
	if err != nil {
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}
		return 0, fmt.Errorf("steam: %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusAccepted:
		// "Nothing newer"; the body is not needed and may be empty.
		return resp.StatusCode, nil
	case http.StatusForbidden, http.StatusUnauthorized:
		return resp.StatusCode, ErrForbidden
	case http.StatusPreconditionFailed:
		return resp.StatusCode, ErrUnknownCode
	default:
		return resp.StatusCode, fmt.Errorf("steam: %s: %s", method, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("steam: %s: decode: %w", method, err)
	}
	return resp.StatusCode, nil
}
