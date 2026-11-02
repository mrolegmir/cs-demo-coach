package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestLocalizedAccountTemplates(t *testing.T) {
	s, _ := newServer(t, coachedStore(t))
	drill := drills.All()[0]
	for _, language := range []string{"ru", "en"} {
		for _, page := range []struct {
			name string
			data any
		}{
			{"login", loginData{Message: loginMessages["invite"]}},
			{"invites", invitesData{Invites: []inviteView{{Link: "/invite/example"}}}},
			{"tokens", tokensData{ShareCodes: true, New: "example", LinkError: linkErrors["authcode"]}},
			{"brief", briefPage{Brief: &store.Brief{Input: &coach.Input{}, Status: store.BriefQueued}}},
			{"history", historyPage{
				Own: true, Kinds: []kindOption{{Kind: detect.KindIsolatedDeath, Label: "Isolated death"}},
				Drills: []drillGroup{{Label: "Isolated death", Drills: []drills.Drill{drill}}},
			}},
			{"match", matchPage{Match: &store.Match{}, Players: []playerView{{Name: "player01", Focus: []focusView{{Lib: &drill}}}}}},
		} {
			t.Run(language+"/"+page.name, func(t *testing.T) {
				r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
				r.AddCookie(&http.Cookie{Name: languageCookie, Value: language})
				w := httptest.NewRecorder()
				s.render(w, r, page.name, page.data)
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "</html>") {
					t.Fatalf("render failed: %d %s", w.Code, w.Body.String())
				}
				if language == "ru" && (page.name == "history" || page.name == "match") &&
					!strings.Contains(w.Body.String(), russianMessages[drill.Title]) {
					t.Fatal("library drill title not localized")
				}
			})
		}
	}
}

func TestPageLanguages(t *testing.T) {
	s, _ := newServer(t, coachedStore(t))
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/matches/7", "/players/3"} {
		for _, language := range []string{"", "ru", "en", "unknown"} {
			t.Run(path+"/"+language, func(t *testing.T) {
				t.Parallel()
				r := httptest.NewRequest(http.MethodGet, path, http.NoBody)
				if language != "" {
					r.AddCookie(&http.Cookie{Name: languageCookie, Value: language})
				}
				r.Header.Set("Accept-Language", "en-US") // Russian is the explicit default.
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				wantLanguage, wantText := "ru", "тренер CS2 с разбором конкретных эпизодов"
				if language == "en" {
					wantLanguage, wantText = "en", "evidence-based CS2 coaching"
				}
				if w.Code != http.StatusOK || w.Header().Get("Content-Language") != wantLanguage ||
					!strings.Contains(w.Body.String(), `<html lang="`+wantLanguage+`">`) ||
					!strings.Contains(w.Body.String(), wantText) {
					t.Fatalf("unexpected localized response: %d %s", w.Code, w.Body.String())
				}
				if !strings.Contains(w.Body.String(), `action="/language"`) {
					t.Fatal("language switch missing")
				}
				if path == "/matches/7" && !strings.Contains(w.Body.String(), `class="status coached"`) {
					t.Fatal("localization changed the status CSS class")
				}
			})
		}
	}
}

func TestCoachingRequestUsesBrowserLanguage(t *testing.T) {
	for _, language := range []string{"", "ru", "en"} {
		st := coachedStore(t)
		_, h := newServer(t, st)
		r := httptest.NewRequest(http.MethodPost, "/matches/7/coach", strings.NewReader("player=1"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if language != "" {
			r.AddCookie(&http.Cookie{Name: languageCookie, Value: language})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusSeeOther || st.coachingLanguage != coach.Language(language) {
			t.Fatalf("language %q: %d, saved %q", language, w.Code, st.coachingLanguage)
		}
	}
}

func TestLanguageSwitch(t *testing.T) {
	s, _ := newServer(t, coachedStore(t))
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	for _, destination := range []string{"/matches/7?notice=busy", "https://example.com", "//example.com", "/\\example.com", ""} {
		form := url.Values{"lang": {"en"}, "return": {destination}}
		r := httptest.NewRequest(http.MethodPost, "https://coach.example/language", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := "/"
		if destination == "/matches/7?notice=busy" {
			want = destination
		}
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != want {
			t.Fatalf("redirect: %d %q", w.Code, w.Header().Get("Location"))
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != languageCookie || cookies[0].Value != "en" ||
			cookies[0].Path != "/" || !cookies[0].Secure || !cookies[0].HttpOnly ||
			cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].MaxAge != 365*24*60*60 {
			t.Fatalf("unexpected cookie: %v", cookies)
		}
		get := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		get.AddCookie(cookies[0])
		w = httptest.NewRecorder()
		h.ServeHTTP(w, get)
		if !strings.Contains(w.Body.String(), "Recent matches") {
			t.Fatal("selection did not persist")
		}
	}
}

func TestLanguageSwitchValidation(t *testing.T) {
	s, _ := newServer(t, coachedStore(t))
	for _, language := range []string{"de", "", "ru", "en"} {
		r := httptest.NewRequest(http.MethodPost, "/language", strings.NewReader("lang="+language))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.setLanguage(w, r)
		want := http.StatusBadRequest
		if language == "ru" || language == "en" {
			want = http.StatusSeeOther
		}
		if w.Code != want {
			t.Fatalf("%q: status %d", language, w.Code)
		}
	}
	s.Auth = &Auth{PublicURL: "https://coach.example"}
	r := httptest.NewRequest(http.MethodPost, "/language", strings.NewReader("lang=en"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://other.example")
	w := httptest.NewRecorder()
	s.setLanguage(w, r)
	if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
		t.Fatal("cross-site switch accepted")
	}
	// Anonymous visitors can switch on the login screen with the right origin.
	r = httptest.NewRequest(http.MethodPost, "/language", strings.NewReader("lang=ru"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", s.Auth.PublicURL)
	w = httptest.NewRecorder()
	s.setLanguage(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatal("anonymous same-origin switch rejected")
	}
}

func TestRussianCatalogCoverage(t *testing.T) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	literals := regexp.MustCompile(`\{\{tr ("(?:[^"\\]|\\.)*")`)
	for _, entry := range entries {
		content, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, literal := range literals.FindAllSubmatch(content, -1) {
			var key string
			if err := json.Unmarshal(literal[1], &key); err != nil {
				t.Fatal(err)
			}
			if russianMessages[key] == "" {
				t.Errorf("%s: missing translation for %q", entry.Name(), key)
			}
		}
	}
	for _, drill := range drills.All() {
		for _, text := range append([]string{drill.Title, drill.Where, drill.Check}, drill.Steps...) {
			if russianMessages[text] == "" {
				t.Errorf("%s: missing drill translation for %q", drill.ID, text)
			}
		}
	}
}
