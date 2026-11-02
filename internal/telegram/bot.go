package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// Store is what the bot needs from the store.
type Store interface {
	UseTelegramCode(ctx context.Context, codeHash []byte, chatID int64) (match.PlayerID, error)
	DeleteTelegramChat(ctx context.Context, chatID int64) (bool, error)
	BriefsToNotify(ctx context.Context, since time.Time) ([]store.BriefNotice, error)
	MarkBriefNotified(ctx context.Context, id int64) error
}

// CodeTTL is how long a link code works.
const CodeTTL = 15 * time.Minute

// NewCode returns a one-time link code and its hash. The code is the deep
// link's start parameter, which allows letters and digits.
func NewCode() (code string, hash []byte) {
	code = rand.Text()
	return code, HashCode(code)
}

// HashCode is what the store keeps of a link code.
func HashCode(code string) []byte {
	h := sha256.Sum256([]byte(code))
	return h[:]
}

// Bot answers /start and /stop and sends ready briefs.
type Bot struct {
	API   *Client
	Store Store
	// PublicURL is the site's base URL, for links to briefs.
	PublicURL string
	Log       *slog.Logger

	name atomic.Pointer[string]
}

// Name is the bot's username, or "" until Run has reached Telegram.
func (b *Bot) Name() string {
	if n := b.name.Load(); n != nil {
		return *n
	}
	return ""
}

// Run asks Telegram for the bot's username, retrying until it answers, then
// handles messages and sends ready briefs every minute until ctx is done.
// The site works meanwhile, without the Telegram button.
func (b *Bot) Run(ctx context.Context) {
	for wait := 10 * time.Second; ; wait = min(2*wait, 10*time.Minute) {
		name, err := b.API.Username(ctx)
		if err == nil {
			b.name.Store(&name)
			b.Log.Info("telegram delivery enabled", "bot", name)
			break
		}
		b.Log.Warn("telegram bot not reachable yet", "err", err, "retry_in", wait)
		if !sleep(ctx, wait) {
			return
		}
	}
	go b.Notify(ctx, time.Minute)
	b.Listen(ctx)
}

// pollWait is the long-poll time in seconds; the HTTP client's timeout must
// be longer.
const pollWait = 50

// maxAge is how long ago a brief may have become ready and still be sent:
// an api that was down for days should not deliver last week's brief.
const maxAge = 48 * time.Hour

// Listen handles incoming messages until ctx is done.
func (b *Bot) Listen(ctx context.Context) {
	var offset int64
	for ctx.Err() == nil {
		us, err := b.API.Updates(ctx, offset, pollWait)
		if err != nil {
			if ctx.Err() == nil {
				b.Log.Warn("telegram updates", "err", err)
				sleep(ctx, 10*time.Second)
			}
			continue
		}
		for _, u := range us {
			offset = max(offset, u.ID+1)
			if u.Message != nil {
				b.handle(ctx, u.Message)
			}
		}
	}
}

// Replies to the bot's commands.
const (
	replyLinked    = "Linked to your CS Demo Coach account. Your weekly briefs will arrive here. Send /stop to unlink."
	replyExpired   = "This link has expired or was already used. Press \"Connect Telegram\" on the site again."
	replyHelp      = "To get your weekly briefs here, press \"Connect Telegram\" on the CS Demo Coach home page."
	replyStopped   = "Unlinked. Briefs will no longer be sent here."
	replyNotLinked = "This chat is not linked to an account."
	replyPrivate   = "Briefs are personal, so this bot only works in a private chat."
	replyFailed    = "Something went wrong on our side. Try again in a minute."
)

func (b *Bot) handle(ctx context.Context, m *Message) {
	cmd, arg, _ := strings.Cut(strings.TrimSpace(m.Text), " ")
	cmd, _, _ = strings.Cut(cmd, "@") // "/stop@SomeBot" in menus
	reply := replyHelp
	switch {
	case m.Chat.Type != "private":
		// Answer commands only: in a group, every other message (and the
		// "bot was added" service message) is not for the bot.
		if !strings.HasPrefix(cmd, "/") {
			return
		}
		reply = replyPrivate
	case cmd == "/start" && arg != "":
		id, err := b.Store.UseTelegramCode(ctx, HashCode(strings.TrimSpace(arg)), m.Chat.ID)
		switch {
		case errors.Is(err, store.ErrNotFound):
			reply = replyExpired
		case err != nil:
			b.Log.Error("telegram link", "err", err)
			reply = replyFailed
		default:
			b.Log.Info("telegram linked", "user", id)
			reply = replyLinked
		}
	case cmd == "/stop":
		was, err := b.Store.DeleteTelegramChat(ctx, m.Chat.ID)
		switch {
		case err != nil:
			b.Log.Error("telegram unlink", "err", err)
			reply = replyFailed
		case was:
			reply = replyStopped
		default:
			reply = replyNotLinked
		}
	}
	if err := b.API.Send(ctx, m.Chat.ID, reply); err != nil {
		b.Log.Warn("telegram reply", "err", err)
	}
}

// Notify sends ready briefs every interval until ctx is done.
func (b *Bot) Notify(ctx context.Context, every time.Duration) {
	for {
		if err := b.NotifyOnce(ctx); err != nil && ctx.Err() == nil {
			b.Log.Error("telegram notify", "err", err)
		}
		if !sleep(ctx, every) {
			return
		}
	}
}

// NotifyOnce sends every ready brief that was not sent yet. A brief that
// fails to send for a passing reason is tried again next time, until it is
// maxAge old; one Telegram refuses for good is marked and not retried.
func (b *Bot) NotifyOnce(ctx context.Context) error {
	ns, err := b.Store.BriefsToNotify(ctx, time.Now().Add(-maxAge))
	if err != nil {
		return err
	}
	var errs []error
	for _, n := range ns {
		br := &n.Brief
		sendErr := b.API.Send(ctx, n.ChatID, Text(br, b.PublicURL))
		if errors.Is(sendErr, ErrGone) {
			// Nobody reads this chat any more; stop sending there.
			b.Log.Info("telegram chat is gone, unlinking", "user", br.SteamID)
			if _, err := b.Store.DeleteTelegramChat(ctx, n.ChatID); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if apiErr, ok := errors.AsType[*APIError](sendErr); ok && apiErr.Permanent() {
			b.Log.Error("telegram refused a brief, not retrying", "brief", br.ID, "err", sendErr)
		} else if sendErr != nil {
			errs = append(errs, sendErr)
			continue
		}
		if err := b.Store.MarkBriefNotified(context.WithoutCancel(ctx), br.ID); err != nil {
			errs = append(errs, err)
			continue
		}
		if sendErr == nil {
			b.Log.Info("brief sent to telegram", "brief", br.ID)
		}
	}
	return errors.Join(errs...)
}

// maxText keeps a message under Telegram's limit of 4096 UTF-16 code units,
// with room for the link.
const maxText = 3800

// Text is the message for a ready brief: each player's focus titles and the
// team's, with a link to the full brief and its evidence.
func Text(br *store.Brief, publicURL string) string {
	var sb strings.Builder
	russian := br.Advice != nil && br.Advice.Language == "ru"
	if russian {
		fmt.Fprintf(&sb, "Твой недельный разбор за %s – %s готов.\n", br.PeriodStart.Format("02.01.2006"), br.PeriodEnd.Format("02.01.2006"))
	} else {
		fmt.Fprintf(&sb, "Your weekly brief for %s – %s is ready.\n", br.PeriodStart.Format("Jan 2"), br.PeriodEnd.Format("Jan 2"))
	}
	if a := br.Advice; a != nil {
		for _, p := range a.Players {
			if len(p.Focus) == 0 {
				continue
			}
			fmt.Fprintf(&sb, "\n%s\n", p.Player)
			for _, f := range p.Focus {
				fmt.Fprintf(&sb, "• %s\n", f.Title)
			}
		}
		if len(a.Team) > 0 {
			if russian {
				sb.WriteString("\nКоманда\n")
			} else {
				sb.WriteString("\nTeam\n")
			}
			for _, t := range a.Team {
				fmt.Fprintf(&sb, "• %s\n", t.Title)
			}
		}
	}
	footer := "\nWhy, with the rounds to watch, and the drills: %s/briefs/%d"
	if russian {
		footer = "\nОбъяснения, эпизоды и упражнения: %s/briefs/%d"
	}
	return truncate(sb.String(), maxText) + fmt.Sprintf(footer, publicURL, br.ID)
}

// truncate cuts s to at most n UTF-16 code units, which is how Telegram
// counts length, marking the cut.
func truncate(s string, n int) string {
	units, cut := 0, -1
	for i, r := range s {
		if cut < 0 && units+utf16.RuneLen(r) > n-2 { // room for the mark
			cut = i
		}
		units += utf16.RuneLen(r)
	}
	if units <= n {
		return s
	}
	return s[:cut] + "…\n"
}

// sleep waits d or until ctx is done, and reports whether d passed.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
