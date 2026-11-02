// Command csdc runs the CS Demo Coach pipeline locally from the command line.
//
//	csdc parse match.dem     # print the parsed match as JSON
//	csdc detect match.dem    # print detector findings and per-player stats as JSON
//	csdc analyze match.dem   # parse, detect and coach; print a readable report
//	csdc watch               # upload new demos from the CS2 replays folder
//
// analyze uses the rule-based coach by default; see usage for the LLM
// settings.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/demo"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/watch"
)

const usage = `usage:
  csdc mechanics <demo.dem|match.json.gz>       export anonymized shooting evidence as JSON
  csdc parse <demo.dem>                         print the parsed match as JSON
  csdc detect <demo.dem>                        print findings and per-player stats as JSON
  csdc analyze [-lang ru|en] [-party a,b,c] [-json] <demo.dem>  coach the party; default party is
                                                the team that started as T

environment for analyze:
  CSDC_LLM           rules (default, no LLM) | ollama | openai | codex
  CSDC_LLM_MODEL     model name, required for ollama and openai; optional for
                     codex (uses a ChatGPT subscription via "codex login")
  CSDC_LLM_BASE_URL  server URL (defaults: http://localhost:11434 for ollama,
                     https://api.openai.com/v1 for openai; any OpenAI-compatible
                     server works)
  CSDC_LLM_API_KEY   API key for openai

  csdc watch [-dir folder] [-all]                upload every demo downloaded into the
                                                CS2 replays folder (found through Steam)

environment for watch:
  CSDC_API_URL       the service's URL
  CSDC_UPLOAD_TOKEN  an upload token from the site's Auto-upload page`

func main() {
	if err := mainErr(); err != nil {
		fmt.Fprintln(os.Stderr, "csdc:", err)
		os.Exit(1)
	}
}

func mainErr() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case "mechanics":
		if len(args) != 1 {
			return errors.New(usage)
		}
		var m *match.Match
		var err error
		if strings.HasSuffix(args[0], ".json.gz") {
			m, err = match.LoadFile(args[0])
		} else {
			m, err = parseFile(args[0])
		}
		if err != nil {
			return err
		}
		m.Anonymize()
		return printJSON(out, m.MechanicsReport())
	case "parse", "detect":
		if len(args) != 1 {
			return errors.New(usage)
		}
		m, err := parseFile(args[0])
		if err != nil {
			return err
		}
		if cmd == "parse" {
			return printJSON(out, m)
		}
		return printJSON(out, detect.Run(m, detect.Default()))
	case "analyze":
		return analyze(ctx, args, out)
	case "watch":
		return watchDemos(ctx, args)
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

func analyze(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	partyFlag := fs.String("party", "", "comma-separated player names to coach")
	asJSON := fs.Bool("json", false, "print advice as JSON")
	language := fs.String("lang", "ru", "advice language: ru or en")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *language != "ru" && *language != "en" {
		return errors.New("language must be ru or en")
	}
	if fs.NArg() != 1 {
		return errors.New(usage)
	}

	m, err := parseFile(fs.Arg(0))
	if err != nil {
		return err
	}
	party, err := pickParty(m, *partyFlag)
	if err != nil {
		return err
	}
	rep := detect.Run(m, detect.Default())
	in := coach.NewInput(m, rep, party)
	in.Language = *language

	c, name, err := coach.FromEnv()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	advice, err := c.Advise(ctx, in)
	if err != nil {
		return fmt.Errorf("%s coach: %w", name, err)
	}
	if *asJSON {
		return printJSON(out, advice)
	}
	_, err = io.WriteString(out, formatAdvice(m, in, advice, name))
	return err
}

// pickParty resolves player names, or defaults to the team that played T in
// the first round.
func pickParty(m *match.Match, names string) ([]match.PlayerID, error) {
	var party []match.PlayerID
	if names == "" {
		for _, p := range m.Rounds[0].Players {
			if p.Side == match.SideT {
				party = append(party, p.Player)
			}
		}
		return party, nil
	}
	for name := range strings.SplitSeq(names, ",") {
		name = strings.TrimSpace(name)
		found := false
		for _, p := range m.Players {
			if strings.EqualFold(p.Name, name) {
				party = append(party, p.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("player %q is not in this match", name)
		}
	}
	return party, nil
}

func formatAdvice(m *match.Match, in *coach.Input, a *coach.Advice, coachName string) string {
	var w strings.Builder
	fmt.Fprintf(&w, "%s, %d rounds, coach: %s\n", m.Map, len(m.Rounds), coachName)
	evidence := func(ids []string) {
		for _, id := range ids {
			if f, ok := in.Evidence[id]; ok {
				fmt.Fprintf(&w, "        [%s] round %d, %s: %s\n", id, f.Round, match.Clock(f.Time), f.Detail)
			}
		}
	}
	for _, pa := range a.Players {
		fmt.Fprintf(&w, "\n== %s\n", pa.Player)
		if len(pa.Focus) == 0 {
			fmt.Fprintln(&w, "  nothing stands out this match")
		}
		for i, f := range pa.Focus {
			fmt.Fprintf(&w, "  %d. %s\n     Why: %s\n     Drill: %s\n", i+1, f.Title, f.Why, f.Drill)
			if f.Priority != "" {
				fmt.Fprintf(&w, "     %s\n", f.Priority)
			}
			if d, ok := drills.ByID(f.DrillID); ok {
				fmt.Fprintf(&w, "     %s (%s):\n", d.Title, d.Where)
				for _, step := range d.Steps {
					fmt.Fprintf(&w, "       - %s\n", step)
				}
				fmt.Fprintf(&w, "     Done right: %s\n", d.Check)
			}
			fmt.Fprintf(&w, "     Evidence:\n")
			evidence(f.Evidence)
		}
	}
	if len(a.Team) > 0 {
		fmt.Fprintln(&w, "\n== Team")
		for i, t := range a.Team {
			fmt.Fprintf(&w, "  %d. %s\n     Why: %s\n     Try: %s\n     Evidence:\n", i+1, t.Title, t.Why, t.Suggestion)
			evidence(t.Evidence)
		}
	}
	if len(a.Notes) > 0 {
		fmt.Fprintln(&w, "\nNotes:")
		for _, n := range a.Notes {
			fmt.Fprintf(&w, "  - %s\n", n)
		}
	}
	return w.String()
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func parseFile(path string) (*match.Match, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return demo.Parse(f)
}

func watchDemos(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	dir := fs.String("dir", "", "folder to watch (default: the CS2 replays folder)")
	all := fs.Bool("all", false, "also upload demos that are already in the folder")
	stateFile := fs.String("state", "", "file that remembers uploaded demos (default: in the user config dir)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	apiURL := os.Getenv("CSDC_API_URL")
	if apiURL == "" {
		return errors.New("set CSDC_API_URL to the service's URL")
	}
	if *dir == "" {
		if *dir = watch.FindReplays(); *dir == "" {
			return errors.New("CS2 replays folder not found; pass -dir")
		}
	}
	if *stateFile == "" {
		cfg, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(cfg, "csdc"), 0o700); err != nil {
			return err
		}
		*stateFile = filepath.Join(cfg, "csdc", "watch-state.json")
	}

	w := &watch.Watcher{
		Dir:       *dir,
		Uploader:  watch.Client{BaseURL: apiURL, Token: os.Getenv("CSDC_UPLOAD_TOKEN")},
		StateFile: *stateFile,
		All:       *all,
		Poll:      30 * time.Second,
		Settle:    20 * time.Second,
		QuotaWait: time.Hour,
		Log:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Now:       time.Now,
	}
	return w.Run(ctx)
}
