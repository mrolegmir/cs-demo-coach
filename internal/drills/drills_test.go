package drills

import (
	"slices"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
)

var allKinds = []detect.Kind{
	detect.KindIsolatedDeath, detect.KindMissedTrade, detect.KindFlashNoEnemy, detect.KindTeamFlash,
	detect.KindUnusedUtility, detect.KindBuyDesync, detect.KindMissedDrop, detect.KindNoUtility,
	detect.KindOpeningDeath,
}

func TestLibrary(t *testing.T) {
	ids := map[string]bool{}
	wheres := []string{"offline server", "deathmatch", "your matches", "with your party"}
	for _, d := range All() {
		switch {
		case d.ID == "" || ids[d.ID]:
			t.Errorf("drill %q: missing or duplicate ID", d.ID)
		case !slices.Contains(allKinds, d.Kind):
			t.Errorf("drill %q: unknown kind %q", d.ID, d.Kind)
		case d.Title == "" || d.Check == "" || len(d.Steps) == 0:
			t.Errorf("drill %q: title, steps and check are required", d.ID)
		case !slices.Contains(wheres, d.Where):
			t.Errorf("drill %q: where = %q", d.ID, d.Where)
		case d.Where == "offline server" && d.Minutes == 0:
			t.Errorf("drill %q: an offline drill needs a time", d.ID)
		}
		ids[d.ID] = true
	}
	// The coach must always have an alternative when a drill is not working.
	for _, k := range allKinds {
		if n := len(ForKind(k)); n < 2 {
			t.Errorf("kind %q has %d drills, want at least 2", k, n)
		}
	}
	if d, ok := ByID("two-pop-flashes"); !ok || d.Kind != detect.KindFlashNoEnemy {
		t.Errorf("ByID = %+v, %v", d, ok)
	}
	if _, ok := ByID("nope"); ok {
		t.Error("unknown ID found")
	}
}
