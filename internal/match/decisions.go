package match

// AdvantageReview is a lost round in which a complete death snapshot records
// the viewer alive with a numerical advantage for their team. It is review
// context, not a finding about why the round was lost.
type AdvantageReview struct {
	Round, Tick          int
	Side                 Side
	OwnAlive, EnemyAlive int
	EndReason            string
	Alive                []PlayerState
	After                []Kill
}

func (m *Match) LostAdvantages(player PlayerID) []AdvantageReview {
	var out []AdvantageReview
	if player == 0 {
		return out
	}
	for _, r := range m.Rounds {
		side := r.Side(player)
		if (side != SideT && side != SideCT) || (r.Winner != SideT && r.Winner != SideCT) || r.Winner == side {
			continue
		}
		tickCounts := map[int]int{}
		for _, k := range r.Kills {
			tickCounts[k.Tick]++
		}
		var best *AdvantageReview
		for _, k := range r.Kills {
			// Same-tick multi-kills cannot establish a stable post-kill count.
			if tickCounts[k.Tick] != 1 || k.Tick < r.FreezeEndTick || k.Tick > r.EndTick || k.Victim == 0 || k.Victim == player {
				continue
			}
			seen := map[PlayerID]bool{}
			own, enemy, victimCount := 0, 0, 0
			viewerAlive, valid := false, true
			for _, a := range k.Alive {
				if a.Player == 0 || seen[a.Player] || (a.Side != SideT && a.Side != SideCT) {
					valid = false
					break
				}
				seen[a.Player] = true
				if a.Player == k.Victim {
					victimCount++
					continue
				}
				if a.Player == player {
					viewerAlive = a.Side == side
				}
				if a.Side == side {
					own++
				} else {
					enemy++
				}
			}
			if !valid || victimCount != 1 || !viewerAlive || enemy == 0 || own <= enemy {
				continue
			}
			if best == nil || own-enemy > best.OwnAlive-best.EnemyAlive {
				best = &AdvantageReview{Round: r.Number, Tick: k.Tick, Side: side, OwnAlive: own, EnemyAlive: enemy, EndReason: r.EndReason}
				for _, a := range k.Alive {
					if a.Player != k.Victim {
						best.Alive = append(best.Alive, a)
					}
				}
			}
		}
		if best != nil {
			for _, k := range r.Kills {
				if k.Tick > best.Tick && k.Tick <= r.EndTick && k.Victim != 0 {
					best.After = append(best.After, k)
				}
			}
			out = append(out, *best)
		}
	}
	return out
}
