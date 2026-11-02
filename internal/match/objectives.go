package match

// BombEvent preserves observed actions. A missing player/site/kit/snapshot is
// unknown, never replaced by a guessed planter, bombsite or default timer.
type BombEvent struct {
	Tick          int           `json:"tick"`
	Kind          string        `json:"kind"`
	Player        PlayerID      `json:"player,omitzero"`
	Site          string        `json:"site,omitempty"`
	HasKit        *bool         `json:"has_kit,omitzero"`
	Alive         []PlayerState `json:"alive,omitempty"`
	AliveComplete bool          `json:"alive_complete,omitzero"`
}

const (
	BombPlantBegin    = "plant_begin"
	BombPlantAborted  = "plant_aborted"
	BombPlanted       = "planted"
	BombDefuseBegin   = "defuse_begin"
	BombDefuseAborted = "defuse_aborted"
	BombDefused       = "defused"
	BombExploded      = "exploded"
)

type ObjectiveRound struct {
	Round        int
	Side, Winner Side
	EndReason    string
	Events       []BombEvent
	// Plant is set only for exactly one recorded completed plant.
	Plant  *BombEvent
	Deaths []Kill
}

func (m *Match) ObjectiveRounds(player PlayerID) []ObjectiveRound {
	var out []ObjectiveRound
	if m.ObjectiveVersion != 1 || player == 0 {
		return out
	}
	for _, r := range m.Rounds {
		side := r.Side(player)
		if (side != SideT && side != SideCT) || len(r.BombEvents) == 0 {
			continue
		}
		v := ObjectiveRound{Round: r.Number, Side: side, Winner: r.Winner, EndReason: r.EndReason, Events: r.BombEvents}
		plants := 0
		for _, e := range r.BombEvents {
			if e.Kind == BombPlanted {
				plants++
				v.Plant = new(e)
			}
		}
		if plants != 1 {
			v.Plant = nil
		} else {
			for _, k := range r.Kills {
				if k.Victim != 0 && k.Tick >= v.Plant.Tick && k.Tick <= r.EndTick {
					v.Deaths = append(v.Deaths, k)
				}
			}
		}
		out = append(out, v)
	}
	return out
}
