package demo

import (
	"cmp"
	"slices"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

func (b *builder) onBomb(event events.BombEventIf) {
	if !b.live() {
		return
	}
	e := match.BombEvent{Tick: b.tick()}
	var actor *common.Player
	var site events.Bombsite
	switch v := event.(type) {
	case events.BombPlantBegin:
		e.Kind, actor, site = match.BombPlantBegin, v.Player, v.Site
	case events.BombPlantAborted:
		e.Kind, actor = match.BombPlantAborted, v.Player
	case events.BombPlanted:
		e.Kind, actor, site = match.BombPlanted, v.Player, v.Site
	case events.BombDefuseStart:
		e.Kind, actor, e.HasKit = match.BombDefuseBegin, v.Player, new(v.HasKit)
	case events.BombDefuseAborted:
		e.Kind, actor = match.BombDefuseAborted, v.Player
	case events.BombDefused:
		e.Kind, actor, site = match.BombDefused, v.Player, v.Site
	case events.BombExplode:
		e.Kind, site = match.BombExploded, v.Site
	default:
		return
	}
	if id, ok := b.track(actor); ok {
		e.Player = id
	}
	switch site {
	case events.BombsiteA:
		e.Site = "A"
	case events.BombsiteB:
		e.Site = "B"
	}
	if e.Kind == match.BombPlanted {
		e.AliveComplete = true
		for _, pl := range b.p.GameState().Participants().Playing() {
			if !pl.IsAlive() {
				continue
			}
			id, ok := b.track(pl)
			s := side(pl.Team)
			if !ok || s == match.SideNone {
				e.AliveComplete = false
				continue
			}
			pos := pl.Position()
			e.Alive = append(e.Alive, match.PlayerState{Player: id, Side: s, Pos: match.Vec{X: pos.X, Y: pos.Y, Z: pos.Z}, Place: pl.LastPlaceName()})
		}
		if len(e.Alive) == 0 {
			e.AliveComplete = false
		}
		slices.SortFunc(e.Alive, func(a, b match.PlayerState) int { return cmp.Compare(a.Player, b.Player) })
	}
	b.cur.BombEvents = append(b.cur.BombEvents, e)
}
