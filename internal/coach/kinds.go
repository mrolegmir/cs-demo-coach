package coach

import "github.com/mrolegmir/cs-demo-coach/internal/detect"

// kindInfo explains a kind of mistake to the LLM and backs the rule-based
// coach's wording.
type kindInfo struct {
	Title string `json:"title"`
	// Meaning is what the detector counts, as a noun phrase.
	Meaning string `json:"meaning"`
	// Chances is what the rate is measured against.
	Chances string `json:"chances"`
	// Why the mistake costs rounds.
	Why string `json:"why"`
	// Drill is a concrete way to practice.
	Drill string `json:"drill"`
}

var kinds = map[detect.Kind]kindInfo{
	detect.KindIsolatedDeath: {
		Title:   "Stop dying alone",
		Meaning: "untraded deaths with no teammate within trading distance",
		Chances: "deaths with a teammate still alive",
		Why:     "a death nobody can trade is a free 5v4; the enemy pays nothing for it",
		Drill:   "before taking a fight, check the minimap: if no teammate is close enough to swing after you, fall back or wait for one",
	},
	detect.KindMissedTrade: {
		Title:   "Trade your teammates",
		Meaning: "teammate deaths you were nearest to (straight line) where the killer survived 5 seconds",
		Chances: "times you were the nearest teammate within trading distance",
		Why:     "the enemy who just got a kill is often reloading or out of position; not punishing it turns an even fight into a lost one",
		Drill:   "play close to your entry (a few steps behind) and pre-aim the angle they are peeking; in deathmatch, practice swinging immediately after a teammate's death sound",
	},
	detect.KindFlashNoEnemy: {
		Title:   "Make your flashes count",
		Meaning: "flashbangs that blinded no enemy for at least 1 second",
		Chances: "flashbangs thrown",
		Why:     "a wasted flash is $200 and a free warning to the enemy that you are coming",
		Drill:   "learn 2 pop-flashes for the site you play most (workshop map or private server), and throw a flash only right before you or a teammate peek",
	},
	detect.KindTeamFlash: {
		Title:   "Stop flashing your own team",
		Meaning: "flashbangs that blinded a teammate or yourself for at least 1.5 seconds",
		Chances: "flashbangs thrown",
		Why:     "a team flash hands the enemy a free kill on a blind teammate",
		Drill:   "call flashes before throwing ('flashing B main') and practice lineups that pop behind your teammates' backs",
	},
	detect.KindUnusedUtility: {
		Title:   "Use your utility before you die",
		Meaning: "deaths while still carrying grenades (decoys excluded)",
		Chances: "deaths",
		Why:     "unused grenades are wasted money and missed chances to delay, blind or damage the enemy",
		Drill:   "decide on one grenade to use in every round before freeze time ends; if you are about to take a risky fight, throw something first",
	},
	detect.KindBuyDesync: {
		Title:   "Buy with your team",
		Meaning: "rounds bought out of line with the team (full buy while most saved, or no rifle buy despite the money while most full-bought)",
		Chances: "non-pistol rounds",
		Why:     "a split buy loses both ways: the full buyer fights alone and the savers are under-equipped",
		Drill:   "agree on the buy at the start of freeze time ('full buy', 'save', 'force') and drop weapons to teammates who are short",
	},
	detect.KindMissedDrop: {
		Title:   "Drop for your teammates",
		Meaning: "full-buy rounds you kept $3000+ while a teammate who could not afford a rifle stayed without one",
		Chances: "full-buy rounds you kept $3000+ after buying",
		Why:     "a teammate on a pistol next to four rifles is the easiest kill in the round; a drop costs you nothing you would use",
		Drill:   "after you buy, check the scoreboard money column; if someone is short and you have $3000+ left, ask what they need and drop it before freeze time ends",
	},
	detect.KindNoUtility: {
		Title:   "Throw your utility",
		Meaning: "full-buy rounds you lived 30+ seconds in without throwing a single grenade",
		Chances: "full-buy rounds you lived 30+ seconds in",
		Why:     "a rifle round without utility means taking every fight on even terms; one flash or smoke per round already changes duels",
		Drill:   "buy at least a flash and a smoke every full-buy round and use one before your first fight: a smoke to cross, a flash to peek",
	},
	detect.KindOpeningDeath: {
		Title:   "Don't take the first fight dry",
		Meaning: "first deaths of the round without a teammate flash blinding an enemy in the 2 seconds before",
		Chances: "rounds",
		Why:     "the first death decides most rounds; taking it without utility gives the enemy an easy 5v4",
		Drill:   "when you are first in, ask for a flash or throw your own pop-flash; otherwise let the entry with utility go first",
	},
}
