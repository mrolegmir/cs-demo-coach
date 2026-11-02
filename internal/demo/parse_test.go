package demo

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

var update = flag.Bool("update", false, "rewrite testdata/matches fixtures from the demos")

// fixtures are demos, mostly from the demoinfocs test set
// (https://gitlab.com/markus-wa/cs-demos-2, s2.7z). The demos are too large to
// commit, so the parsed, anonymized matches are committed instead and the
// test compares against them when the demo is available locally in demos/.
var fixtures = []string{
	"cs2_valve_mm_8thAug23_ancient",
	"Anubis_ShortMatch_2023-08-04",
	"arsham_infernocs2_1stSept23",
	// A 2026 Valve matchmaking demo in the current format, kept out of git.
	"mm_2026_dust2",
}

func TestParseMatchesFixtures(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			demoPath := filepath.Join("..", "..", "demos", name+".dem")
			fixturePath := filepath.Join("..", "..", "testdata", "matches", name+".json.gz")

			f, err := os.Open(demoPath)
			if errors.Is(err, fs.ErrNotExist) {
				t.Skipf("demo not available: %s", demoPath)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			got, err := Parse(f)
			if err != nil {
				t.Fatal(err)
			}
			got.ID = name
			got.Anonymize()
			if got.MechanicsVersion != 4 {
				t.Fatal("missing mechanics capture version")
			}
			if got.UtilityVersion != 1 {
				t.Fatal("missing utility capture")
			}
			if got.ObjectiveVersion != 1 {
				t.Fatal("missing objective capture")
			}
			bombEvents, plants := 0, 0
			for _, r := range got.Rounds {
				for _, e := range r.BombEvents {
					bombEvents++
					if e.Tick < r.FreezeEndTick || e.Tick > r.EndTick || e.Player > match.PlayerID(len(got.Players)) || (e.Site != "" && e.Site != "A" && e.Site != "B") {
						t.Fatal("invalid bomb event")
					}
					if e.Kind == match.BombPlanted && e.AliveComplete {
						plants++
					}
					if e.Kind == match.BombDefuseBegin && e.HasKit == nil {
						t.Fatal("missing kit state")
					}
					for _, a := range e.Alive {
						if a.Player == 0 || a.Player > match.PlayerID(len(got.Players)) {
							t.Fatal("plant snapshot not anonymized")
						}
					}
				}
			}
			if bombEvents == 0 || plants == 0 {
				t.Fatal("fixture lost objective events/snapshots")
			}
			t.Logf("bomb events=%d complete plants=%d", bombEvents, plants)
			var utilityEvents, utilityKnown int
			for _, r := range got.Rounds {
				for _, d := range r.UtilityDamage {
					utilityEvents++
					if d.Attacker == 0 || d.Victim == 0 || d.Attacker > match.PlayerID(len(got.Players)) || d.Victim > match.PlayerID(len(got.Players)) || match.UtilityFamily(d.Weapon) == "" {
						t.Fatal("invalid utility event or identities not anonymized")
					}
					if d.HealthDamage != nil {
						utilityKnown++
					}
				}
			}
			t.Logf("utility events=%d known=%d", utilityEvents, utilityKnown)
			if (name == "mm_2026_dust2" || name == "arsham_infernocs2_1stSept23") && (utilityEvents == 0 || utilityKnown == 0) {
				t.Fatal("supported fixture lost utility damage capture")
			}
			var shots, measured, moving, damage, linked int
			for _, r := range got.Rounds {
				damage += len(r.Damage)
				for _, d := range r.Damage {
					for _, s := range d.Preparation {
						if s.Tick >= d.Tick || float64(d.Tick-s.Tick)/got.TickRate > 0.5 {
							t.Fatal("future/stale contact sample")
						}
						if s.View == nil || s.View.Eye == nil || s.TargetEye == nil {
							t.Fatal("supported fixture lost paired eye fields")
						}
					}
				}
				for di, si := range r.DamageLinks() {
					d := r.Damage[di]
					if d.Attacker == 0 || d.Victim == 0 || d.Attacker > match.PlayerID(len(got.Players)) || d.Victim > match.PlayerID(len(got.Players)) {
						t.Fatal("damage player not anonymized")
					}
					if si >= 0 {
						linked++
					}
				}
				for _, s := range r.Shots {
					if s.View == nil || s.View.Eye == nil {
						t.Fatal("supported fixture lost shot view/eye fields")
					}
					if len(s.Preparation) > 0 && s.Preparation[len(s.Preparation)-1].Tick != s.Tick {
						t.Fatal("missing shot-time movement sample")
					}
					for i, sample := range s.Preparation {
						if sample.Tick > s.Tick || float64(s.Tick-sample.Tick)/got.TickRate > 0.5 || (i > 0 && sample.Tick <= s.Preparation[i-1].Tick) {
							t.Fatal("invalid movement history window/order")
						}
						if sample.Speed != nil && (sample.WindowTicks <= 0 || float64(sample.WindowTicks)/got.TickRate > 0.1) {
							t.Fatal("speed interval not recorded or too long")
						}
					}
					shots++
					if s.Player == 0 || s.Player > match.PlayerID(len(got.Players)) {
						t.Fatal("shot player not anonymized")
					}
					if s.Speed != nil {
						measured++
						if *s.Speed > 80 {
							moving++
						}
					}
				}
			}
			if shots == 0 || measured == 0 {
				t.Fatalf("no usable shot capture: shots=%d measured=%d", shots, measured)
			}
			for _, seq := range got.ShootingSequences() {
				if len(seq.Preparation) == 0 {
					t.Fatal("first-shot preparation missing")
				}
			}
			t.Logf("shots=%d speed_samples=%d above_80=%d rifle_sequences=%d", shots, measured, moving, len(got.ShootingSequences()))
			if (name == "mm_2026_dust2" || name == "arsham_infernocs2_1stSept23") && (damage == 0 || linked == 0) {
				t.Fatalf("no damage attribution: events=%d linked=%d", damage, linked)
			}
			t.Logf("firearm_damage_events=%d uniquely_linked_enemy_events=%d", damage, linked)

			if *update {
				if err := match.SaveFile(fixturePath, got); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := match.LoadFile(fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			// Compare as JSON: the fixture round-trip turns empty slices into nil.
			if !bytes.Equal(mustJSON(t, got), mustJSON(t, want)) {
				t.Errorf("parsed match differs from %s; run with -update if the change is intended", fixturePath)
			}
		})
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	_, err := Parse(strings.NewReader("not a demo"))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
