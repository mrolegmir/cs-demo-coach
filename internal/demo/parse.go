// Package demo turns a CS2 demo into a match.Match. It is the only package
// that depends on the demo parser library.
package demo

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	dem "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// flashLinkWindow is how far apart a PlayerFlashed event without projectile
// info may be from the thrower's flash detonation to still be attributed to it.
const flashLinkWindow = 200 * time.Millisecond

// buyWindow is how long after freeze time players can still buy.
const buyWindow = 20 * time.Second

// Parse reads a CS2 demo. Warmup and rounds before the last match restart are
// dropped. Bots are not tracked as players.
func Parse(r io.Reader) (*match.Match, error) {
	p := dem.NewParser(r)
	defer p.Close()

	b := &builder{
		p:          p,
		players:    map[match.PlayerID]string{},
		flashes:    map[int]*match.Flash{},
		thrownFrom: map[int]string{},
		positions:  map[match.PlayerID]positionSample{},
		movement:   map[match.PlayerID][]match.MovementSample{},
	}
	b.register()

	err := p.ParseToEnd()
	if err != nil && !errors.Is(err, dem.ErrUnexpectedEndOfDemo) {
		return nil, fmt.Errorf("parse demo: %w", err)
	}

	m := &match.Match{
		MechanicsVersion: 4,
		UtilityVersion:   1,
		ObjectiveVersion: 1,
		Map:              b.mapName,
		TickRate:         p.TickRate(),
		Rounds:           b.rounds,
	}
	for id, name := range b.players {
		m.Players = append(m.Players, match.Player{ID: id, Name: name})
	}
	slices.SortFunc(m.Players, func(a, b match.Player) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	if len(m.Rounds) == 0 {
		return nil, errors.New("parse demo: no completed rounds")
	}
	return m, nil
}

type builder struct {
	positions map[match.PlayerID]positionSample
	movement  map[match.PlayerID][]match.MovementSample
	p         dem.Parser
	rounds    []match.Round
	cur       *match.Round
	mapName   string
	players   map[match.PlayerID]string
	// flashes in the current round by grenade entity ID.
	flashes map[int]*match.Flash
	// thrownFrom is where each grenade in flight was thrown from, by entity
	// ID: by the time a flash goes off, its thrower may have moved or died.
	thrownFrom map[int]string
}

func (b *builder) register() {
	b.p.RegisterNetMessageHandler(func(h *msg.CDemoFileHeader) { b.mapName = h.GetMapName() })
	b.p.RegisterEventHandler(func(events.MatchStart) { b.rounds = nil; b.cur = nil })
	b.p.RegisterEventHandler(b.onRoundStart)
	b.p.RegisterEventHandler(func(events.RoundFreezetimeEnd) { b.onFreezeEnd() })
	// Some demos only carry the game-rules flag, not the game event.
	b.p.RegisterEventHandler(func(e events.RoundFreezetimeChanged) {
		if e.OldIsFreezetime && !e.NewIsFreezetime {
			b.onFreezeEnd()
		}
	})
	b.p.RegisterEventHandler(b.onFrame)
	b.p.RegisterEventHandler(b.onKill)
	b.p.RegisterEventHandler(b.onShot)
	b.p.RegisterEventHandler(b.onDamage)
	b.p.RegisterEventHandler(b.onBomb)
	b.p.RegisterEventHandler(b.onFlashExplode)
	b.p.RegisterEventHandler(b.onPlayerFlashed)
	b.p.RegisterEventHandler(b.onGrenadeThrow)
	b.p.RegisterEventHandler(b.onRoundEnd)
}

func (b *builder) tick() int { return b.p.GameState().IngameTick() }

// live reports whether the current round is past freeze time.
func (b *builder) live() bool { return b.cur != nil && b.cur.FreezeEndTick > 0 }

func (b *builder) onRoundStart(events.RoundStart) {
	if b.p.GameState().IsWarmupPeriod() {
		b.cur = nil
		return
	}
	b.cur = &match.Round{Number: len(b.rounds) + 1, StartTick: b.tick()}
	clear(b.flashes)
	clear(b.thrownFrom)
	clear(b.positions)
	clear(b.movement)
}

func (b *builder) onFreezeEnd() {
	if b.cur == nil || b.cur.FreezeEndTick > 0 {
		return
	}
	b.cur.FreezeEndTick = b.tick()
}

// onFrame takes the buy snapshot once the buy window has closed.
func (b *builder) onFrame(events.FrameDone) {
	if b.live() && b.cur.Players == nil && b.tick()-b.cur.FreezeEndTick >= int(buyWindow.Seconds()*b.p.TickRate()) {
		b.snapshotBuys()
	}
	if b.live() {
		for _, pl := range b.p.GameState().Participants().Playing() {
			if !pl.IsAlive() {
				delete(b.positions, playerID(pl))
				delete(b.movement, playerID(pl))
				continue
			}
			pos := pl.Position()
			b.recordMovement(playerID(pl), positionSample{tick: b.tick(), pos: match.Vec{X: pos.X, Y: pos.Y, Z: pos.Z}}, pl.IsAirborne(), b.p.TickRate(), captureView(pl))
		}
	}
}

type positionSample struct {
	tick int
	pos  match.Vec
}

func (b *builder) onDamage(e events.PlayerHurt) {
	if !b.live() || e.Weapon == nil || e.Attacker == nil || e.Player == nil {
		return
	}
	weapon := e.Weapon.Type
	if weapon == common.EqHE || weapon == common.EqMolotov || weapon == common.EqIncendiary {
		b.onUtilityDamage(e)
		return
	}
	switch weapon.Class() {
	case common.EqClassPistols, common.EqClassSMG, common.EqClassHeavy, common.EqClassRifle:
	default:
		return
	}
	attacker, ok := b.track(e.Attacker)
	if !ok {
		return
	}
	victim, ok := b.track(e.Player)
	if !ok {
		return
	}
	a, v := e.Attacker.Position(), e.Player.Position()
	var preparation []match.ContactSample
	if attacker != victim && e.Attacker.Team != e.Player.Team && e.HealthDamageTaken > 0 {
		preparation = contactBefore(b.movement[attacker], b.movement[victim], b.tick(), b.p.TickRate())
	}
	b.cur.Damage = append(b.cur.Damage, match.Damage{
		Tick: b.tick(), Attacker: attacker, Victim: victim, Weapon: weapon.String(),
		HealthDamage: e.HealthDamageTaken, HitGroup: int(e.HitGroup),
		Distance: a.Sub(v).Norm(), Friendly: e.Attacker.Team == e.Player.Team,
		Preparation: preparation,
	})
}

func (b *builder) onShot(e events.WeaponFire) {
	if !b.live() || e.Weapon == nil {
		return
	}
	id, ok := b.track(e.Shooter)
	if !ok || !e.Shooter.IsAlive() {
		return
	}
	// Capture firearms only; grenade/knife events are not shots for this pilot.
	switch e.Weapon.Class() {
	case common.EqClassPistols, common.EqClassSMG, common.EqClassHeavy, common.EqClassRifle:
	default:
		return
	}
	pos := e.Shooter.Position()
	s := match.Shot{Tick: b.tick(), Player: id, Weapon: e.Weapon.String(), Pos: match.Vec{X: pos.X, Y: pos.Y, Z: pos.Z}, Yaw: float64(e.Shooter.ViewDirectionX()), Pitch: float64(e.Shooter.ViewDirectionY()), Airborne: e.Shooter.IsAirborne(), Place: e.Shooter.LastPlaceName()}
	s.View = captureView(e.Shooter)
	if prev, exists := b.positions[id]; exists {
		s.Speed = match.ShotSpeed(prev.pos, s.Pos, s.Tick-prev.tick, b.p.TickRate())
		if s.Speed != nil {
			s.SpeedWindowTicks = s.Tick - prev.tick
		}
	}
	first := match.ReviewRifle(s.Weapon)
	if first {
		for i := len(b.cur.Shots) - 1; i >= 0; i-- {
			if prev := b.cur.Shots[i]; prev.Player == id {
				first = !match.SameRifleSequence(prev, s, b.p.TickRate())
				break
			}
		}
	}
	if first {
		s.Preparation = movementBefore(b.movement[id], s, b.p.TickRate())
	}
	b.cur.Shots = append(b.cur.Shots, s)
}

// snapshotBuys records what every player bought this round. It runs when the
// buy window closes or right before the first kill, whichever comes first, so
// the snapshot sees full inventories.
func (b *builder) snapshotBuys() {
	b.cur.Players = []match.PlayerRound{}
	for _, pl := range b.p.GameState().Participants().Playing() {
		id, ok := b.track(pl)
		if !ok {
			continue
		}
		b.cur.Players = append(b.cur.Players, match.PlayerRound{
			Player:         id,
			Side:           side(pl.Team),
			Money:          pl.Money(),
			Spent:          pl.MoneySpentThisRound(),
			EquipmentValue: pl.EquipmentValueCurrent(),
		})
	}
	slices.SortFunc(b.cur.Players, func(a, b match.PlayerRound) int { return cmp.Compare(a.Player, b.Player) })
}

func (b *builder) onKill(e events.Kill) {
	if !b.live() || e.Victim == nil {
		return
	}
	if b.cur.Players == nil {
		b.snapshotBuys()
	}
	k := match.Kill{
		Tick:     b.tick(),
		Victim:   playerID(e.Victim),
		Killer:   playerID(e.Killer),
		Assister: playerID(e.Assister),
		Headshot: e.IsHeadshot,
		Wallbang: e.IsWallBang(),
	}
	if e.Weapon != nil {
		k.Weapon = e.Weapon.String()
	}
	if e.Killer != nil && e.Killer != e.Victim && e.Killer.Team == e.Victim.Team {
		k.TeamKill = true
	}
	for _, w := range e.Victim.Weapons() {
		if w != nil && w.Class() == common.EqClassGrenade {
			k.VictimUtility = append(k.VictimUtility, w.String())
		}
	}
	for _, pl := range b.p.GameState().Participants().Playing() {
		if pl != e.Victim && !pl.IsAlive() {
			continue
		}
		pos := pl.Position()
		k.Alive = append(k.Alive, match.PlayerState{
			Player: playerID(pl),
			Side:   side(pl.Team),
			Pos:    match.Vec{X: pos.X, Y: pos.Y, Z: pos.Z},
			Place:  pl.LastPlaceName(),
		})
	}
	slices.Sort(k.VictimUtility) // inventory is a map
	slices.SortFunc(k.Alive, func(a, b match.PlayerState) int { return cmp.Compare(a.Player, b.Player) })
	b.cur.Kills = append(b.cur.Kills, k)
}

func (b *builder) flash(entityID int) *match.Flash {
	f, ok := b.flashes[entityID]
	if !ok {
		f = &match.Flash{Tick: b.tick()}
		b.flashes[entityID] = f
	}
	return f
}

func (b *builder) onFlashExplode(e events.FlashExplode) {
	if !b.live() || e.Thrower == nil {
		return
	}
	f := b.flash(e.GrenadeEntityID)
	f.Tick = b.tick()
	f.Thrower = playerID(e.Thrower)
	f.Side = side(e.Thrower.Team)
	f.Pos = match.Vec{X: e.Position.X, Y: e.Position.Y, Z: e.Position.Z}
	f.Place = b.thrownFrom[e.GrenadeEntityID]
}

func (b *builder) onPlayerFlashed(e events.PlayerFlashed) {
	if !b.live() || e.Player == nil || e.Attacker == nil {
		return
	}
	var f *match.Flash
	if e.Projectile != nil && e.Projectile.Entity != nil {
		f = b.flash(e.Projectile.Entity.ID())
	} else {
		f = b.recentFlashBy(playerID(e.Attacker))
	}
	if f == nil {
		return
	}
	if f.Thrower == 0 {
		f.Thrower = playerID(e.Attacker)
		f.Side = side(e.Attacker.Team)
	}
	f.Blinded = append(f.Blinded, match.Blinding{
		Player:   playerID(e.Player),
		Side:     side(e.Player.Team),
		Duration: e.Player.FlashDurationTime(),
	})
}

// recentFlashBy returns the thrower's latest flash if it detonated within
// flashLinkWindow.
func (b *builder) recentFlashBy(thrower match.PlayerID) *match.Flash {
	window := int(flashLinkWindow.Seconds() * b.p.TickRate())
	var latest *match.Flash
	for _, f := range b.flashes {
		if f.Thrower == thrower && b.tick()-f.Tick <= window && (latest == nil || f.Tick > latest.Tick) {
			latest = f
		}
	}
	return latest
}

func (b *builder) onGrenadeThrow(e events.GrenadeProjectileThrow) {
	if !b.live() || e.Projectile == nil || e.Projectile.Thrower == nil || e.Projectile.WeaponInstance == nil {
		return
	}
	if e.Projectile.Entity != nil {
		b.thrownFrom[e.Projectile.Entity.ID()] = e.Projectile.Thrower.LastPlaceName()
	}
	b.cur.Grenades = append(b.cur.Grenades, match.Grenade{
		Tick:    b.tick(),
		Thrower: playerID(e.Projectile.Thrower),
		Type:    e.Projectile.WeaponInstance.String(),
	})
}

func (b *builder) onRoundEnd(e events.RoundEnd) {
	if !b.live() {
		return
	}
	r := b.cur
	if r.Players == nil {
		b.snapshotBuys()
	}
	r.EndTick = b.tick()
	r.Winner = side(e.Winner)
	r.EndReason = endReason(e.Reason)
	for _, f := range b.flashes {
		if f.Thrower != 0 {
			r.Flashes = append(r.Flashes, *f)
		}
	}
	slices.SortFunc(r.Flashes, func(a, b match.Flash) int {
		return cmp.Or(cmp.Compare(a.Tick, b.Tick), cmp.Compare(a.Thrower, b.Thrower))
	})
	b.rounds = append(b.rounds, *r)
	b.cur = nil
}

// track records a human player and returns its ID; bots are skipped.
func (b *builder) track(pl *common.Player) (match.PlayerID, bool) {
	if pl == nil || pl.IsBot || pl.SteamID64 == 0 {
		return 0, false
	}
	id := match.PlayerID(pl.SteamID64)
	b.players[id] = pl.Name
	return id, true
}

func playerID(pl *common.Player) match.PlayerID {
	if pl == nil || pl.IsBot {
		return 0
	}
	return match.PlayerID(pl.SteamID64)
}

func side(t common.Team) match.Side {
	switch t {
	case common.TeamTerrorists:
		return match.SideT
	case common.TeamCounterTerrorists:
		return match.SideCT
	default:
		return match.SideNone
	}
}

func endReason(r events.RoundEndReason) string {
	switch r {
	case events.RoundEndReasonTargetBombed:
		return "bomb_exploded"
	case events.RoundEndReasonBombDefused:
		return "bomb_defused"
	case events.RoundEndReasonCTWin:
		return "ct_eliminated_t"
	case events.RoundEndReasonTerroristsWin:
		return "t_eliminated_ct"
	case events.RoundEndReasonTargetSaved:
		return "time_ran_out"
	default:
		return fmt.Sprintf("reason_%d", r)
	}
}
