package match

import (
	"math"
	"time"
)

const (
	FlashUnique       = "unique"
	FlashAmbiguous    = "ambiguous"
	FlashUnclassified = "unclassified"
)

type FlashReview struct {
	Round int
	Flash
	EnemyTargets, UnclassifiedBlindings int
}

// FlashSource identifies a round-local stored flash, including unknown throwers.
type FlashSource struct {
	Index, Tick int
	Thrower     PlayerID
	Side        Side
}

type FlashKill struct {
	Round, Tick    int
	Killer, Victim PlayerID
	Sources        []FlashSource
	State          string
	Delay          time.Duration // Only available for a unique, eligible match.
}

type FlashSupport struct {
	TimingAvailable                                                  bool
	Flashes                                                          []FlashReview
	Kills                                                            []FlashKill
	TeammateKills, SupportedKills, AmbiguousKills, UnclassifiedKills int
}

// FlashSupportFor describes temporal overlap, not causality or official flash
// assists. Every possible positive blind interval competes for attribution,
// including enemy flashes and flashes whose thrower or side is unknown.
func (m *Match) FlashSupportFor(player PlayerID) FlashSupport {
	rep := FlashSupport{TimingAvailable: m.TickRate > 0 && !math.IsInf(m.TickRate, 0)}
	if player == 0 || m.StartSide(player) == SideNone {
		return rep
	}
	for _, r := range m.Rounds {
		for _, f := range r.Flashes {
			if f.Thrower != player {
				continue
			}
			v := FlashReview{Round: r.Number, Flash: f}
			enemies := map[PlayerID]bool{}
			for _, b := range f.Blinded {
				if !playingSide(f.Side) || !playingSide(b.Side) || b.Player == 0 || b.Duration < 0 || (b.Player == f.Thrower && b.Side != f.Side) {
					v.UnclassifiedBlindings++
					continue
				}
				if b.Duration > 0 && b.Side != f.Side {
					enemies[b.Player] = true
				}
			}
			v.EnemyTargets = len(enemies)
			rep.Flashes = append(rep.Flashes, v)
		}
		if !rep.TimingAvailable {
			continue
		}
		for _, k := range r.Kills {
			if k.Killer == 0 || k.Victim == 0 || k.Killer == k.Victim || k.TeamKill || k.Tick < r.FreezeEndTick || k.Tick > r.EndTick {
				continue
			}
			link := FlashKill{Round: r.Number, Tick: k.Tick, Killer: k.Killer, Victim: k.Victim, State: FlashUnclassified}
			relevant := k.Killer == player
			for fi, f := range r.Flashes {
				if f.Tick < r.FreezeEndTick || f.Tick > k.Tick {
					continue
				}
				elapsed := float64(k.Tick-f.Tick) / m.TickRate
				for _, b := range f.Blinded {
					if b.Player == k.Victim && b.Duration > 0 && elapsed < b.Duration.Seconds() {
						link.Sources = append(link.Sources, FlashSource{Index: fi, Tick: f.Tick, Thrower: f.Thrower, Side: f.Side})
						relevant = relevant || f.Thrower == player
						break // Duplicate victim records do not create a second source.
					}
				}
			}
			if !relevant || len(link.Sources) == 0 {
				continue
			}
			if len(link.Sources) > 1 {
				link.State = FlashAmbiguous
				rep.AmbiguousKills++
			} else {
				s := link.Sources[0]
				killerSide, victimSide, known := flashKillSides(k)
				f := r.Flashes[s.Index]
				elapsed := float64(k.Tick-f.Tick) / m.TickRate
				eligible := known && s.Thrower != 0 && s.Thrower != k.Victim && s.Side == killerSide && k.Tick > f.Tick
				for _, b := range f.Blinded {
					if b.Player == k.Victim && b.Duration > 0 && elapsed < b.Duration.Seconds() && b.Side != victimSide {
						eligible = false
					}
				}
				if eligible {
					link.State = FlashUnique
					link.Delay = time.Duration(elapsed * float64(time.Second))
					if s.Thrower == player && k.Killer != player {
						rep.TeammateKills++
					}
					if k.Killer == player && s.Thrower != player {
						rep.SupportedKills++
					}
				} else {
					rep.UnclassifiedKills++
				}
			}
			rep.Kills = append(rep.Kills, link)
		}
	}
	return rep
}

func playingSide(s Side) bool { return s == SideT || s == SideCT }

func flashKillSides(k Kill) (Side, Side, bool) {
	var killer, victim Side
	killers, victims := 0, 0
	for _, a := range k.Alive {
		if a.Player == k.Killer {
			killer = a.Side
			killers++
		}
		if a.Player == k.Victim {
			victim = a.Side
			victims++
		}
	}
	return killer, victim, killers == 1 && victims == 1 && playingSide(killer) && playingSide(victim) && killer != victim
}
