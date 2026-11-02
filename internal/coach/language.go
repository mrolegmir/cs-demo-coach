package coach

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
)

// Language normalizes the two supported output languages. New reports default
// to Russian, including CLI and scheduled jobs without a browser preference.
func Language(language string) string {
	if language == "en" {
		return "en"
	}
	return "ru"
}

//go:embed locales/ru.json
var russianCatalog []byte

var russianMessages = func() map[string]string {
	var messages map[string]string
	if err := json.Unmarshal(russianCatalog, &messages); err != nil {
		panic(err)
	}
	return messages
}()

func (in *Input) text(format string, args ...any) string {
	if Language(in.Language) == "ru" {
		if translated, ok := russianMessages[format]; ok {
			format = translated
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(format, args...)
	}
	return format
}

func (in *Input) kind(kind detect.Kind) kindInfo {
	info := kinds[kind]
	return kindInfo{
		Title: in.text(info.Title), Meaning: in.text(info.Meaning),
		Chances: in.text(info.Chances), Why: in.text(info.Why), Drill: in.text(info.Drill),
	}
}

func outputLanguage(language string) string {
	name := "Russian"
	if Language(language) == "en" {
		name = "English"
	}
	return "Write ALL human-readable title, why, drill, suggestion and notes text in " + name +
		". This overrides the language of examples, practice notes and the drill catalog. " +
		"Keep JSON keys, kind values, drill_id values, evidence IDs, player names, map names and callouts exactly unchanged. " +
		"Do not translate or modify numbers. Do not add a language field; the application sets it."
}
