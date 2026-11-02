// Package drills is the curated library of practice drills the coach picks
// from (ADR 14). Each drill targets one kind of mistake, takes a known time,
// and says how players can tell they did it right, so advice never rests on
// a drill a model made up.
package drills

import (
	"slices"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
)

// Drill is one way to practice against one kind of mistake.
type Drill struct {
	ID    string      `json:"id"`
	Kind  detect.Kind `json:"kind"`
	Title string      `json:"title"`
	// Where is where it is done: "offline server", "deathmatch", "your
	// matches" or "with your party".
	Where   string   `json:"where"`
	Minutes int      `json:"minutes"`
	Steps   []string `json:"steps"`
	// Check is how players know they did it right.
	Check string `json:"check"`
}

// utilityGuide is how players find lineups: community utility guides in the
// Workshop are drawn on the real map in practice mode, so nobody needs a
// video or a separate map.
const utilityGuide = "Subscribe to a utility guide for your map in the Workshop: it is drawn on the real map, not a separate one. " +
	"Start a practice match on that map and pick the guide in the menu on the left"

// Practice server setup shared by the offline drills. All commands exist in
// CS2; sv_cheats is needed for the grenade ones.
const practiceServer = "Start a practice match with bots on your map, open the console and run: " +
	"sv_cheats 1; mp_roundtime_defuse 60; mp_freezetime 0; mp_buy_anywhere 1; " +
	"ammo_grenade_limit_total 5; sv_infinite_ammo 1; sv_grenade_trajectory_prac_pipreview 1; bot_kick"

var library = []Drill{
	{
		ID: "minimap-before-fight", Kind: detect.KindIsolatedDeath, Title: "Check the minimap before every fight",
		Where: "your matches", Minutes: 0,
		Steps: []string{
			"Before you peek or hold a contested angle, glance at the minimap.",
			"If no teammate is within a few seconds of you, fall back a step or wait for one.",
			"Say where you are going before you go (\"I'm going B apps alone\"), so a teammate can follow or tell you not to.",
		},
		Check: "In your next matches, most of your deaths come with a teammate close enough to trade.",
	},
	{
		ID: "play-in-pairs", Kind: detect.KindIsolatedDeath, Title: "Play every round in a pair",
		Where: "with your party", Minutes: 0,
		Steps: []string{
			"At the start of each round, pick the teammate you play with (\"I'm with you, Anna\").",
			"Stay within a few seconds of each other: one peeks, the other swings right after.",
			"Swap pairs at half time so everyone learns it.",
		},
		Check: "After a few rounds, you can say where your pair is without looking.",
	},
	{
		ID: "refrag-spacing", Kind: detect.KindMissedTrade, Title: "Refrag spacing",
		Where: "with your party", Minutes: 15,
		Steps: []string{
			"In a private lobby or Wingman with a friend, the friend peeks an angle with bots, you follow two or three steps behind.",
			"When they die, swing immediately at the same angle, crosshair at head height on the spot the kill came from.",
			"Swap roles every five tries.",
		},
		Check: "You are swinging within a second of the death sound, without stopping to look first.",
	},
	{
		ID: "swing-on-death-sound", Kind: detect.KindMissedTrade, Title: "Swing on the death sound",
		Where: "your matches", Minutes: 0,
		Steps: []string{
			"When you play behind an entry, keep your crosshair on the angle they are fighting.",
			"The moment you hear them die, swing that angle; the killer is often reloading or repositioning.",
			"If you were too far to trade, note it and stand closer next round.",
		},
		Check: "In your next matches, the enemy who kills your teammate dies within a few seconds more often.",
	},
	{
		ID: "two-pop-flashes", Kind: detect.KindFlashNoEnemy, Title: "Learn two pop-flashes for your site",
		Where: "offline server", Minutes: 20,
		Steps: []string{
			practiceServer + ".",
			utilityGuide + ".",
			"Pick the site you play most, or the place your coach names, and one flash for each of its two main entrances from the guide.",
			"Throw each one until it pops above the angle with no time for the enemy to turn away (sv_rethrow_last_grenade repeats the last throw).",
			"Use them in your next matches only right before you or a teammate peeks.",
		},
		Check: "You can throw each flash five times in a row and it pops where you want it.",
	},
	{
		ID: "flash-then-peek", Kind: detect.KindFlashNoEnemy, Title: "Flash, then peek at once",
		Where: "your matches", Minutes: 0,
		Steps: []string{
			"Only throw a flash when someone is ready to peek right behind it.",
			"Say it first (\"flashing, go\") so the peek and the flash land together.",
			"If nobody is going to peek, keep the flash.",
		},
		Check: "In your next matches, your flashes blind an enemy more often than the lobby's do.",
	},
	{
		ID: "call-your-flashes", Kind: detect.KindTeamFlash, Title: "Call every flash",
		Where: "with your party", Minutes: 0,
		Steps: []string{
			"Before throwing, say where (\"flashing A main\").",
			"Teammates in front turn away for a second; you throw only after they can.",
			"Agree on the words once, so nobody has to think about them mid-round.",
		},
		Check: "A round goes by without anyone on your team asking who flashed them.",
	},
	{
		ID: "flash-over-teammates", Kind: detect.KindTeamFlash, Title: "Flashes that pop behind your team",
		Where: "offline server", Minutes: 15,
		Steps: []string{
			practiceServer + ".",
			utilityGuide + ": look for flashes thrown from behind your team.",
			"Stand where your teammates usually are when you flash for them, and learn one flash per site that pops past them, above the enemy's angle.",
			"Check where it bursts with the trajectory preview: behind the teammate's back, not in front of their face.",
		},
		Check: "Standing where your teammate would, you are not blinded by your own flash.",
	},
	{
		ID: "one-grenade-a-round", Kind: detect.KindUnusedUtility, Title: "One grenade a round, decided in freeze time",
		Where: "your matches", Minutes: 0,
		Steps: []string{
			"During freeze time, decide which grenade you will use first and where.",
			"Throw it before your first fight, not after.",
			"If you are about to take a risky fight with grenades left, throw one first: a molotov to clear a corner, a flash to peek.",
		},
		Check: "In your next matches, you die with grenades left less often than the lobby.",
	},
	{
		ID: "utility-before-you-die", Kind: detect.KindUnusedUtility, Title: "Empty your pockets when the round is lost",
		Where: "your matches", Minutes: 0,
		Steps: []string{
			"When you are low on health or the round is going badly, throw what you have at the enemy's path: a molotov on the bomb, an HE at a stacked spot.",
			"When you are last alive with grenades, use them to delay before taking the fight.",
		},
		Check: "After the round you can name the grenades you used, not the ones you carried.",
	},
	{
		ID: "freeze-time-buy-call", Kind: detect.KindBuyDesync, Title: "Call the buy in freeze time",
		Where: "with your party", Minutes: 0,
		Steps: []string{
			"One player says the buy as freeze time starts: \"full buy\", \"save\" or \"force\".",
			"Everyone buys that; disagree next round, not this one.",
			"Players short on money say so, and someone drops.",
		},
		Check: "In your next matches, the team buys the same way in almost every round.",
	},
	{
		ID: "economy-rule", Kind: detect.KindBuyDesync, Title: "Agree on one economy rule",
		Where: "with your party", Minutes: 5,
		Steps: []string{
			"Before you queue, agree on when you save: for example, save when most of you cannot buy a rifle and full utility.",
			"Agree what a force buy is, and when you do one (second round after a won pistol, say).",
			"Stick to it for a few matches before changing it.",
		},
		Check: "Nobody asks \"are we buying?\" in freeze time.",
	},
	{
		ID: "scoreboard-after-buy", Kind: detect.KindMissedDrop, Title: "Check the scoreboard after you buy",
		Where: "your matches", Minutes: 0,
		Steps: []string{
			"After you buy, hold Tab and look at the money column.",
			"If you have $3000 or more left and a teammate cannot afford a rifle, ask what they want and drop it before freeze time ends.",
		},
		Check: "In your next matches, no teammate is on a pistol in a full-buy round while you keep enough money for a rifle.",
	},
	{
		ID: "drop-call", Kind: detect.KindMissedDrop, Title: "Ask and offer drops out loud",
		Where: "with your party", Minutes: 0,
		Steps: []string{
			"Players who are short say it at the start of freeze time (\"need a drop, AK\").",
			"The player with the most money answers first.",
		},
		Check: "Drops happen before freeze time ends, not halfway through the round.",
	},
	{
		ID: "smoke-and-flash-every-buy", Kind: detect.KindNoUtility, Title: "A smoke and a flash every rifle round",
		Where: "your matches", Minutes: 0,
		Steps: []string{
			"Every full-buy round, buy at least a smoke and a flash.",
			"Use one before your first fight: a smoke to cross, a flash to peek.",
			"If the round ends and you threw nothing, use one earlier next round.",
		},
		Check: "In your next matches, you throw something in nearly every rifle round you live through.",
	},
	{
		ID: "three-smokes", Kind: detect.KindNoUtility, Title: "Learn three smokes for your map",
		Where: "offline server", Minutes: 20,
		Steps: []string{
			practiceServer + ".",
			utilityGuide + ".",
			"Learn three smokes from the guide that you would really use: one to take each site and one to cross mid.",
			"Throw each until it lands five times in a row, then use them in your next match on that map.",
		},
		Check: "You can throw all three without looking up the lineup.",
	},
	{
		ID: "self-pop-flash", Kind: detect.KindOpeningDeath, Title: "Self pop-flash for the first peek",
		Where: "offline server", Minutes: 15,
		Steps: []string{
			practiceServer + ".",
			utilityGuide + ": look for a self pop-flash.",
			"Pick one for the angle where you most often die first, or the place your coach names, so you can throw it and peek behind it.",
			"Practice throw-then-peek until the flash pops just as you clear the corner.",
		},
		Check: "When you take the first fight, you have thrown a flash first or a teammate has.",
	},
	{
		ID: "entry-with-support", Kind: detect.KindOpeningDeath, Title: "Don't go first without a flash",
		Where: "with your party", Minutes: 0,
		Steps: []string{
			"Whoever goes first asks for a flash (\"flash me in B\") or throws their own.",
			"Without one, let the player who has utility go first, or wait for the support.",
		},
		Check: "In your next matches, your first deaths come after a flash more often.",
	},
}

// All returns every drill, grouped by kind in a fixed order.
func All() []Drill { return slices.Clone(library) }

// ForKind returns the drills for one kind of mistake, in library order.
func ForKind(k detect.Kind) []Drill {
	var out []Drill
	for _, d := range library {
		if d.Kind == k {
			out = append(out, d)
		}
	}
	return out
}

// ByID returns a drill by its ID.
func ByID(id string) (Drill, bool) {
	i := slices.IndexFunc(library, func(d Drill) bool { return d.ID == id })
	if i < 0 {
		return Drill{}, false
	}
	return library[i], true
}
