package coach

import (
	"cmp"
	"fmt"
	"os"
)

// FromEnv builds the coach selected by the environment and returns it with
// a display name:
//
//	CSDC_LLM           rules (default, no LLM) | ollama | openai | codex
//	CSDC_LLM_MODEL     model name, required for ollama and openai; optional
//	                   for codex (the CLI's default model)
//	CSDC_LLM_BASE_URL  server URL (defaults: http://localhost:11434 for ollama,
//	                   https://api.openai.com/v1 for openai)
//	CSDC_LLM_API_KEY   API key for openai
func FromEnv() (Coach, string, error) {
	kind := cmp.Or(os.Getenv("CSDC_LLM"), "rules")
	model := os.Getenv("CSDC_LLM_MODEL")
	base := os.Getenv("CSDC_LLM_BASE_URL")
	if (kind == "ollama" || kind == "openai") && model == "" {
		return nil, "", fmt.Errorf("CSDC_LLM=%s needs CSDC_LLM_MODEL", kind)
	}
	switch kind {
	case "rules":
		return Rules{PerPlayer: 2}, "rules", nil
	case "ollama":
		return LLM{Provider: Ollama{BaseURL: base, Model: model}}, "ollama/" + model, nil
	case "openai":
		return LLM{Provider: OpenAI{
			BaseURL: cmp.Or(base, "https://api.openai.com/v1"),
			Model:   model,
			APIKey:  os.Getenv("CSDC_LLM_API_KEY"),
		}}, model, nil
	case "codex":
		name := "codex"
		if model != "" {
			name += "/" + model
		}
		return LLM{Provider: Codex{Model: model}}, name, nil
	default:
		return nil, "", fmt.Errorf("unknown CSDC_LLM %q", kind)
	}
}
