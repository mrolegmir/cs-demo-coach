package coach

import (
	"fmt"
	"maps"
	"slices"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Identify stamps stable IDs from validated input, never from model assertions.
// Legacy inputs without an identity map retain their existing representation.
func (in *Input) Identify(a *Advice) {
	if in.PlayerIDs == nil {
		return
	}
	for i := range a.Players {
		a.Players[i].PlayerID = in.PlayerIDs[a.Players[i].Player]
	}
}

// PlayerLabels gives the model unambiguous human-readable references. Labels
// are scoped to one input; PlayerIDs retains the stable identity behind them.
// Reserve every original name so a nickname resembling a suffix cannot collide.
func PlayerLabels(names map[match.PlayerID]string) map[match.PlayerID]string {
	counts := map[string]int{}
	used := map[string]bool{}
	for _, name := range names {
		counts[name]++
		used[name] = true
	}
	out := map[match.PlayerID]string{}
	for ordinal, id := range slices.Sorted(maps.Keys(names)) {
		label := names[id]
		if label == "" || counts[label] > 1 {
			label = fmt.Sprintf("%s [player %d]", names[id], ordinal+1)
			for used[label] {
				label += "*"
			}
			used[label] = true
		}
		out[id] = label
	}
	return out
}

// ResolvePlayer reads new advice by ID. Legacy advice is attributed only when
// its name identifies exactly one participant and that participant is in party.
func ResolvePlayer(pa PlayerAdvice, players []match.Player, party []match.PlayerID) match.PlayerID {
	if pa.PlayerID != 0 {
		if slices.Contains(party, pa.PlayerID) && slices.ContainsFunc(players, func(p match.Player) bool { return p.ID == pa.PlayerID }) {
			return pa.PlayerID
		}
		return 0
	}
	var id match.PlayerID
	for _, p := range players {
		if p.Name == pa.Player {
			if id != 0 {
				return 0
			}
			id = p.ID
		}
	}
	if slices.Contains(party, id) {
		return id
	}
	return 0
}
