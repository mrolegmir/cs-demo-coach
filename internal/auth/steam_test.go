package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const returnTo = "https://csdc.example.com/auth/steam/callback"

// fakeSteam answers check_authentication with valid unless the signature is
// "bad", and records what it was asked.
func fakeSteam(t *testing.T) (*httptest.Server, *url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		valid := got.Get("openid.sig") != "bad"
		_, _ = io.WriteString(w, "ns:http://specs.openid.net/auth/2.0\nis_valid:"+map[bool]string{true: "true", false: "false"}[valid]+"\n")
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func callback(endpoint, id, sig string) url.Values {
	claimed := "https://steamcommunity.com/openid/id/" + id
	return url.Values{
		"openid.response_nonce": {now.Add(-time.Minute).Format(time.RFC3339) + "nonce"},
		"openid.ns":             {openIDNS},
		"openid.mode":           {"id_res"},
		"openid.op_endpoint":    {endpoint},
		"openid.claimed_id":     {claimed},
		"openid.identity":       {claimed},
		"openid.return_to":      {returnTo},
		"openid.sig":            {sig},
		"openid.signed":         {"signed,op_endpoint,claimed_id,identity,return_to"},
	}
}

func TestLoginURL(t *testing.T) {
	u, err := url.Parse(Steam{}.LoginURL("https://csdc.example.com", returnTo))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "steamcommunity.com" || q.Get("openid.mode") != "checkid_setup" ||
		q.Get("openid.return_to") != returnTo || q.Get("openid.realm") != "https://csdc.example.com" {
		t.Fatalf("login URL = %s", u)
	}
}

func TestVerify(t *testing.T) {
	srv, asked := fakeSteam(t)
	s := Steam{Endpoint: srv.URL}

	id, err := s.Verify(t.Context(), returnTo, callback(srv.URL, "76561198000000001", "good"), now)
	if err != nil || id != 76561198000000001 {
		t.Fatalf("Verify = %d, %v", id, err)
	}
	if asked.Get("openid.mode") != "check_authentication" || asked.Get("openid.sig") != "good" {
		t.Errorf("check_authentication request = %v", *asked)
	}

	tests := []struct {
		name   string
		mutate func(url.Values)
	}{
		{"signature rejected by steam", func(q url.Values) { q.Set("openid.sig", "bad") }},
		{"cancelled", func(q url.Values) { q.Set("openid.mode", "cancel") }},
		{"other return_to", func(q url.Values) { q.Set("openid.return_to", "https://evil.example/cb") }},
		{"other provider", func(q url.Values) { q.Set("openid.op_endpoint", "https://evil.example/openid") }},
		{"not a steam id", func(q url.Values) {
			q.Set("openid.claimed_id", "https://evil.example/id/1")
			q.Set("openid.identity", "https://evil.example/id/1")
		}},
		{"identity differs from claimed_id", func(q url.Values) {
			q.Set("openid.identity", "https://steamcommunity.com/openid/id/76561198000000002")
		}},
		{"duplicated claimed_id to smuggle another account", func(q url.Values) {
			q.Add("openid.claimed_id", "https://steamcommunity.com/openid/id/76561198000000009")
			q.Add("openid.identity", "https://steamcommunity.com/openid/id/76561198000000009")
		}},
		{"replayed old assertion", func(q url.Values) {
			q.Set("openid.response_nonce", now.Add(-time.Hour).Format(time.RFC3339)+"nonce")
		}},
		{"nonce from the future", func(q url.Values) {
			q.Set("openid.response_nonce", now.Add(time.Hour).Format(time.RFC3339)+"nonce")
		}},
		{"no nonce", func(q url.Values) { q.Del("openid.response_nonce") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := callback(srv.URL, "76561198000000001", "good")
			tt.mutate(q)
			if _, err := s.Verify(t.Context(), returnTo, q, now); err == nil {
				t.Fatal("login accepted")
			}
		})
	}
}

func TestVerifyUsesSteamByDefault(t *testing.T) {
	q := callback(SteamEndpoint, "76561198000000001", "good")
	q.Set("openid.op_endpoint", "https://other.example")
	_, err := Steam{}.Verify(t.Context(), returnTo, q, now)
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("err = %v", err)
	}
}
