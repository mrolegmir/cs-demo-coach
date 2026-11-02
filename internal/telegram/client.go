// Package telegram sends weekly briefs to Telegram (ADR 13): a small Bot API
// client, and a bot that links chats to accounts and sends ready briefs.
package telegram

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DefaultAPI is the Telegram Bot API.
const DefaultAPI = "https://api.telegram.org"

// ErrGone means nothing can be sent to the chat any more: the user blocked
// the bot (403) or the chat no longer exists.
var ErrGone = errors.New("telegram: the chat is gone")

// APIError is a request the Bot API refused.
type APIError struct {
	Method      string
	Status      int
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram: %s: %d %s", e.Method, e.Status, e.Description)
}

// Permanent reports whether repeating the request cannot help: a 4xx other
// than flood control (429).
func (e *APIError) Permanent() bool {
	return e.Status >= 400 && e.Status < 500 && e.Status != http.StatusTooManyRequests
}

// Client calls the Bot API.
type Client struct {
	Token string
	Base  string       // default DefaultAPI
	HTTP  *http.Client // default http.DefaultClient; its timeout must exceed long polls
}

// Update is an incoming update; only messages are asked for.
type Update struct {
	ID      int64    `json:"update_id"`
	Message *Message `json:"message"`
}

// Message is an incoming message.
type Message struct {
	Chat Chat   `json:"chat"`
	Text string `json:"text"`
}

// Chat is where a message came from.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private" for a chat with one user
}

// Username returns the bot's @username, which deep links need.
func (c *Client) Username(ctx context.Context) (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	if err := c.call(ctx, "getMe", struct{}{}, &me); err != nil {
		return "", err
	}
	if me.Username == "" {
		return "", errors.New("telegram: getMe: no username")
	}
	return me.Username, nil
}

// Updates long-polls for messages after offset for up to wait seconds.
func (c *Client) Updates(ctx context.Context, offset int64, wait int) ([]Update, error) {
	params := map[string]any{"offset": offset, "timeout": wait, "allowed_updates": []string{"message"}}
	var us []Update
	return us, c.call(ctx, "getUpdates", params, &us)
}

// Send sends a plain-text message.
func (c *Client) Send(ctx context.Context, chatID int64, text string) error {
	params := map[string]any{"chat_id": chatID, "text": text, "link_preview_options": map[string]bool{"is_disabled": true}}
	return c.call(ctx, "sendMessage", params, &struct{}{})
}

// call posts params to a Bot API method and decodes the result into out.
// Errors never include the URL: it carries the token.
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cmp.Or(c.Base, DefaultAPI)+"/bot"+c.Token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: %s: bad request", method)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cmp.Or(c.HTTP, http.DefaultClient).Do(req)
	if err != nil {
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}
		return fmt.Errorf("telegram: %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var r struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&r)
	switch {
	case resp.StatusCode == http.StatusForbidden,
		resp.StatusCode == http.StatusBadRequest && strings.Contains(r.Description, "chat not found"):
		return ErrGone
	case resp.StatusCode != http.StatusOK || decodeErr != nil || !r.OK:
		return &APIError{Method: method, Status: resp.StatusCode, Description: cmp.Or(r.Description, resp.Status)}
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		return fmt.Errorf("telegram: %s: decode: %w", method, err)
	}
	return nil
}
