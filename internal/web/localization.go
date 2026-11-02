package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const languageCookie = "csdc_language"

//go:embed locales/ru.json
var russianCatalog []byte

var russianMessages = func() map[string]string {
	var messages map[string]string
	if err := json.Unmarshal(russianCatalog, &messages); err != nil {
		panic(err) // An invalid embedded catalog is a build-time defect.
	}
	return messages
}()

func requestLanguage(r *http.Request) string {
	if c, err := r.Cookie(languageCookie); err == nil && c.Value == "en" {
		return "en"
	}
	return "ru"
}

// English source strings are also the fallback for saved report content and
// unknown messages. Translation always returns plain text for html/template.
func translate(language, text string, args ...any) string {
	if language == "ru" {
		if translated, ok := russianMessages[text]; ok {
			text = translated
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

func (s *Server) setLanguage(w http.ResponseWriter, r *http.Request) {
	if s.Auth != nil && !s.sameOrigin(r) {
		http.Error(w, "cross-site request", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	language := r.PostForm.Get("lang")
	if language != "ru" && language != "en" {
		http.Error(w, "unsupported language", http.StatusBadRequest)
		return
	}
	secure := r.TLS != nil || (s.Auth != nil && strings.HasPrefix(s.Auth.PublicURL, "https://"))
	http.SetCookie(w, &http.Cookie{
		Name: languageCookie, Value: language, Path: "/", MaxAge: 365 * 24 * 60 * 60,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	// Only local absolute paths may be used as the return destination.
	destination := r.PostForm.Get("return")
	u, err := url.Parse(destination)
	if err != nil || !strings.HasPrefix(destination, "/") || strings.HasPrefix(destination, "//") ||
		strings.ContainsAny(destination, "\\\r\n") || u.IsAbs() || u.Host != "" {
		destination = "/"
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
}
