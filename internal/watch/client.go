package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrUnauthorized means the upload token is missing, wrong or revoked.
var ErrUnauthorized = errors.New("watch: upload token rejected")

// Client uploads demos to POST /api/demos with a user's upload token.
type Client struct {
	BaseURL string
	Token   string
	// HTTP defaults to a client with uploadTimeout.
	HTTP *http.Client
}

// uploadTimeout bounds one upload, so a stalled connection cannot stop the
// watcher for good. A 300 MB demo takes about 4 minutes at 10 Mbit/s up.
const uploadTimeout = 30 * time.Minute

var defaultClient = &http.Client{Timeout: uploadTimeout}

// uploadResponse mirrors web.UploadResponse; it is declared here so the CLI
// does not link the web server.
type uploadResponse struct {
	URL       string `json:"url"`
	Duplicate bool   `json:"duplicate"`
}

// Upload streams a demo file to the service.
func (c Client) Upload(ctx context.Context, path string) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Result{}, err
	}

	base := strings.TrimSuffix(c.BaseURL, "/")
	// The file's time is when CS2 saved the demo, right after the match.
	q := url.Values{"name": {filepath.Base(path)}, "played": {info.ModTime().UTC().Format(time.RFC3339)}}
	target := base + "/api/demos?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, f)
	if err != nil {
		return Result{}, err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	client := c.HTTP
	if client == nil {
		client = defaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var body uploadResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return Result{}, fmt.Errorf("watch: decode upload response: %w", err)
		}
		return Result{URL: base + body.URL, Duplicate: body.Duplicate}, nil
	case http.StatusUnauthorized:
		return Result{}, ErrUnauthorized
	case http.StatusTooManyRequests:
		return Result{}, ErrQuota
	case http.StatusUnsupportedMediaType, http.StatusRequestEntityTooLarge:
		return Result{}, fmt.Errorf("%w: %s", ErrRejected, message(resp))
	default:
		return Result{}, fmt.Errorf("watch: upload: %s: %s", resp.Status, message(resp))
	}
}

func message(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return strings.TrimSpace(string(b))
}
