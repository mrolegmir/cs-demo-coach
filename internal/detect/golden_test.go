package detect

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestGolden runs all default detectors on the committed match fixtures and
// compares the report with testdata/<match>.golden.json. Review the golden
// diff whenever a detector changes: it shows exactly which findings appeared
// or disappeared.
func TestGolden(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "matches", "*.json.gz"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no match fixtures found (err: %v)", err)
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".json.gz")
		t.Run(name, func(t *testing.T) {
			m, err := match.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(Run(m, Default()), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", name+".golden.json")
			if *update {
				if err := os.WriteFile(golden, append(got, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
				t.Errorf("report differs from %s; run with -update and review the diff", golden)
			}
		})
	}
}

// TestPairsAddUp checks pairs against the trade stats: every chance to
// trade belongs to one pair, and every untraded one is a missed trade.
func TestPairsAddUp(t *testing.T) {
	paths, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "matches", "*.json.gz"))
	for _, path := range paths {
		m, err := match.LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rep := Run(m, Default())
		var chances, missed, pairChances, untraded int
		for _, s := range rep.Stats {
			if s.Kind == KindMissedTrade {
				chances += s.Chances
				missed += s.Count
			}
		}
		for _, p := range rep.Pairs {
			if p.Player == p.Partner || m.StartSide(p.Player) != m.StartSide(p.Partner) {
				t.Errorf("%s: pair %+v is not two teammates", path, p)
			}
			pairChances += p.Chances
			untraded += p.Chances - p.Traded
		}
		if pairChances != chances || untraded != missed || len(rep.Pairs) == 0 {
			t.Errorf("%s: pairs have %d chances, %d untraded; trade stats %d, %d", path, pairChances, untraded, chances, missed)
		}
		if got := TradePairs(m); len(got) != len(rep.Pairs) {
			t.Errorf("%s: TradePairs = %d pairs, Run = %d", path, len(got), len(rep.Pairs))
		}
	}
}
