// Package importer finds new matches through Steam match sharing codes.
//
// For every user who linked their match history it walks
// GetNextMatchSharingCode from the newest known code and stores the codes it
// finds. Turning a code into a demo needs the CS2 Game Coordinator, which
// only answers a signed-in Steam client; until a bot account does that
// (ADR 9), the codes wait.
package importer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/sharecode"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// MaxPerCheck bounds the codes read for one user in one check, so a link
// set to a months-old code catches up over several rounds.
const MaxPerCheck = 20

// Messages shown to the user when Steam rejects their link.
const (
	MsgAuthCode    = "Steam rejected the match history authentication code. Create a new one and save it again."
	MsgUnknownCode = "Steam does not know this share code for your account. Paste the code of one of your recent matches."
)

// Store is what the importer needs from the store.
type Store interface {
	SteamLinksToCheck(ctx context.Context, before time.Time) ([]store.SteamLink, error)
	RecordShareCodes(ctx context.Context, link store.SteamLink, codes []string, linkErr string) (bool, error)
}

// API is the part of the Steam Web API the importer calls.
type API interface {
	NextCode(ctx context.Context, player match.PlayerID, authCode, known string) (string, error)
	KeyValid(ctx context.Context, player match.PlayerID) (bool, error)
}

// Importer polls Steam for new share codes.
type Importer struct {
	Store Store
	API   API
	Log   *slog.Logger
	// Every is how often each link is checked.
	Every time.Duration
}

// Run checks links until ctx is canceled. It wakes up four times per Every,
// so a link is checked about Every after its last check rather than up to
// twice that.
func (im *Importer) Run(ctx context.Context) {
	for {
		if err := im.RunOnce(ctx, time.Now()); err != nil && ctx.Err() == nil {
			im.Log.Error("share codes", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(im.Every / 4):
		}
	}
}

// errBadKey stops a round: every link would fail the same way, and none of
// them is at fault.
var errBadKey = errors.New("importer: Steam rejected the API key (CSDC_STEAM_API_KEY)")

// RunOnce checks every link that was not checked during the last Every.
// Only Steam rejecting the user's own codes disables a link. Any other error
// still records the check with the codes found before it, so the link is
// retried after Every rather than on every wake-up.
func (im *Importer) RunOnce(ctx context.Context, now time.Time) error {
	links, err := im.Store.SteamLinksToCheck(ctx, now.Add(-im.Every))
	if err != nil {
		return err
	}
	var errs []error
	for _, l := range links {
		codes, linkErr, err := im.check(ctx, l)
		if errors.Is(err, errBadKey) {
			return err
		}
		if err != nil {
			im.Log.Warn("share codes: check failed", "user", l.SteamID, "err", err)
		}
		stored, err := im.Store.RecordShareCodes(ctx, l, codes, linkErr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if stored && len(codes) > 0 {
			im.Log.Info("share codes found", "user", l.SteamID, "count", len(codes))
		}
	}
	return errors.Join(errs...)
}

// check walks one user's history. It returns the codes found so far even
// when it stops on an error, and the message for the user when the error is
// theirs to fix.
func (im *Importer) check(ctx context.Context, l store.SteamLink) (codes []string, linkErr string, err error) {
	known := l.LastCode
	for range MaxPerCheck {
		next, err := im.API.NextCode(ctx, l.SteamID, l.AuthCode, known)
		switch {
		case errors.Is(err, ErrForbidden):
			ok, kerr := im.API.KeyValid(ctx, l.SteamID)
			if kerr != nil {
				return codes, "", kerr
			}
			if !ok {
				return nil, "", errBadKey
			}
			return codes, MsgAuthCode, nil
		case errors.Is(err, ErrUnknownCode):
			return codes, MsgUnknownCode, nil
		case err != nil:
			return codes, "", err
		}
		if next == "" {
			return codes, "", nil
		}
		if next, err = sharecode.Normalize(next); err != nil {
			return codes, "", err
		}
		codes = append(codes, next)
		known = next
	}
	return codes, "", nil
}
