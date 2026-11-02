package coach

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Ollama is a Provider for a local Ollama server using its native API, which
// unlike Ollama's OpenAI-compatible endpoint lets us set the context size.
type Ollama struct {
	BaseURL string // default http://localhost:11434
	Model   string
	// NumCtx is the context window in tokens; Ollama's default (4096) is too
	// small for a match. Zero means 16384.
	NumCtx int
	Client *http.Client
}

type ollamaRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format"`
	Think    bool          `json:"think"`
	Options  struct {
		NumCtx      int     `json:"num_ctx"`
		Temperature float64 `json:"temperature"`
	} `json:"options"`
}

type ollamaResponse struct {
	Message chatMessage `json:"message"`
	Error   string      `json:"error"`
}

func (o Ollama) Complete(ctx context.Context, system, user string) (string, error) {
	req := ollamaRequest{
		Model: o.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Format: "json",
	}
	req.Options.NumCtx = o.NumCtx
	if req.Options.NumCtx == 0 {
		req.Options.NumCtx = 16384
	}
	req.Options.Temperature = 0.2
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	base := o.BaseURL
	if base == "" {
		base = "http://localhost:11434"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(base, "/")+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := o.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("ollama response: %w", err)
	}
	var out ollamaResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("ollama response (status %d): %w", resp.StatusCode, err)
	}
	if out.Error != "" || resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, out.Error)
	}
	return out.Message.Content, nil
}
