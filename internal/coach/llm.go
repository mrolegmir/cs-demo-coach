package coach

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mrolegmir/cs-demo-coach/internal/drills"
)

// Provider sends a chat completion request to an LLM and returns the
// assistant's reply. Implementations must ask the model for a JSON object.
type Provider interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// LLM is a coach that asks a language model to prioritize and explain the
// findings. Replies that fail Validate are sent back once with the errors;
// a second failure is returned as an error rather than shown to the user.
type LLM struct {
	Provider Provider
}

// ErrUngrounded is returned when the model keeps producing advice that does
// not pass validation.
var ErrUngrounded = errors.New("coach: model advice failed validation")

func (c LLM) Advise(ctx context.Context, in *Input) (*Advice, error) {
	if err := in.ValidateIdentity(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUngrounded, err)
	}
	// A model cannot add a grounded focus when no issue stands out. Keep
	// concrete pair advice, but avoid spending a call on an empty focus list.
	hasIssue := false
	for _, ps := range in.Stats {
		for _, is := range ps.Issues {
			hasIssue = hasIssue || is.StandsOut
		}
	}
	if !hasIssue {
		return (Rules{PerPlayer: 2}).Advise(ctx, in)
	}
	// The prompt carries stats and examples, not the whole evidence table.
	input, err := json.Marshal(struct {
		Language string                   `json:"language"`
		Map      string                   `json:"map,omitempty"`
		Rounds   int                      `json:"rounds,omitempty"`
		Period   string                   `json:"period,omitempty"`
		Matches  int                      `json:"matches,omitempty"`
		Party    []string                 `json:"party"`
		Stats    []PlayerStats            `json:"players"`
		Pairs    []Pair                   `json:"pairs,omitempty"`
		Roles    []Role                   `json:"roles,omitempty"`
		Kinds    map[string]kindInfo      `json:"kinds"`
		Drills   map[string][]drillChoice `json:"drills"`
	}{Language(in.Language), in.Map, in.Rounds, in.Period, in.Matches, in.Party, in.Stats, in.Pairs, in.Roles, kindTable(), drillTable(in)})
	if err != nil {
		return nil, err
	}
	user := "Match data:\n" + string(input)
	prompt := systemPrompt + "\n\n" + outputLanguage(in.Language)

	reply, err := c.Provider.Complete(ctx, prompt, user)
	if err != nil {
		return nil, err
	}
	a, verr := parseAdvice(reply, in)
	if verr == nil {
		return in.finish(a), nil
	}

	retry := user + "\n\nYour previous answer was:\n" + reply +
		"\n\nIt was rejected for these reasons:\n" + verr.Error() +
		"\n\nReturn a corrected JSON object."
	reply, err = c.Provider.Complete(ctx, prompt, retry)
	if err != nil {
		return nil, err
	}
	a, verr = parseAdvice(reply, in)
	if verr != nil {
		return nil, fmt.Errorf("%w: %w", ErrUngrounded, verr)
	}
	return in.finish(a), nil
}

func parseAdvice(reply string, in *Input) (*Advice, error) {
	var a *Advice
	if err := json.Unmarshal([]byte(extractJSON(reply)), &a); err != nil {
		return nil, fmt.Errorf("reply is not valid JSON: %w", err)
	}
	if err := in.Validate(a); err != nil {
		return nil, err
	}
	return a, nil
}

// extractJSON strips a markdown fence or leading reasoning text some models
// add around the object.
func extractJSON(s string) string {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return s
	}
	return s[start : end+1]
}

// drillChoice is what the model needs to choose a drill; the steps stay on
// the page.
type drillChoice struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Where string `json:"where"`
	Check string `json:"check"`
}

// drillTable lists the library drills for the kinds that stand out for
// someone, which are the only ones a focus area can be about.
func drillTable(in *Input) map[string][]drillChoice {
	t := map[string][]drillChoice{}
	for _, ps := range in.Stats {
		for _, is := range ps.Issues {
			if !is.StandsOut || t[string(is.Kind)] != nil {
				continue
			}
			for _, d := range drills.ForKind(is.Kind) {
				t[string(is.Kind)] = append(t[string(is.Kind)], drillChoice{d.ID, d.Title, d.Where, d.Check})
			}
		}
	}
	return t
}

func kindTable() map[string]kindInfo {
	t := make(map[string]kindInfo, len(kinds))
	for k, v := range kinds {
		t[string(k)] = v
	}
	return t
}

const systemPrompt = `You are a Counter-Strike 2 coach for casual players who play a few hours a week, usually as a party of friends. They cannot work on twenty things; they need the one to three that matter most, and proof.

You receive mistakes that deterministic detectors found, either in one match ("map", "rounds") or across the party's matches of a period ("period", "matches"). Per party member you get: count, chances (how often the mistake was possible), the player's rate, the lobby rate (all players in the same matches), and example findings with IDs. For a period, issues may also carry "status": "active" (a habit), "improving", "resolved" or "one_off" (bad games); mention improvements briefly and focus on active habits. An issue may carry "practice": what the player logged practicing for it and the rate before and after they started; "verdict" is "working", "not_yet", "too_early" (fewer than 3 matches since) or "no_baseline" (no matches before the practice to compare with). "drills" in practice lists the library drills they logged. Say whether the drill is paying off using those numbers, and if the verdict is "not_yet", pick a different drill. "places" says where on the map an issue's findings happened, most first, and examples carry their "place". "kinds" explains what each detector measures. "drills" is the drill library for the kinds that stand out: every focus area must use one of them.

Your job:
1. For each party member pick at most 3 distinct focus areas, ONLY from issues with "stands_out": true. Choose issues that matter for winning rounds. The application ranks the selected issues: active habits before improving ones, then by excess occurrences against the lobby, with a computed explanation. Do not supply a "priority" field. It is fine to give a player zero focus areas.
2. For each focus area explain WHY it costs rounds, using the numbers and concrete examples (round, time), pick ONE drill from "drills" for that kind (its "id" goes in "drill_id"), and say in "drill" how it applies to this player's examples. When most findings happened at one or two places, name them in "drill": that is where to start (for a lineup drill, the lineup for that spot). Do not repeat the drill's steps; the page shows them.
3. Add team advice (0-2 items) when several party members share a problem or when a problem is about coordination (trades, buys, flash support). "pairs" says, for two party members, how often "player" was traded when "partner" was the nearest teammate: a pair with a low rate over several chances is a concrete team item ("when you die near Anna, she trades 1 of 6"), backed by missed_trade findings of the partner. Name who should play closer or swing first. "roles" says what each member does: how often they are in the round's first fight against the lobby, how many of their flashes blind an enemy per ten rounds, how much they play the AWP, and the "labels" this adds up to. Use roles for team advice: an entry who keeps dying first without a flash needs a named teammate to flash for them, ideally the one whose flashes already blind enemies most; two players fighting for the same role, or nobody in it, is worth saying.
4. Put caveats in "notes", e.g. a short match or too few chances to judge.

Rules:
- Only use facts from the data. Do not invent rounds, numbers or events.
- Every focus area and team item must list evidence: finding IDs from the examples. A focus area's evidence must include at least one finding of that kind whose primary player is the person receiving the advice; being related to someone else's mistake is not enough.
- "player" must be a name from "party", exactly as written.
- Write to the player in the second person ("you"), never "he" or "she".
- Never write finding IDs (like "f12" or "m3-f12") in the text; they go only in "evidence". Refer to examples by round, and by the "match" field when there is one (e.g. "round 7 on de_dust2, Oct 24"). Some examples describe a whole round and have no time; do not invent one.
- Rates in the data are fractions; write them as whole percentages (0.37 -> "37%"), e.g. "77% vs 49% lobby". Do not round counts.
- Be direct and specific. No generic advice like "communicate more" without saying what to communicate.

Reply with a single JSON object and nothing else:
{
  "players": [{"player": "<name>", "focus": [{"kind": "<kind>", "title": "<short imperative>", "why": "<1-3 sentences>", "drill_id": "<id from drills>", "drill": "<1-2 sentences>", "evidence": ["f12", "f40"]}]}],
  "team": [{"title": "<short>", "why": "<1-3 sentences>", "suggestion": "<1-2 sentences>", "evidence": ["f7"]}],
  "notes": ["<optional caveat>"]
}`
