package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// A fight is a golden setup's character on the boss dummy for a minute or two under mod-chronicle, its bot
// running a fixed cycle from fightCyclesFile that never reacts to procs. The replay builds the same character
// in the sim with an empty rotation, casts each spell the log says the bot landed at the time it landed, and
// compares what came of it ability by ability.

const (
	fightsDir       = "sim/core/testdata/fights"
	fightCyclesFile = "cycles.json"

	// the server updates the dummy, its auras included, every map update, and the player, its swings, casts
	// and auras, every world tick (MapUpdateInterval and MinWorldUpdateTime live)
	mapTick   = 100 * time.Millisecond
	worldTick = 10 * time.Millisecond
	// up to here the bot swings too, as TestRecordedRun decides it
	meleeYards = 5

	autoShotSpell = 75
	// Sniper Training's buff on the server, and the aura the sim registers for it
	sniperTrainingBuff = 64420
	sniperTrainingAura = 53304

	defaultFightIterations = 1000
	fightSeed              = 7000
)

// bossDummy is the boss dummy (999000) as goldchar.jsonl captured it: a level 83 giant world boss.
var bossDummy = damageUnit{
	unitSnapshot: unitSnapshot{Level: 83, WorldBoss: true, Resistances: []float64{10643}, MaxHealth: 24009944},
	CreatureType: 5,
}

// fightMinionNames maps a minion's name in the log to its pet's name in the sim.
var fightMinionNames = map[string]string{}

// fightSimActions maps a server spell id in the log to the sim action that stands for it, when the sim
// deals the damage, aura or power under another id.
var fightSimActions = map[int32]core.ActionID{
	// power from the spell a cast or talent triggers, which the sim books under the cast or talent
	31818: {SpellID: 57946}, // Life Tap
	31930: {SpellID: 31878}, // Judgements of the Wise
	20268: {SpellID: 53408}, // Judgement of Wisdom's mana
	29131: {SpellID: 2687},  // Bloodrage's rage over time
	51178: {SpellID: 50213}, // King of the Jungle's energy, from Tiger's Fury
	68285: {SpellID: 34300}, // Improved Leader of the Pack
	16959: {SpellID: 37117}, // Primal Fury's rage
	12964: {SpellID: 13002}, // Unbridled Wrath
	34720: {SpellID: 34499}, // Thrill of the Hunt's mana
	29077: {SpellID: 29076}, // Master of Elements' mana
	30824: {SpellID: 30823}, // Shamanistic Rage's mana
	59117: {SpellID: 54118}, // Improved Soul Leech's mana
	35548: {SpellID: 35553}, // Combat Potency's energy
	14181: {SpellID: 58425}, // Relentless Strikes' energy, the sim's 5/5 rank
	53506: {SpellID: 24858}, // Moonkin Form's mana
	// a talent's buff or debuff, which the sim names by the talent or the judgement
	20053: {SpellID: 20057}, // Vengeance
	59578: {SpellID: 53488}, // The Art of War
	20186: {SpellID: 53408}, // Judgement of Wisdom
	54499: {SpellID: 20337}, // Heart of the Crusader
	26017: {SpellID: 26016}, // Vindication
	12970: {SpellID: 12974}, // Flurry
	12721: {SpellID: 12867}, // Deep Wounds
	58578: {SpellID: 50887}, // Icy Talons
	60433: {SpellID: 48511}, // Earth and Moon
	6150:  {SpellID: 19556}, // Improved Aspect of the Hawk's Quick Shots
	63468: {SpellID: 53238}, // Piercing Shots
	34501: {SpellID: 34503}, // Expose Weakness
	34837: {SpellID: 34839}, // Master Tactician
	64420: {SpellID: 53304}, // Sniper Training
	56453: {SpellID: 56344}, // Lock and Load
	57531: {SpellID: 31572}, // Arcane Potency
	48108: {SpellID: 44448}, // Hot Streak
	22959: {SpellID: 12873}, // Improved Scorch
	74396: {SpellID: 44545}, // Fingers of Frost's charges
	57761: {SpellID: 44549}, // Brain Freeze's Fireball!
	12579: {SpellID: 28593}, // Winter's Chill
	60795: {SpellID: 60794}, // Libram of Obstruction
	68055: {SpellID: 53696}, // Judgements of the Just
	60549: {SpellID: 60633}, // Deadly Gladiator's Libram of Fortitude's Deadly Aggression
	50448: {SpellID: 50449}, // Bloody Vengeance
	64859: {SpellID: 55226}, // Blade Barrier
	58374: {SpellID: 58397}, // Glyph of Blocking
	50227: {SpellID: 46953}, // Sword and Board
	29178: {SpellID: 30160}, // Elemental Devastation
	52437: {SpellID: 29724}, // Sudden Death
	57522: {SpellID: 57518}, // Wrecking Crew's Enrage
	60503: {SpellID: 68051}, // Taste for Blood
	30070: {SpellID: 29859}, // Blood Frenzy
	46857: {SpellID: 46855}, // Trauma
	33151: {SpellID: 33154}, // Surge of Light
	63848: {SpellID: 51662}, // Hunger For Blood's buff
	51124: {SpellID: 51130}, // Killing Machine
	51735: {SpellID: 51161}, // Ebon Plague
	// an item's proc, which the sim names by the item
	71643: {ItemID: 50348}, // Dislodged Foreign Object (heroic)'s stacks
	67759: {ItemID: 47188}, // Reign of the Unliving (heroic)'s Shard of Flame
	67760: {ItemID: 47188}, // and its Pillar of Flame
	// the sim's Sunder Armor is 47467, a rank players no longer get: the server turns it into a flat armor cut
	7386:  {SpellID: 47467},
	58567: {SpellID: 47467},
}

// fightLogIDs names a sim action under a tag, or a second sim aura of one server aura, by the server spell
// the log has it as.
var fightLogIDs = map[core.ActionID]int32{
	{SpellID: 27013}:         48468, // Insect Swarm's hit debuff
	{SpellID: 1680, Tag: 1}:  44949, // Whirlwind's off hand
	{SpellID: 55271, Tag: 2}: 70890, // Scourge Strike's shadow damage
	// death knight off hand strikes
	{SpellID: 49921, Tag: 2}: 66216, // Plague Strike
	{SpellID: 55268, Tag: 2}: 66962, // Frost Strike
	{SpellID: 51425, Tag: 2}: 66974, // Obliterate
	{SpellID: 49930, Tag: 2}: 66979, // Blood Strike
	// Lightning Overload's second spell
	{SpellID: 49238, Tag: 6}: 45284, // Lightning Bolt
	{SpellID: 49271, Tag: 6}: 45297, // Chain Lightning
}

// fightLogTicks are server spells a dot casts for its tick, which the log has as direct damage, by the dot.
var fightLogTicks = map[int32]int32{
	53352: 60053, // Explosive Shot
	// a leech's ticks are direct damage to Chronicle
	48300: 48300, // Devouring Plague
}

// fightRefreshesLeftOut are sim auras, by id, whose refreshes the server does its own way, with no effect on
// the fight; their uptime is still compared.
var fightRefreshesLeftOut = map[int32]string{
	53304: "Sniper Training: the server refreshes it every few seconds the hunter stands still, the sim's option keeps it up",
}

// fightEnergizeLeftOut are server power gains the sim gives some way the comparison doesn't read.
var fightEnergizeLeftOut = map[int32]string{
	57669: "Replenishment: the sim gives it as MP5 in the mana regen",
}

// fightMissesLeftOut are server spells whose logged misses decide nothing the sim models, or repeat another
// spell's.
var fightMissesLeftOut = map[int32]string{
	48819: "Consecration: its area aura burns on after a missed cast result, its ticks rolling their own",
	51585: "Blade Twisting's daze, which the boss dummy is immune to and the sim doesn't model",
	18118: "Aftermath's daze, which the boss dummy is immune to and the sim doesn't model",
	50783: "Slam's damage spell, which repeats a missed cast's miss that Slam (47475) already counts",
}

// fightSimAurasOn moves a sim aura to where the server has it.
var fightSimAurasOn = map[int32]string{
	48819: "target", // Consecration's area aura, on what it burns; the sim's sits on the paladin
}

// fightAurasByCast names a sim aura shared by several casts by the cast the fight's cycle has: the sim's
// Faerie Fire debuff is 770 whoever puts it on.
var fightAurasByCast = map[int32]int32{
	770:   16857, // Faerie Fire (Feral)
	48566: 48564, // Mangle (Bear)
}

// fightDrop lets one check of one fight disagree; each has a row in the fights README.
type fightDrop struct {
	Setup, Subject, Check string
}

var droppedFightChecks = []fightDrop{
	{"DeathknightBlood", "Bloodworm melee", "crit mean"},
	{"DeathknightBlood", "Bloodworm melee", "hit mean"},
	{"DeathknightFrost", "51714", "count"},
	{"DeathknightFrost", "51714", "misses"},
	{"DeathknightFrostUH", "51714", "misses"},
	{"DeathknightFrostUH", "melee", "count"},
	{"DeathknightUnholy", "melee", "count"},
	{"DruidBalance", "48468", "tick crit mean"},
	{"DruidBalance", "aura 48517 on self", "uptime"},
	{"DruidBalance", "aura 60486 on self", "applications"},
	{"DruidBalancePhase3", "48468", "tick crit mean"},
	{"DruidFeral", "48574", "crit mean"},
	{"DruidFeral", "48574", "hit mean"},
	{"DruidFeralApl", "melee", "count"},
	{"MageFire", "energize 29076 mana", "gain sizes"},
	{"MageFrost", "aura 44549 on self", "applications"},
	{"MageFrost", "aura 57669 on self", "applications"},
	{"MageFrostFire", "47610", "crit mean"},
	{"MageFrostFire", "47610", "tick hit mean"},
	{"MageFrostFire", "energize 29076 mana", "gain sizes"},
	{"PaladinProtection", "26016", "misses"},
	{"PaladinProtection", "aura 26016 on target", "applications"},
	{"PaladinProtection", "aura 26016 on target", "uptime"},
	{"PaladinRetribution", "aura 53408 on target", "applications"},
	{"PaladinRetribution", "aura 60314 on self", "uptime"},
	{"PriestShadow", "aura 60486 on self", "applications"},
	{"PriestShadow", "aura 60486 on self", "uptime"},
	{"PriestSmite", "aura 60486 on self", "applications"},
	{"PriestSmite", "aura 60486 on self", "uptime"},
	{"RogueAssassination", "melee", "count"},
	{"RogueCombat", "aura 58670 on self", "applications"},
	{"RogueCombat", "aura 58670 on self", "uptime"},
	{"RogueCombat", "melee", "count"},
	{"RogueSubtlety", "aura 48660 on target", "applications"},
	{"RogueSubtlety", "aura 48660 on target", "uptime"},
	{"RogueSubtlety", "melee", "count"},
	{"ShamanElemental", "aura 60486 on self", "applications"},
	{"ShamanEnhancement", "60103", "crit mean"},
	{"ShamanEnhancement", "60103", "hit mean"},
	{"WarriorArms", "47465", "tick crit mean"},
	{"WarriorArms", "47465", "tick hit mean"},
	{"WarriorProtectionWarrior", "47465", "tick hit mean"},
}

// fightLeftOut is a server aura by the log's id, where it sits ("self", "target", a minion's name), "" for
// anywhere.
type fightLeftOut struct {
	Spell int32
	On    string
}

func (k fightLeftOut) subject() string {
	if k.On == "" {
		return strconv.Itoa(int(k.Spell))
	}
	return fmt.Sprintf("%d on %s", k.Spell, k.On)
}

// fightAurasLeftOut are server auras the sim has no aura for, and why that's no mismatch.
var fightAurasLeftOut = map[fightLeftOut]string{
	{Spell: 23885}:             "Bloodthirst's healing buff: the sim doesn't heal",
	{Spell: 29131}:             "Bloodrage's rage over time: the sim gives the rage with no aura",
	{Spell: 60468}:             "Shadow Embrace's healing cut: the sim doesn't heal",
	{Spell: 71644}:             "Dislodged Foreign Object (heroic)'s trigger: the sim stacks Surging Power with no aura for it",
	{Spell: 57623}:             "Horn of Winter: the sim keeps the setup's buff up all fight, which its cast doesn't refresh",
	{Spell: 53138}:             "Abomination's Might on a Bloodworm: the sim gives pets the setup's raid auras as stats, not auras",
	{Spell: 65142}:             "Ebon Plague's linked Crypt Fever aura: the sim has Ebon Plague as one aura",
	{Spell: 62071}:             "Savage Roar's second aura: the sim has Savage Roar as one aura",
	{Spell: 51185}:             "King of the Jungle's damage buff: the sim has it inside Enrage's aura",
	{Spell: 34084}:             "the hunter passive 34082's marker on each ranged hit, a spell with no effects",
	{Spell: 53257}:             "Cobra Strikes, for the pet's next specials: no pet fights",
	{Spell: 49050}:             "Aimed Shot's healing cut: the sim doesn't heal",
	{Spell: 42846, On: "self"}: "Arcane Missiles' channel on the mage: the sim has the channel's aura on the target only",
	{Spell: 44544}:             "Fingers of Frost's second aura: the sim has Fingers of Frost as one aura, its charges",
	{Spell: 48156, On: "self"}: "Mind Flay's channel on the priest: the sim has the channel's aura on the target only",
	{Spell: 32409, On: "self"}: "Shadow Word: Death's backlash on the priest: the sim doesn't hurt the priest",
	{Spell: 48301}:             "Mind Trauma's healing cut: the sim doesn't heal",
	{Spell: 5302}:              "Defensive State, which only opens Riposte: the sim doesn't model it",
	{Spell: 51699}:             "Honor Among Thieves: the sim gives its combo points with no aura, from its party crit option",
	{Spell: 52916}:             "Honor Among Thieves: the sim gives its combo points with no aura, from its party crit option",
	{Spell: 49231}:             "Earth Shock's attack speed slow: the dummy never attacks, and the sim doesn't model it",
	{Spell: 47486}:             "Mortal Strike's healing cut: the sim doesn't heal",
	{Spell: 67544}:             "Lock and Load's cooldown marker: the sim keeps the cooldown with no aura",
	{Spell: 69369}:             "Predator's Swiftness, an instant Nature spell: the sim doesn't model it, and no cycle casts one",
}

// fightSimAurasLeftOut are sim auras the server has no aura for, by the sim's action. Left in, one would count
// as part of the server aura its id shares.
var fightSimAurasLeftOut = map[core.ActionID]string{
	{SpellID: 44448, Tag: 1}: "Hot Streak's count of crits in a row, which the server keeps inside the talent's script",
}

type fightCycles struct {
	Fights []fightCycle `json:"fights"`
}

// fightCycle is what a setup's bot does in its fight. The e2e runs it; the replay reads it to tell the bot's
// casts from everything they trigger.
type fightCycle struct {
	Setup string `json:"setup"`
	// how long the bot keeps cycling
	Seconds int     `json:"seconds"`
	Yards   float64 `json:"yards"`
	Behind  bool    `json:"behind,omitempty"`
	// a hunter's ammo item, set as the ammo slot
	Ammo int32 `json:"ammo,omitempty"`
	// passives the golden build doesn't teach and the sim character has, which the e2e .learns
	Learn []int32 `json:"learn,omitempty"`
	// cast once each, in order, before the cycle; Auto Shot here starts a hunter shooting
	Before []fightStep `json:"before,omitempty"`
	// tried in order, again and again, whatever the server says to each
	Cycle []fightStep `json:"cycle"`
}

type fightStep struct {
	Spell int32  `json:"spell"`
	Name  string `json:"name"`
	// how long the bot waits after trying this step before it tries the next
	WaitMs int `json:"waitMs"`
}

func readFightCycles(path string) ([]fightCycle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file fightCycles
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return file.Fights, nil
}

func fightCycleFor(cycles []fightCycle, setup string) (fightCycle, error) {
	i := slices.IndexFunc(cycles, func(c fightCycle) bool { return c.Setup == setup })
	if i < 0 {
		return fightCycle{}, fmt.Errorf("no cycle for setup %q", setup)
	}
	return cycles[i], nil
}

func (c fightCycle) melee() bool { return c.Yards <= meleeYards }

// casts is every spell the bot casts, Auto Shot aside: the spells the replay casts too.
func (c fightCycle) casts() map[int32]bool {
	casts := map[int32]bool{}
	for _, step := range slices.Concat(c.Before, c.Cycle) {
		if step.Spell != autoShotSpell {
			casts[step.Spell] = true
		}
	}
	return casts
}

// fightSetupName is the setup a fight's file is named for: `<setup>.log.gz` committed, or the e2e's
// `<setup>_<player>_<unix>.log`.
func fightSetupName(path string) string {
	base := filepath.Base(path)
	if i := strings.IndexAny(base, "_."); i >= 0 {
		return base[:i]
	}
	return base
}

// fightPlayerName is the player an e2e log's name carries, "" for a committed fight's.
func fightPlayerName(path string) string {
	parts := strings.Split(strings.SplitN(filepath.Base(path), ".", 2)[0], "_")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

// fightStartFloor is the second an e2e log's fight began, in ms on the log's own clock, which the e2e names
// its log after; 0 for a committed fight, trimmed to the fight already. The player's casts before it are the
// golden build's, e.g. a death knight's Horn of Winter.
func fightStartFloor(path string) int64 {
	parts := strings.Split(strings.SplitN(filepath.Base(path), ".", 2)[0], "_")
	if len(parts) < 3 {
		return 0
	}
	seconds, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0
	}
	return seconds * 1000
}

type fightCast struct {
	Spell int32
	At    time.Duration // from the start of the fight
}

// fightLog is what the replay reads out of a fight's Chronicle log.
type fightLog struct {
	path   string
	setup  string
	log    *chronicleLog
	player combatant
	// the player and every pet or guardian it owns, by GUID
	mine map[string]bool
	// the dummy, the unit they hit most: anything else they hit, like a critter wandering into Thunder Clap,
	// isn't in the sim's fight
	target string
	// sim pet names of the minions the log has
	minions []string
	// from the bot's first swing, Auto Shot or cast, a hardcast's at its start, to the last damage it or a
	// minion dealt
	startMs, endMs int64
	casts          []fightCast
	// the first and last melee swing or Auto Shot, from the start
	autos                  [2]time.Duration
	hasAutos               bool
	sniperTraining         bool
	names                  map[int32]string
	cycle                  fightCycle
	spellsWithoutCastEvent []int32
	// for each aura on the player that went on before the fight, by the sim's id, when the last time it went
	// on was, from the start: a proc's internal cooldown runs from then
	preFightProcs map[fightAuraKey]time.Duration
}

func (fl *fightLog) duration() time.Duration { return msDuration(fl.endMs - fl.startMs) }

func msDuration(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

func readFightLog(path string, c fightCycle, source string) (*fightLog, error) {
	log, err := readChronicleLog(path)
	if err != nil {
		return nil, err
	}
	if source == "" {
		source = fightPlayerName(path)
	}
	fl, err := newFightLog(log, c, source, fightStartFloor(path))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	fl.path = path
	return fl, nil
}

// newFightLog reads the fight out of the log, the player's casts and swings from fromMs on.
func newFightLog(log *chronicleLog, c fightCycle, source string, fromMs int64) (*fightLog, error) {
	player, err := pickSource(log, source)
	if err != nil {
		return nil, err
	}
	fl := &fightLog{log: log, setup: c.Setup, cycle: c, player: player, mine: map[string]bool{player.GUID: true},
		names: map[int32]string{}}
	for guid := range log.Owners {
		if ownerOf(log, guid) == player.GUID {
			fl.mine[guid] = true
			fl.minions = append(fl.minions, simMinionName(log.Names[guid]))
		}
	}
	sort.Strings(fl.minions)

	casts := c.casts()
	var castMs []int64
	var castSpells []int32
	var autoMs []int64
	for _, e := range log.Events {
		if e.Spell.ID != 0 && e.Spell.Name != "" {
			fl.names[e.Spell.ID] = e.Spell.Name
		}
		if e.Src.GUID != player.GUID {
			continue
		}
		switch {
		case e.Type == "SPELL_AURA_APPLIED" && e.Spell.ID == sniperTrainingBuff:
			fl.sniperTraining = true
		case e.TimeMs < fromMs:
		case e.Type == "SPELL_CAST_SUCCESS" && e.Spell.ID == autoShotSpell:
			autoMs = append(autoMs, e.TimeMs)
		case e.Type == "SPELL_CAST_SUCCESS" && casts[e.Spell.ID]:
			castMs = append(castMs, e.TimeMs)
			castSpells = append(castSpells, e.Spell.ID)
		case e.isSwing():
			autoMs = append(autoMs, e.TimeMs)
		}
	}
	var firsts []int64
	for i, ms := range castMs {
		firsts = append(firsts, ms-hardcastLead(castSpells[i]).Milliseconds())
	}
	if len(autoMs) > 0 {
		firsts = append(firsts, slices.Min(autoMs))
	}
	if len(firsts) == 0 {
		return nil, fmt.Errorf("%s cast none of %s's cycle and never swung or shot", player.Name, c.Setup)
	}
	fl.startMs = slices.Min(firsts)
	hits := map[string]int{}
	for _, e := range log.Events {
		if fl.mine[e.Src.GUID] && !fl.mine[e.Dst.GUID] && e.TimeMs >= fl.startMs && (e.isDamage() || e.isMiss()) {
			hits[e.Dst.GUID]++
		}
	}
	for guid, n := range hits {
		if n > hits[fl.target] || n == hits[fl.target] && guid < fl.target {
			fl.target = guid
		}
	}
	for _, e := range log.Events {
		if fl.mine[e.Src.GUID] && e.Dst.GUID == fl.target && e.TimeMs >= fl.startMs && (e.isDamage() || e.isMiss()) {
			fl.endMs = max(fl.endMs, e.TimeMs)
		}
	}
	if fl.endMs <= fl.startMs {
		return nil, fmt.Errorf("%s dealt no damage after its first cast", player.Name)
	}
	for i, ms := range castMs {
		fl.casts = append(fl.casts, fightCast{Spell: castSpells[i], At: msDuration(ms - fl.startMs)})
	}
	fl.preFightProcs = map[fightAuraKey]time.Duration{}
	up := map[int32]bool{}
	for _, e := range log.Events {
		if e.TimeMs >= fl.startMs || e.Src.GUID != player.GUID || e.Dst.GUID != player.GUID {
			continue
		}
		switch e.Type {
		case "SPELL_AURA_APPLIED":
			if !up[e.Spell.ID] {
				spell, item := logAction(e.Spell.ID)
				fl.preFightProcs[fightAuraKey{Spell: spell, Item: item, On: "self"}] = msDuration(e.TimeMs - fl.startMs)
			}
			up[e.Spell.ID] = true
		case "SPELL_AURA_REMOVED":
			up[e.Spell.ID] = false
		}
	}
	if len(autoMs) > 0 {
		fl.hasAutos = true
		fl.autos = [2]time.Duration{msDuration(slices.Min(autoMs) - fl.startMs), msDuration(slices.Max(autoMs) - fl.startMs)}
	}
	for spell := range casts {
		if !slices.ContainsFunc(fl.casts, func(cast fightCast) bool { return cast.Spell == spell }) {
			fl.spellsWithoutCastEvent = append(fl.spellsWithoutCastEvent, spell)
		}
	}
	slices.Sort(fl.spellsWithoutCastEvent)
	return fl, nil
}

func simMinionName(name string) string {
	if simName, ok := fightMinionNames[name]; ok {
		return simName
	}
	return name
}

// fightKey names an ability: a spell, Melee (0) or Auto Shot (75), of the player or one of its minions.
type fightKey struct {
	Minion string
	Spell  int32
	// a sim action named by its item, which a log never has
	Item int32
}

func (k fightKey) subject() string {
	var s string
	switch {
	case k.Item != 0:
		s = fmt.Sprintf("item %d", k.Item)
	case k.Spell == 0:
		s = "melee"
	default:
		s = strconv.Itoa(int(k.Spell))
	}
	if k.Minion != "" {
		s = k.Minion + " " + s
	}
	return s
}

// fightAuraKey is an aura the player or a minion put on: On is "self", "target" or the minion's name.
type fightAuraKey struct {
	Spell, Item int32
	On          string
}

func (k fightAuraKey) subject() string {
	if k.Item != 0 {
		return fmt.Sprintf("aura item %d on %s", k.Item, k.On)
	}
	return fmt.Sprintf("aura %d on %s", k.Spell, k.On)
}

type fightEnergizeKey struct {
	Spell, Item int32
	Power       string
}

func (k fightEnergizeKey) subject() string {
	if k.Item != 0 {
		return fmt.Sprintf("energize item %d %s", k.Item, k.Power)
	}
	return fmt.Sprintf("energize %d %s", k.Spell, k.Power)
}

// logAction is the sim's action for a log spell id: the same id unless fightSimActions maps it.
func logAction(spell int32) (int32, int32) {
	if id, ok := fightSimActions[spell]; ok {
		return id.SpellID, id.ItemID
	}
	return spell, 0
}

type auraSample struct {
	applied int
	// the log's: how often it went on while off, refreshes and stack changes aside
	gained int
	uptime time.Duration
}

// castsAfterTheEnd is the reason of a cast the replay never got to: it would have come after the sim's fight
// ended, held for a cast or GCD that ended too late.
const castsAfterTheEnd = ""

type castResult struct {
	landed bool
	at     time.Duration
	reason string
	// waited for the sim's own cast or GCD before it, which landed on the sim's map update, not the server's
	held bool
	// what it waited longer for, landing late
	waited string
}

// onTime is whether the cast landed within a map update of the logged time, or two when it waited for the
// sim's cast before it: that one's landing was rounded to the sim's own map update, a second rounding.
func (r castResult) onTime(want time.Duration) bool {
	d := r.at - want
	return r.landed && d >= -mapTick && (d <= mapTick || r.held && d <= 2*mapTick)
}

// fightSample is one fight's worth of what the comparison reads: the log's, or one sim iteration's.
type fightSample struct {
	casts []castResult
	// each ability's direct damage, avoidance included, and its ticks
	direct, periodic map[fightKey]*outcomeBreakdown
	ticks            map[fightKey][]int64 // ms from the start
	auras            map[fightAuraKey]*auraSample
	energize         map[fightEnergizeKey]float64
	// each single gain's size: all of the log's, and the sim's it gave alone in its first gainSizeIterations
	gains  map[fightEnergizeKey][]float64
	damage float64
	// the log's fightAurasLeftOut that went on
	leftOut map[fightLeftOut]bool
}

func newFightSample(casts int) *fightSample {
	return &fightSample{
		casts:    make([]castResult, casts),
		direct:   map[fightKey]*outcomeBreakdown{},
		periodic: map[fightKey]*outcomeBreakdown{},
		ticks:    map[fightKey][]int64{},
		auras:    map[fightAuraKey]*auraSample{},
		energize: map[fightEnergizeKey]float64{},
		gains:    map[fightEnergizeKey][]float64{},
		leftOut:  map[fightLeftOut]bool{},
	}
}

// attempts is how many times the ability went off: landed or avoided.
func attempts(b *outcomeBreakdown) int64 {
	if b == nil {
		return 0
	}
	n := b.landed()
	for _, count := range b.Avoided {
		n += count
	}
	return n
}

func (s *fightSample) aura(key fightAuraKey) *auraSample {
	a := s.auras[key]
	if a == nil {
		a = &auraSample{}
		s.auras[key] = a
	}
	return a
}

// fightTally is one side of the comparison: the log's one fight, or the sim's iterations.
type fightTally struct {
	samples []*fightSample
	// the sim's fightSimAurasLeftOut its character has
	leftOut []core.ActionID
}

func newFightTally() *fightTally { return &fightTally{} }

func breakdown(m map[fightKey]*outcomeBreakdown, key fightKey) *outcomeBreakdown {
	b := m[key]
	if b == nil {
		b = &outcomeBreakdown{}
		m[key] = b
	}
	return b
}

// avoidKind puts the log's and the sim's avoidance on one footing: the sim can't tell a spell's resist from
// its miss.
func avoidKind(kind string) string {
	kind = strings.ToLower(kind)
	if kind == "resist" {
		return "miss"
	}
	return kind
}

func avoidPlural(kind string) string {
	switch kind {
	case "miss":
		return "misses"
	case "parry":
		return "parries"
	}
	return kind + "s"
}

var logPowers = map[int64]string{0: "mana", 1: "rage", 2: "focus", 3: "energy", 6: "runic power"}

// tally folds the log's fight window into one sample: the player's and its minions' damage, misses, ticks,
// energizes, and the auras they put on or took off. An aura's state before the window counts for its uptime.
func (fl *fightLog) tally() *fightTally {
	t := newFightTally()
	s := newFightSample(0)
	t.samples = []*fightSample{s}

	type auraState struct {
		open     bool
		openedAt int64
		changed  bool
	}
	states := map[fightAuraKey]*auraState{}
	clip := func(from, to int64) time.Duration {
		return msDuration(max(0, min(to, fl.endMs)-max(from, fl.startMs)))
	}

	for _, e := range fl.log.Events {
		if !fl.mine[e.Src.GUID] || e.TimeMs > fl.endMs {
			continue
		}
		minion := ""
		if e.Src.GUID != fl.player.GUID {
			minion = simMinionName(fl.log.Names[e.Src.GUID])
		}
		spell, item := logAction(e.Spell.ID)
		key := fightKey{Minion: minion, Spell: spell, Item: item}
		inWindow := e.TimeMs >= fl.startMs

		switch {
		case (e.Type == "SPELL_AURA_APPLIED" || e.Type == "SPELL_AURA_REMOVED") && fl.auraOn(e.Dst) == "":

		case (e.Type == "SPELL_AURA_APPLIED" || e.Type == "SPELL_AURA_REMOVED") && fl.leftOut(e) != nil:
			s.leftOut[*fl.leftOut(e)] = true

		case e.Type == "SPELL_AURA_APPLIED" || e.Type == "SPELL_AURA_REMOVED":
			ak := fightAuraKey{Spell: spell, Item: item, On: fl.auraOn(e.Dst)}
			st := states[ak]
			if st == nil {
				st = &auraState{}
				states[ak] = st
			}
			a := s.aura(ak)
			if e.Type == "SPELL_AURA_APPLIED" {
				if inWindow {
					a.applied++
					st.changed = true
				}
				if !st.open {
					st.open, st.openedAt = true, e.TimeMs
					if inWindow {
						a.gained++
					}
				}
			} else if st.open {
				// only after a logged gain: Chronicle logs some auras only coming off (Crusader Strike's)
				a.uptime += clip(st.openedAt, e.TimeMs)
				st.open = false
				st.changed = st.changed || inWindow
			}

		case !inWindow:

		// what they did to anything but the dummy, like Shadow Word: Death's backlash, isn't the sim's
		case e.Dst.GUID != fl.target && (e.isDamage() || e.isMiss()):

		case e.isDamage():
			hit := e.damage()
			s.damage += float64(hit.Amount)
			tick, isTick := fightLogTicks[e.Spell.ID]
			if isTick {
				key.Spell = tick
			}
			if e.Type == "SPELL_PERIODIC_DAMAGE" || isTick {
				breakdown(s.periodic, key).addDamage(float64(hit.Amount), hit.Crit, hit.Glancing, hit.Blocked > 0)
				s.ticks[key] = append(s.ticks[key], e.TimeMs-fl.startMs)
			} else {
				breakdown(s.direct, key).addDamage(float64(hit.Amount), hit.Crit, hit.Glancing, hit.Blocked > 0)
			}

		case e.isMiss() && fightMissesLeftOut[e.Spell.ID] != "":

		case e.isMiss():
			breakdown(s.direct, key).avoid(avoidKind(e.missType()))

		case e.Type == "SPELL_ENERGIZE" || e.Type == "SPELL_PERIODIC_ENERGIZE":
			powerType := parseInt(arg(e.Args, 1))
			power, ok := logPowers[powerType]
			if !ok {
				continue
			}
			ek := fightEnergizeKey{Spell: spell, Item: item, Power: power}
			amount := float64(parseInt(arg(e.Args, 0))) / powerScale(uint32(powerType))
			s.energize[ek] += amount
			s.gains[ek] = append(s.gains[ek], amount)
		}
	}

	for ak, st := range states {
		if st.open {
			s.auras[ak].uptime += clip(st.openedAt, fl.endMs)
		}
		// an aura up for the whole fight, or never, is the setup's state, which the golden characters check
		if !st.changed {
			delete(s.auras, ak)
		}
	}
	return t
}

// leftOut is the fightAurasLeftOut entry an aura event falls under, nil for none.
func (fl *fightLog) leftOut(e chronicleEvent) *fightLeftOut {
	for _, k := range []fightLeftOut{{e.Spell.ID, fl.auraOn(e.Dst)}, {e.Spell.ID, ""}} {
		if _, ok := fightAurasLeftOut[k]; ok {
			return &k
		}
	}
	return nil
}

// auraOn is where an aura sits: "self", "target", a minion's name, or "" on any other unit.
func (fl *fightLog) auraOn(dst combatant) string {
	switch {
	case dst.GUID == fl.player.GUID:
		return "self"
	case fl.mine[dst.GUID]:
		return simMinionName(fl.log.Names[dst.GUID])
	case dst.GUID == fl.target:
		return "target"
	}
	return ""
}

// fightSim is the setup's character on the boss dummy, set up to replay one fight.
type fightSim struct {
	d     *damageSim
	fight *fightLog
	// the sim spell for each spell the bot casts
	spells map[int32]*core.Spell
	tally  *fightTally
	sample *fightSample
	// the units whose auras and power the comparison reads, and where an aura on each sits
	units []*core.Unit
	on    []string
	// what each spell or item had given in power after the last iteration
	gainTotals map[fightEnergizeKey]float64
	// in the first gainSizeIterations, what each had given, and how often, at the last hit or miss
	gainsAt map[fightEnergizeKey]gainCount
	// a spell's auras with several tags are one aura on the server, up from the first to the last: how many of
	// each key's are up, since when, and whether one went on or off in the fight
	auraUp      map[fightAuraKey]int
	auraSince   map[fightAuraKey]time.Duration
	auraChanged map[fightAuraKey]bool
	auraEverUp  map[fightAuraKey]bool
	// each aura that's up, and when it runs out and its stacks as the last map update saw them, to see a
	// refresh; fresh ones went on since that update
	upAuras map[*core.Aura]fightAuraKey
	expires map[*core.Aura]time.Duration
	stacks  map[*core.Aura]int32
	fresh   map[*core.Aura]bool
	// spells that skip the damage callbacks (Necrosis), seen only in their metrics, and what those held at the
	// last map update
	metricsOnly []metricsOnlySpell
	// for each spell the bot casts, its misses in the metrics and in the outcomes seen, at the last map
	// update: a miss that skips the damage callbacks (Corruption's) only shows in the metrics
	missesCounted, missesSeen map[*core.Spell]int32
	pulled                    bool
	// when the last cast's GCD ends
	gcdUntil time.Duration
	// tests watch the damage and the auras go by; a refresh comes as another on
	onDamage func(sim *core.Simulation, key fightKey, periodic bool, result *core.SpellResult)
	onAura   func(sim *core.Simulation, key fightAuraKey, on bool)
}

func (f *fightSim) watchAuras() {
	for i, unit := range f.units {
		// a proc's trigger often shares the proc's id and is up for good, which would make the proc look so
		timed := map[core.ActionID]bool{}
		for _, aura := range unit.GetAuras() {
			if aura.Duration != core.NeverExpires {
				timed[aura.ActionID] = true
			}
		}
		for _, aura := range unit.GetAuras() {
			id := aura.ActionID
			if id.IsEmptyAction() || id.OtherID != 0 || aura.Duration == core.NeverExpires && timed[id] {
				continue
			}
			if _, ok := fightSimAurasLeftOut[id]; ok {
				f.tally.leftOut = append(f.tally.leftOut, id)
				continue
			}
			key := fightAuraKey{Spell: id.SpellID, Item: id.ItemID, On: f.on[i]}
			if logID, ok := fightLogIDs[id]; ok {
				key.Spell, key.Item = logID, 0
			}
			if cast, ok := fightAurasByCast[id.SpellID]; ok && f.fight.cycle.casts()[cast] {
				key.Spell = cast
			}
			if on, ok := fightSimAurasOn[id.SpellID]; ok {
				key.On = on
			}
			aura.ApplyOnGain(func(aura *core.Aura, sim *core.Simulation) {
				f.upAuras[aura], f.fresh[aura] = key, true
				f.auraChange(sim, key, true)
			})
			aura.ApplyOnExpire(func(aura *core.Aura, sim *core.Simulation) {
				delete(f.upAuras, aura)
				f.auraChange(sim, key, false)
			})
		}
	}
}

// auraChange counts an application when a key's first aura goes up, and its uptime when the last comes off.
func (f *fightSim) auraChange(sim *core.Simulation, key fightAuraKey, on bool) {
	n := f.auraUp[key]
	if on {
		f.auraUp[key] = n + 1
		if n > 0 {
			return
		}
		f.auraSince[key] = sim.CurrentTime
		if f.pulled {
			f.auraEverUp[key] = true
			f.sample.aura(key).applied++
		}
	} else {
		f.auraUp[key] = n - 1
		if n != 1 {
			return
		}
		if f.sample != nil {
			f.sample.aura(key).uptime += max(sim.CurrentTime-max(f.auraSince[key], 0), 0)
		}
	}
	// the fight's end takes everything off, which the server doesn't
	if f.pulled && sim.CurrentTime < sim.Duration {
		f.auraChanged[key] = true
		if f.onAura != nil {
			f.onAura(sim, key, on)
		}
	}
}

// refreshes counts an aura that's up running out later than it did, or with other stacks, as applied again:
// the server logs every update of an aura the client gets, a refresh, a stack or a charge, as an
// application. The sim's auras refresh by procs, rollovers or in place, and only some of those go through
// Activate. The server sends a unit's aura updates when it updates the unit, the player every world tick and
// the others every map update, so it looks at the player's auras (self) or the others' as each comes; two
// changes in one, or a change in the one the aura went on, count once.
func (f *fightSim) refreshes(sim *core.Simulation, self bool) {
	refreshed := map[fightAuraKey]bool{}
	for aura, key := range f.upAuras {
		if (key.On == "self") != self {
			continue
		}
		at, stacks := aura.ExpiresAt(), aura.GetStacks()
		// a permanent aura runs out at NeverExpires, which the margin would overflow
		if !f.fresh[aura] && (at-f.expires[aura] > time.Millisecond || stacks != f.stacks[aura]) {
			refreshed[key] = true
		}
		f.expires[aura], f.stacks[aura] = at, stacks
		delete(f.fresh, aura)
	}
	if !f.pulled {
		return
	}
	for key := range refreshed {
		f.sample.aura(key).applied++
		f.auraChanged[key] = true
		if f.onAura != nil {
			f.onAura(sim, key, true)
		}
	}
}

var simPowers = map[proto.ResourceType]string{
	proto.ResourceType_ResourceTypeMana: "mana", proto.ResourceType_ResourceTypeRage: "rage",
	proto.ResourceType_ResourceTypeEnergy: "energy", proto.ResourceType_ResourceTypeRunicPower: "runic power",
	proto.ResourceType_ResourceTypeFocus: "focus",
}

// newFightSim builds the setup's character the way the golden characters do, geared.
func newFightSim(s goldcharSetup, fl *fightLog) (*fightSim, error) {
	s.geared = true
	player, err := goldcharPlayer(s)
	if err != nil {
		return nil, err
	}
	mirror := s.Auras
	if hunter := player.GetHunter(); hunter != nil {
		ammo, ok := ammoTypes[fl.cycle.Ammo]
		if !ok {
			return nil, fmt.Errorf("%s: ammo %d has no sim setting", s.Name, fl.cycle.Ammo)
		}
		hunter.Options.Ammo = ammo
		if fl.sniperTraining {
			hunter.Options.SniperTrainingUptime = 1
			mirror = append(slices.Clone(mirror), sniperTrainingAura)
		}
	}
	return newFightSimFor(player, mirror, fl)
}

// newFightSimFor puts the player on the boss dummy with an empty rotation, so that only the replay casts,
// standing where the cycle has the bot stand. The server auras in mirror the sim registers go on at the
// start of every iteration.
func newFightSimFor(player *proto.Player, mirror []int32, fl *fightLog) (f *fightSim, err error) {
	// an empty APL rather than none: channels read the rotation when they end
	player.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
	player.DistanceFromTarget = fl.cycle.Yards
	player.InFrontOfTarget = !fl.cycle.Behind

	var rec damageRecord
	rec.Target = bossDummy
	d, err := newDamageSim(rec, player, mirror, 1)
	if err != nil {
		return nil, err
	}
	d.sim.BaseDuration = fl.duration()

	f = &fightSim{d: d, fight: fl, spells: map[int32]*core.Spell{}, tally: newFightTally(),
		gainTotals: map[fightEnergizeKey]float64{}}
	for id := range fl.cycle.casts() {
		simID, _ := logAction(id)
		spell := castableSpell(&d.char.Unit, simID)
		if spell == nil {
			return nil, fmt.Errorf("%s's sim character has no spell %d to cast", fl.setup, id)
		}
		f.spells[id] = spell
	}

	f.units = append(f.units, &d.char.Unit)
	f.on = append(f.on, "self")
	f.listen(&d.char.Unit, "")
	for _, pet := range d.char.Pets {
		f.units = append(f.units, &pet.Unit)
		f.on = append(f.on, pet.Name)
		f.listen(&pet.Unit, pet.Name)
	}
	f.units = append(f.units, d.target)
	f.on = append(f.on, "target")
	for i, unit := range f.units[:len(f.units)-1] {
		minion := ""
		if i > 0 {
			minion = f.on[i]
		}
		for _, spell := range unit.Spellbook {
			if spell.Flags.Matches(core.SpellFlagNoOnDamageDealt) && !spell.ActionID.IsEmptyAction() {
				f.metricsOnly = append(f.metricsOnly, metricsOnlySpell{spell: spell, minion: minion,
					periodic: hasDot(spell, d.target)})
			}
		}
	}
	f.auraUp, f.auraSince = map[fightAuraKey]int{}, map[fightAuraKey]time.Duration{}
	f.upAuras, f.expires = map[*core.Aura]fightAuraKey{}, map[*core.Aura]time.Duration{}
	f.stacks, f.fresh = map[*core.Aura]int32{}, map[*core.Aura]bool{}
	f.watchAuras()
	return f, nil
}

// castableSpell is the spell an APL could cast by that id, the lowest tag of several.
func castableSpell(unit *core.Unit, id int32) *core.Spell {
	var found *core.Spell
	for _, s := range unit.Spellbook {
		if s.ActionID.SpellID == id && s.ActionID.ItemID == 0 && s.ActionID.OtherID == 0 &&
			s.Flags.Matches(core.SpellFlagAPL) && (found == nil || s.ActionID.Tag < found.ActionID.Tag) {
			found = s
		}
	}
	return found
}

// simFightKey names a sim action the way fightLog.tally names a log one: both hands' swings are Melee, as
// Chronicle doesn't say which hand swung.
func simFightKey(minion string, id core.ActionID) fightKey {
	key := fightKey{Minion: minion, Spell: id.SpellID, Item: id.ItemID}
	if logID, ok := fightLogIDs[id]; ok {
		key.Spell = logID
	}
	switch id.OtherID {
	case proto.OtherAction_OtherActionAttack:
		key.Spell = 0
	case proto.OtherAction_OtherActionShoot:
		key.Spell = autoShotSpell
	}
	return key
}

func (f *fightSim) listen(unit *core.Unit, minion string) {
	record := func(periodic bool) func(*core.Aura, *core.Simulation, *core.Spell, *core.SpellResult) {
		return func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if sim.CurrentTime < 0 || result.Target.Type != core.EnemyUnit || f.sample == nil {
				return
			}
			// before any return below: the metrics count every miss, a dot's missed tick too, and
			// silentMisses adds back whatever this didn't see
			if result.Outcome.Matches(core.OutcomeMiss) {
				f.missesSeen[spell]++
			}
			// an aura or debuff landing, not a damage event
			if result.Landed() && result.Damage <= 0 {
				return
			}
			// Chronicle doesn't log a persistent area aura's missed tick (Consecration)
			if periodic && !result.Landed() && spell.AOEDot() != nil {
				return
			}
			key := simFightKey(minion, spell.ActionID)
			if key.Spell == 0 && key.Item == 0 && spell.ActionID.OtherID != proto.OtherAction_OtherActionAttack {
				return
			}
			f.addOutcome(sim, key, periodic, result)
			if f.onDamage != nil {
				f.onDamage(sim, key, periodic, result)
			}
		}
	}
	unit.RegisterAura(core.Aura{
		Label:    "fight outcomes",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt:       record(false),
		OnPeriodicDamageDealt: record(true),
	})
}

type metricsOnlySpell struct {
	spell    *core.Spell
	minion   string
	periodic bool
	seen     core.SpellMetrics
}

// hasDot is whether the spell has a dot on the target; Dot indexes a nil slice for a spell without one.
func hasDot(spell *core.Spell, target *core.Unit) (has bool) {
	defer func() { _ = recover() }()
	return spell.Dot(target) != nil
}

// silentMisses adds each cast spell's misses since the last map update that its outcomes didn't show.
func (f *fightSim) silentMisses(sim *core.Simulation) {
	for _, spell := range f.spells {
		counted := spell.SpellMetrics[f.d.target.UnitIndex].Misses
		silent := counted - f.missesCounted[spell] - f.missesSeen[spell]
		f.missesCounted[spell], f.missesSeen[spell] = counted, 0
		if silent <= 0 || f.sample == nil || sim.CurrentTime < 0 {
			continue
		}
		key := simFightKey("", spell.ActionID)
		for range silent {
			result := &core.SpellResult{Target: f.d.target, Outcome: core.OutcomeMiss}
			f.addOutcome(sim, key, false, result)
			if f.onDamage != nil {
				f.onDamage(sim, key, false, result)
			}
		}
	}
}

// metricsOnlyDamage adds what each metrics-only spell dealt since the last map update. The metrics keep no
// damage per outcome or tick, so a dot's damage (Unholy Blight's) is one tick, and anything else's is hits at
// their mean (Necrosis, which always hits).
func (f *fightSim) metricsOnlyDamage(sim *core.Simulation) {
	target := f.d.target
	for i := range f.metricsOnly {
		m := &f.metricsOnly[i]
		now := m.spell.SpellMetrics[target.UnitIndex]
		hits, damage := now.Hits-m.seen.Hits, now.TotalDamage-m.seen.TotalDamage
		m.seen = now
		if damage <= 0 || f.sample == nil || sim.CurrentTime < 0 {
			continue
		}
		key := simFightKey(m.minion, m.spell.ActionID)
		results := []*core.SpellResult{{Target: target, Outcome: core.OutcomeHit, Damage: damage}}
		periodic := m.periodic
		if !periodic && hits > 1 {
			results = nil
			for range hits {
				results = append(results, &core.SpellResult{Target: target, Outcome: core.OutcomeHit, Damage: damage / float64(hits)})
			}
		}
		for _, result := range results {
			f.addOutcome(sim, key, periodic, result)
			if f.onDamage != nil {
				f.onDamage(sim, key, periodic, result)
			}
		}
	}
}

func (f *fightSim) addOutcome(sim *core.Simulation, key fightKey, periodic bool, result *core.SpellResult) {
	if f.gainsAt != nil {
		f.noteGainSizes()
	}
	s := f.sample
	// a tick that misses is only a spell's miss in the log, which can't tell it from the cast's
	if periodic && !result.Landed() {
		periodic = false
	}
	if periodic {
		breakdown(s.periodic, key).addDamage(result.Damage, result.DidCrit(),
			result.Outcome.Matches(core.OutcomeGlance), result.Outcome.Matches(core.OutcomeBlock))
		s.ticks[key] = append(s.ticks[key], sim.CurrentTime.Milliseconds())
		s.damage += result.Damage
		return
	}
	b := breakdown(s.direct, key)
	if !result.Landed() {
		b.avoid(simAvoidance(result.Outcome))
		return
	}
	b.addDamage(result.Damage, result.DidCrit(), result.Outcome.Matches(core.OutcomeGlance),
		result.Outcome.Matches(core.OutcomeBlock))
	s.damage += result.Damage
}

func addAction(sim *core.Simulation, at time.Duration, priority core.ActionPriority, do func(*core.Simulation)) {
	sim.AddPendingAction(&core.PendingAction{NextActionAt: at, Priority: priority, OnAction: do})
}

// run replays the fight once per iteration, each on its own seed.
func (f *fightSim) run(iterations int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	for i := 0; i < iterations; i++ {
		f.runIteration(int64(fightSeed+i), f.scheduleCasts)
	}
	return nil
}

// runIteration runs one fight: the setup's auras on, the pull, then whatever schedule puts in.
func (f *fightSim) runIteration(seed int64, schedule func(sim *core.Simulation)) *fightSample {
	sim := f.d.sim
	f.pulled = false
	sim.Reseed(seed)
	// Reset reads this to pick how the fight ends: 0 ends it on time, after the log's length
	sim.Encounter.EndFightAtHealth = 0
	sim.Reset()
	// the target's health percent comes from this, else from the time left (Rend's +35% above 75% health);
	// a fight barely scratches the dummy
	sim.Encounter.EndFightAtHealth = bossDummy.MaxHealth
	for _, aura := range f.d.mirrored {
		if !aura.IsActive() {
			aura.Activate(sim)
		}
	}
	f.sample = newFightSample(len(f.fight.casts))
	f.gainsAt = nil
	if len(f.tally.samples) < gainSizeIterations {
		f.gainsAt = f.powerGainCounts()
	}
	f.gcdUntil = 0
	f.auraChanged, f.auraEverUp = map[fightAuraKey]bool{}, map[fightAuraKey]bool{}
	sim.PrePull()
	// after the pull, before anything the fight does at 0
	addAction(sim, 0, core.ActionPriorityPrePull-1, f.atPull)
	for i := range f.metricsOnly {
		f.metricsOnly[i].seen = core.SpellMetrics{}
	}
	f.missesCounted, f.missesSeen = map[*core.Spell]int32{}, map[*core.Spell]int32{}
	// the sim only looks at what expired when something else happens, and the replay can go a while without
	// anything, so every map update gets an event: auras run out on them, as on the server
	var mapUpdate func(sim *core.Simulation)
	mapUpdate = func(sim *core.Simulation) {
		f.refreshes(sim, false)
		f.metricsOnlyDamage(sim)
		f.silentMisses(sim)
		addAction(sim, sim.NextServerTick(sim.CurrentTime+time.Nanosecond), core.ActionPriorityLow, mapUpdate)
	}
	addAction(sim, sim.NextServerTick(0), core.ActionPriorityLow, mapUpdate)
	var playerUpdate func(sim *core.Simulation)
	playerUpdate = func(sim *core.Simulation) {
		f.refreshes(sim, true)
		addAction(sim, sim.CurrentTime+worldTick, core.ActionPriorityLow, playerUpdate)
	}
	addAction(sim, 0, core.ActionPriorityLow, playerUpdate)
	f.scheduleAutos(sim)
	schedule(sim)
	for !sim.Step() {
	}
	f.metricsOnlyDamage(sim)
	f.silentMisses(sim)
	sim.Cleanup()
	f.pulled = false
	f.readMetrics()
	f.tally.samples = append(f.tally.samples, f.sample)
	sample := f.sample
	f.sample = nil
	return sample
}

// atPull notes the auras already up, takes away the pets the server bot never had, and holds the autos
// until the log's first swing or shot.
func (f *fightSim) atPull(sim *core.Simulation) {
	f.pulled = true
	for key, n := range f.auraUp {
		if n > 0 {
			f.auraEverUp[key] = true
		}
	}
	for aura := range f.upAuras {
		f.expires[aura], f.stacks[aura] = aura.ExpiresAt(), aura.GetStacks()
		delete(f.fresh, aura)
	}
	for _, pet := range f.d.char.Pets {
		if pet.IsEnabled() && !slices.Contains(f.fight.minions, pet.Name) {
			pet.Disable(sim)
		}
	}
	// a proc before the fight, in the golden build, started its internal cooldown on the server
	for key, at := range f.fight.preFightProcs {
		aura := f.d.char.GetIcdAuraByID(core.ActionID{SpellID: key.Spell, ItemID: key.Item})
		if aura == nil {
			continue
		}
		if ready := at + aura.Icd.Duration; ready > aura.Icd.ReadyAt() {
			aura.Icd.Set(ready)
		}
	}
	if !f.fight.hasAutos || f.fight.autos[0] > 0 {
		f.d.char.AutoAttacks.CancelAutoSwing(sim)
	}
}

func (f *fightSim) scheduleAutos(sim *core.Simulation) {
	if !f.fight.hasAutos {
		return
	}
	aa := &f.d.char.AutoAttacks
	if f.fight.autos[0] > 0 {
		addAction(sim, f.fight.autos[0], core.ActionPriorityAuto, aa.EnableAutoSwing)
	}
	// the bot stops attacking when its cycle ends, though its dots tick on
	addAction(sim, f.fight.autos[1]+time.Millisecond, core.ActionPriorityAuto, aa.CancelAutoSwing)
}

func (f *fightSim) scheduleCasts(sim *core.Simulation) {
	for j, cast := range f.fight.casts {
		r := &castReplay{f: f, j: j, spell: f.spells[cast.Spell], at: cast.At}
		addAction(sim, max(cast.At-hardcastLead(cast.Spell), 0), castPriority, r.attempt)
	}
}

// hardcastLead is how long before it lands a hardcast can start at most: its base cast time, which haste
// and talents only shorten. A channel lands as it starts.
func hardcastLead(spell int32) time.Duration {
	sd := serverdata.SpellByID(spell)
	if sd == nil || sd.CastMs <= 0 || sd.Flags&serverdata.FlagChanneled != 0 {
		return 0
	}
	return time.Duration(sd.CastMs) * time.Millisecond
}

// castPriority puts a cast after a hardcast landing at the same time, which would otherwise lose its effects
const castPriority = core.ActionPriorityLow

// castReplay casts one logged spell so it lands when the log has it land: a hardcast starts its own cast
// time early, a channel and an instant go at the logged time.
type castReplay struct {
	f       *fightSim
	j       int
	spell   *core.Spell
	at      time.Duration
	replans int
	held    bool
	waits   int
	waited  string
}

// castWait is the longest a cast waits for the sim's own cast, GCD or cooldown: a proc that sped up the
// server's cast but not the sim's would otherwise cost the cast, and with it the damage and auras the rest
// of the comparison reads. It lands late, so it's still a miss.
const castWait = 2 * time.Second

func (r *castReplay) attempt(sim *core.Simulation) {
	spell := r.spell
	// planned from the latest the hardcast could start, then moved to where the sim's own cast time puts its
	// start; an instant, or a hardcast a proc made instant, waits for the logged time
	ct := spell.CastTime()
	if spell.Flags.Matches(core.SpellFlagChanneled) {
		ct = 0
	}
	if start := r.at - ct; start-sim.CurrentTime > time.Millisecond && r.replans < 8 {
		r.replans++
		addAction(sim, start, castPriority, r.attempt)
		return
	}
	// a GCD, cooldown or cast that ends within one map update holds the cast for it, once: the log's
	// timestamps jitter by a few ms, and the sim's casts land on its own map updates
	wait, why := r.f.blockedFor(sim, spell)
	switch {
	case wait > 0 && wait <= mapTick && !r.held:
		r.held = true
		addAction(sim, sim.CurrentTime+wait, castPriority, r.attempt)
		return
	case wait > 0 && wait <= castWait && r.waits < 3:
		r.waits++
		r.waited = why
		addAction(sim, sim.CurrentTime+wait, castPriority, r.attempt)
		return
	}
	if ok, reason := r.f.cast(sim, spell); !ok {
		r.f.sample.casts[r.j] = castResult{reason: reason}
		return
	}
	r.f.sample.casts[r.j] = castResult{landed: true, at: sim.CurrentTime + spell.CurCast.CastTime, held: r.held,
		waited: r.waited}
}

// cast casts the spell, or says why the sim refused it. The empty rotation re-polls every 50 ms while
// the GCD is up and holds the GCD timer that long, which no server does, so a timer past the last cast's
// GCD is cleared first.
func (f *fightSim) cast(sim *core.Simulation, spell *core.Spell) (bool, string) {
	if spell.Unit.GCD.ReadyAt() > sim.CurrentTime && f.gcdUntil <= sim.CurrentTime {
		spell.Unit.GCD.Set(sim.CurrentTime)
	}
	reason := refusal(sim, spell)
	if !spell.Cast(sim, nil) {
		if reason == "" {
			reason = "refused"
		}
		return false, reason
	}
	if t := spell.CurCast.EffectiveTime(); t > 0 {
		f.gcdUntil = max(f.gcdUntil, sim.CurrentTime+t)
	}
	return true, ""
}

// blockedFor is how long a cast, the GCD or a cooldown keeps the spell from casting.
// The longest of the three says why: "casting", "gcd" or "cooldown".
func (f *fightSim) blockedFor(sim *core.Simulation, spell *core.Spell) (wait time.Duration, why string) {
	for _, b := range []struct {
		why  string
		wait time.Duration
	}{
		{"casting", spell.Unit.Hardcast.Expires - sim.CurrentTime},
		{"gcd", core.TernaryDuration(spell.DefaultCast.GCD > 0, f.gcdUntil-sim.CurrentTime, 0)},
		{"cooldown", spell.TimeToReady(sim)},
	} {
		if b.wait > wait {
			wait, why = b.wait, b.why
		}
	}
	return wait, why
}

// refusal is why Cast would refuse the spell now, in castFn's terms, "" when nothing says it would.
func refusal(sim *core.Simulation, spell *core.Spell) string {
	switch {
	case spell.ExtraCastCondition != nil && !spell.ExtraCastCondition(sim, spell.Unit.CurrentTarget):
		return "condition"
	case !spell.IsReady(sim):
		return "cooldown"
	case spell.DefaultCast.GCD > 0 && !spell.Unit.GCD.IsReady(sim):
		return "gcd"
	case spell.Unit.Hardcast.Expires > sim.CurrentTime:
		return "casting"
	case !spell.CanCast(sim, spell.Unit.CurrentTarget):
		return "cost"
	}
	return ""
}

// readMetrics takes this iteration's share of each power gain, and leaves out an aura that never went on or
// off in the fight and never refreshed: the setup's state.
func (f *fightSim) readMetrics() {
	gains := f.powerGains()
	for key, total := range gains {
		if gain := total - f.gainTotals[key]; gain != 0 {
			f.sample.energize[key] += gain
		}
	}
	f.gainTotals = gains
	for key := range f.sample.auras {
		if !f.auraEverUp[key] || !f.auraChanged[key] {
			delete(f.sample.auras, key)
		}
	}
}

// powerGains is what each spell or item has given the character and its pets, over every iteration so far.
func (f *fightSim) powerGains() map[fightEnergizeKey]float64 {
	gains := map[fightEnergizeKey]float64{}
	for key, g := range f.powerGainCounts() {
		gains[key] = g.amount
	}
	return gains
}

type gainCount struct {
	amount float64
	events int32
}

// powerGainCounts is powerGains with how many gains made each. A spell's cost shares its id and power with
// what it gives, but a cost's metrics only count down.
func (f *fightSim) powerGainCounts() map[fightEnergizeKey]gainCount {
	gains := map[fightEnergizeKey]gainCount{}
	for _, unit := range f.units {
		for _, r := range unit.Metrics.ToProto().Resources {
			power, ok := simPowers[r.Type]
			id := core.ProtoToActionID(r.Id)
			if ok && id.OtherID == 0 && r.Gain > 0 {
				key := fightEnergizeKey{Spell: id.SpellID, Item: id.ItemID, Power: power}
				g := gains[key]
				gains[key] = gainCount{amount: g.amount + r.Gain, events: g.events + r.Events}
			}
		}
	}
	return gains
}

// gainSizeIterations is how many iterations note each power gain's size: reading the metrics at every hit
// costs too much for all of them.
const gainSizeIterations = 100

// noteGainSizes notes each power gain since the last hit or miss that came alone. The metrics only add gains
// up; a proc's gain comes with the hit that procs it, and a tick's between two hits. A gain of nothing (Thrill
// of the Hunt for a free shot) is no gain in the log.
func (f *fightSim) noteGainSizes() {
	now := f.powerGainCounts()
	for key, g := range now {
		was := f.gainsAt[key]
		if size := g.amount - was.amount; g.events-was.events == 1 && size > 0 {
			f.sample.gains[key] = append(f.sample.gains[key], size)
		}
	}
	f.gainsAt = now
}

// fightCheck is one comparison of a fight: Subject names what it's about by id, for drops.
type fightCheck struct {
	Subject string
	Label   string
	Check
	Dropped bool
}

type fightComparison struct {
	fl     *fightLog
	log    *fightTally
	sim    *fightTally
	checks []fightCheck
	simDPS float64
	logDPS float64
}

func (c *fightComparison) add(subject, label string, checks ...Check) {
	for _, check := range checks {
		c.checks = append(c.checks, fightCheck{Subject: subject, Label: label, Check: check})
	}
}

func (c *fightComparison) spellName(spell, item int32) string {
	switch {
	case item != 0:
		return fmt.Sprintf("item %d", item)
	case c.fl.names[spell] != "":
		return c.fl.names[spell]
	}
	return fmt.Sprintf("spell %d", spell)
}

func (c *fightComparison) abilityLabel(k fightKey) string {
	var name string
	switch {
	case k.Item != 0:
		name = fmt.Sprintf("item %d", k.Item)
	case k.Spell == 0:
		name = "Melee"
	default:
		name = fmt.Sprintf("%s (%d)", c.spellName(k.Spell, 0), k.Spell)
	}
	if k.Minion != "" {
		name = k.Minion + ": " + name
	}
	return name
}

// compareFight holds the log to the sim's iterations: the logged casts landing within a map update, then per
// ability its count, crits, each kind of avoidance and mean hit, crit and glance, per dot its ticks, their
// spacing and ticks per application, per aura its applications and uptime, and per power gain its amount.
func compareFight(fl *fightLog, logTally, simTally *fightTally) *fightComparison {
	c := &fightComparison{fl: fl, log: logTally, sim: simTally}
	logSample := logTally.samples[0]
	seconds := fl.duration().Seconds()
	c.logDPS = logSample.damage / seconds
	for _, s := range simTally.samples {
		c.simDPS += s.damage / seconds / float64(len(simTally.samples))
	}

	c.compareCasts()

	directKeys, periodicKeys := map[fightKey]bool{}, map[fightKey]bool{}
	auraKeys := map[fightAuraKey]bool{}
	energizeKeys := map[fightEnergizeKey]bool{}
	for _, s := range append([]*fightSample{logSample}, simTally.samples...) {
		for k := range s.direct {
			directKeys[k] = true
		}
		for k := range s.periodic {
			periodicKeys[k] = true
		}
		for k := range s.auras {
			auraKeys[k] = true
		}
		for k := range s.energize {
			energizeKeys[k] = true
		}
	}
	for _, k := range sortedFightKeys(directKeys) {
		c.compareDirect(k)
	}
	for _, k := range sortedFightKeys(periodicKeys) {
		c.comparePeriodic(k)
	}
	for _, k := range sortedKeys(auraKeys, func(a, b fightAuraKey) bool {
		return a.On < b.On || a.On == b.On && (a.Spell < b.Spell || a.Spell == b.Spell && a.Item < b.Item)
	}) {
		c.compareAura(k)
	}
	runicPower := false
	for _, k := range sortedKeys(energizeKeys, func(a, b fightEnergizeKey) bool {
		return a.Power < b.Power || a.Power == b.Power && (a.Spell < b.Spell || a.Spell == b.Spell && a.Item < b.Item)
	}) {
		if k.Power == "runic power" {
			runicPower = true
			continue
		}
		if why, ok := fightEnergizeLeftOut[k.Spell]; ok && k.Item == 0 {
			c.add(k.subject(), fmt.Sprintf("%s (%d) %s", c.spellName(k.Spell, 0), k.Spell, k.Power), notCompared("amount", why))
			continue
		}
		c.compareEnergize(k)
	}
	if runicPower {
		c.add("runic power", "runic power", notCompared("amount",
			"the server gives it through rune costs and Butchery's regen, neither of which it logs"))
	}
	for _, k := range sortedKeys(logSample.leftOut, func(a, b fightLeftOut) bool {
		return a.Spell < b.Spell || a.Spell == b.Spell && a.On < b.On
	}) {
		c.add("aura "+k.subject(), fmt.Sprintf("aura %s (%s)", c.spellName(k.Spell, 0), k.subject()),
			notCompared("applications", fightAurasLeftOut[k]))
	}
	for _, id := range simTally.leftOut {
		c.add("sim aura "+id.String(), "sim aura "+id.String(), notCompared("applications", fightSimAurasLeftOut[id]))
	}
	return c
}

func sortedFightKeys(keys map[fightKey]bool) []fightKey {
	return sortedKeys(keys, func(a, b fightKey) bool {
		if a.Minion != b.Minion {
			return a.Minion < b.Minion
		}
		if a.Spell != b.Spell {
			return a.Spell < b.Spell
		}
		return a.Item < b.Item
	})
}

func sortedKeys[K comparable](keys map[K]bool, less func(a, b K) bool) []K {
	out := make([]K, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

// castMissesAllowed is how many of a spell's logged casts the sim may miss per iteration on average: half, or
// two. The sim's procs and power go their own random ways, so a hardcast a proc sped up on the server, or a
// strike it had the energy for, isn't always the sim's; a GCD, cooldown or cost the sim gets wrong misses
// all of them.
func castMissesAllowed(logged int) float64 { return max(2, 0.5*float64(logged)) }

// compareCasts wants the sim to land the spell's logged casts within a map update of the log, missing no more
// than castMissesAllowed. A refusal is a miss: the replay never tries again. A cast the fight's end cut off in
// the sim isn't counted.
func (c *fightComparison) compareCasts() {
	bySpell := map[int32][]int{}
	var spells []int32
	for j, cast := range c.fl.casts {
		if _, ok := bySpell[cast.Spell]; !ok {
			spells = append(spells, cast.Spell)
		}
		bySpell[cast.Spell] = append(bySpell[cast.Spell], j)
	}
	slices.Sort(spells)
	n := float64(len(c.sim.samples))
	for _, spell := range spells {
		var onTime, late, ended float64
		reasons := map[string]int{}
		type worst struct {
			j    int
			hits int
		}
		var casts []worst
		for _, j := range bySpell[spell] {
			want := c.fl.casts[j].At
			var hits int
			for _, s := range c.sim.samples {
				r := s.casts[j]
				switch {
				case !r.landed && r.reason == castsAfterTheEnd:
					ended++
				case !r.landed:
					reasons[r.reason]++
				case r.onTime(want):
					hits++
				default:
					late++
				}
			}
			onTime += float64(hits)
			casts = append(casts, worst{j, hits})
		}
		misses := (late + float64(sum(mapValues(reasons)))) / n
		detail := fmt.Sprintf("%d logged, %.1f on time per iteration", len(bySpell[spell]), onTime/n)
		if late > 0 {
			detail += fmt.Sprintf(", %.1f off by more than a map update", late/n)
		}
		if len(reasons) > 0 {
			var parts []string
			for _, reason := range sortedNames(reasons) {
				parts = append(parts, fmt.Sprintf("%s %.1f", reason, float64(reasons[reason])/n))
			}
			detail += ", refused: " + strings.Join(parts, ", ")
		}
		if ended > 0 {
			detail += fmt.Sprintf(", %.1f past the sim's end", ended/n)
		}
		label := fmt.Sprintf("%s (%d)", c.spellName(spell, 0), spell)
		if misses > castMissesAllowed(len(bySpell[spell])) {
			sort.SliceStable(casts, func(a, b int) bool { return casts[a].hits < casts[b].hits })
			var missed []string
			for _, w := range casts[:min(3, len(casts))] {
				missed = append(missed, c.castMiss(w.j))
			}
			c.add(strconv.Itoa(int(spell)), label, fail("casts", fmt.Sprintf("%s; %.1f missed, allowed %.1f; worst %s",
				detail, misses, castMissesAllowed(len(bySpell[spell])), strings.Join(missed, ", "))))
			continue
		}
		c.add(strconv.Itoa(int(spell)), label, pass("casts", detail))
	}
	for _, spell := range c.fl.spellsWithoutCastEvent {
		c.add(strconv.Itoa(int(spell)), fmt.Sprintf("%s (%d)", c.spellName(spell, 0), spell),
			notCompared("casts", "the bot never landed it"))
	}
}

func mapValues(m map[string]int) []float64 {
	var values []float64
	for _, v := range m {
		values = append(values, float64(v))
	}
	return values
}

func sortedNames[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// castMiss describes a cast the sim didn't land on time: when, and what the iterations did instead.
func (c *fightComparison) castMiss(j int) string {
	want := c.fl.casts[j].At
	reasons := map[string]int{}
	var offsets []time.Duration
	for _, s := range c.sim.samples {
		r := s.casts[j]
		if r.landed {
			offsets = append(offsets, r.at-want)
		} else {
			reasons[r.reason]++
		}
	}
	var parts []string
	for _, reason := range sortedNames(reasons) {
		if reason == castsAfterTheEnd {
			parts = append(parts, fmt.Sprintf("past the end %d", reasons[reason]))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", reason, reasons[reason]))
	}
	if len(offsets) > 0 {
		slices.Sort(offsets)
		parts = append(parts, fmt.Sprintf("landed %d, median %+dms", len(offsets), offsets[len(offsets)/2].Milliseconds()))
	}
	waited := map[string]int{}
	for _, s := range c.sim.samples {
		if r := s.casts[j]; r.landed && r.waited != "" && !r.onTime(want) {
			waited[r.waited]++
		}
	}
	for _, why := range sortedNames(waited) {
		parts = append(parts, fmt.Sprintf("late for its %s %d", why, waited[why]))
	}
	return fmt.Sprintf("%.1fs (%s)", want.Seconds(), strings.Join(parts, ", "))
}

// outcomes is each sim iteration's breakdown of one ability, an empty one where it never went off.
func (c *fightComparison) outcomes(k fightKey, periodic bool) []*outcomeBreakdown {
	var out []*outcomeBreakdown
	for _, s := range c.sim.samples {
		m := s.direct
		if periodic {
			m = s.periodic
		}
		b := m[k]
		if b == nil {
			b = &outcomeBreakdown{}
		}
		out = append(out, b)
	}
	return out
}

func (c *fightComparison) compareDirect(k fightKey) {
	label, subject := c.abilityLabel(k), k.subject()
	logB := c.log.samples[0].direct[k]
	if logB == nil {
		logB = &outcomeBreakdown{}
	}
	sims := c.outcomes(k, false)
	c.add(subject, label, checkRange("count", float64(attempts(logB)), perFight(sims, func(b *outcomeBreakdown) (float64, bool) {
		return float64(attempts(b)), true
	}), 0.5, ""))
	c.add(subject, label, compareOutcomes("", logB, sims)...)

	kinds := map[string]bool{}
	for _, b := range append([]*outcomeBreakdown{logB}, sims...) {
		for kind := range b.Avoided {
			kinds[kind] = true
		}
	}
	// counts rather than rates: a rate of two or three attempts is too coarse to say anything, and the
	// attempts are the logged casts either way
	for _, kind := range sortedNames(kinds) {
		count := func(b *outcomeBreakdown) (float64, bool) { return float64(b.Avoided[kind]), true }
		log, _ := count(logB)
		c.add(subject, label, checkRange(avoidPlural(kind), log, perFight(sims, count), 0, ""))
	}
}

// perFight is one value per sim iteration that has it.
func perFight(sims []*outcomeBreakdown, value func(*outcomeBreakdown) (float64, bool)) []float64 {
	var out []float64
	for _, b := range sims {
		if v, ok := value(b); ok {
			out = append(out, v)
		}
	}
	return out
}

// compareOutcomes holds the log's crits and its hit, crit and glance means to the sim's, fight by fight, where
// the log has some.
func compareOutcomes(prefix string, logB *outcomeBreakdown, sims []*outcomeBreakdown) []Check {
	var checks []Check
	crits := func(b *outcomeBreakdown) (float64, bool) { return float64(b.Crit.N), true }
	if logB.landed() > 0 {
		log, _ := crits(logB)
		checks = append(checks, checkRange(prefix+"crits", log, perFight(sims, crits), 0, ""))
	}
	for _, o := range []struct {
		name string
		of   func(*outcomeBreakdown) outcomeStats
	}{
		{"hit mean", func(b *outcomeBreakdown) outcomeStats { return b.Hit }},
		{"crit mean", func(b *outcomeBreakdown) outcomeStats { return b.Crit }},
		{"glance mean", func(b *outcomeBreakdown) outcomeStats { return b.Glance }},
	} {
		meanOf := func(b *outcomeBreakdown) (float64, bool) {
			stats := o.of(b)
			return stats.mean(), stats.N > 0
		}
		log, ok := meanOf(logB)
		if !ok {
			continue
		}
		// a point or 0.1% for the server's truncations
		checks = append(checks, checkRange(prefix+o.name, log, perFight(sims, meanOf), max(1, 0.001*log)/2, ""))
	}
	return checks
}

func (c *fightComparison) comparePeriodic(k fightKey) {
	label, subject := c.abilityLabel(k), k.subject()
	logB := c.log.samples[0].periodic[k]
	if logB == nil {
		logB = &outcomeBreakdown{}
	}
	logTicks := c.log.samples[0].ticks[k]
	var counts, medians []float64
	for _, s := range c.sim.samples {
		counts = append(counts, float64(len(s.ticks[k])))
		if gaps := intervals(s.ticks[k]); len(gaps) > 0 {
			medians = append(medians, float64(median(gaps)))
		}
	}
	c.add(subject, label, checkRange("ticks", float64(len(logTicks)), counts, 0.5, ""))
	c.add(subject, label, compareOutcomes("tick ", logB, c.outcomes(k, true))...)

	// a dot's ticks are as far apart as the haste it went on with, which differs fight to fight
	if logGaps := intervals(logTicks); len(logGaps) > 0 && len(medians) > 0 {
		c.add(subject, label, checkRange("tick spacing", float64(median(logGaps)), medians,
			float64(mapTick.Milliseconds())/2, "ms"))
	}

	// a proc's dot is refreshed by whatever procs it, which a refresh-counting server and the sim needn't agree
	// on; the cycle's own dots go on when cast
	if k.Minion != "" || k.Item != 0 || !c.fl.cycle.casts()[k.Spell] {
		return
	}
	ak := fightAuraKey{Spell: k.Spell, On: "target"}
	logApplied := c.log.samples[0].auras[ak]
	if logApplied == nil || logApplied.applied == 0 {
		return
	}
	var ratios []float64
	for _, s := range c.sim.samples {
		if a := s.auras[ak]; a != nil && a.applied > 0 {
			ratios = append(ratios, float64(len(s.ticks[k]))/float64(a.applied))
		}
	}
	if len(ratios) > 0 {
		logRatio := float64(len(logTicks)) / float64(logApplied.applied)
		c.add(subject, label, checkRange("ticks per application", logRatio, ratios, 1/float64(logApplied.applied), ""))
	}
}

func (c *fightComparison) compareAura(k fightAuraKey) {
	label := fmt.Sprintf("aura %s (%d) on %s", c.spellName(k.Spell, k.Item), k.Spell, k.On)
	if k.Item != 0 {
		label = fmt.Sprintf("aura %s on %s", c.spellName(k.Spell, k.Item), k.On)
	}
	logA := c.log.samples[0].auras[k]
	if logA == nil {
		logA = &auraSample{}
	}
	var applied, uptime []float64
	for _, s := range c.sim.samples {
		a := s.auras[k]
		if a == nil {
			a = &auraSample{}
		}
		applied = append(applied, float64(a.applied))
		uptime = append(uptime, a.uptime.Seconds())
	}
	applications := checkRange("applications", float64(logA.applied), applied, 0.5, "")
	if why, ok := fightRefreshesLeftOut[k.Spell]; ok && k.Item == 0 {
		applications = notCompared("applications", why)
	}
	// each time it's up can start and end a map update apart: the sim's on its own map updates, a cast's on the
	// log's time. A refresh only moves the end, so it's no more slack.
	c.add(k.subject(), label, applications,
		checkRange("uptime", logA.uptime.Seconds(), uptime, mapTick.Seconds()*float64(1+logA.gained)/2, "s"))
}

func (c *fightComparison) compareEnergize(k fightEnergizeKey) {
	label := fmt.Sprintf("%s %s", c.spellName(k.Spell, k.Item), k.Power)
	if k.Item == 0 {
		label = fmt.Sprintf("%s (%d) %s", c.spellName(k.Spell, 0), k.Spell, k.Power)
	}
	var amounts []float64
	for _, s := range c.sim.samples {
		amounts = append(amounts, s.energize[k])
	}
	logAmount := c.log.samples[0].energize[k]
	logGains := c.log.samples[0].gains[k]
	most := 0.0
	if len(logGains) > 0 {
		most = slices.Max(logGains)
	}
	// the server truncates each gain, and one ticking on can land either side of the fight's end
	floor := max(1, 0.005*math.Abs(mean(amounts))+most/2)
	c.add(k.subject(), label, checkRange("amount", logAmount, amounts, floor, ""))
	if len(logGains) > 0 {
		var simGains []float64
		for _, s := range c.sim.samples {
			simGains = append(simGains, s.gains[k]...)
		}
		c.add(k.subject(), label, checkGainSizes(logGains, simGains))
	}
}

// checkGainSizes wants each of the log's gains within the sizes the sim gave one at a time, give or take a
// point or 1%: the server truncates, and a gain that scales with a stat moves with its procs. The amount alone
// spreads with how often a proc goes off, which hides a kind of gain the sim never gives (Master of Elements
// for a spell the sim refunds nothing for).
func checkGainSizes(log, sim []float64) Check {
	const name = "gain sizes"
	if len(sim) == 0 {
		return notCompared(name, "the sim never gave one alone")
	}
	lo, hi := slices.Min(sim), slices.Max(sim)
	detail := fmt.Sprintf("log %.4g to %.4g over %d gains, sim %.4g to %.4g", slices.Min(log), slices.Max(log),
		len(log), lo, hi)
	var outside []string
	seen := map[float64]bool{}
	for _, g := range log {
		if slack := max(1, 0.01*g); !seen[g] && (g < lo-slack || g > hi+slack) {
			outside = append(outside, fmt.Sprintf("%.4g", g))
		}
		seen[g] = true
	}
	if len(outside) > 0 {
		return fail(name, detail+"; outside: "+strings.Join(outside, ", "))
	}
	return pass(name, detail)
}

func sum(values []float64) float64 {
	var total float64
	for _, v := range values {
		total += v
	}
	return total
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return sum(values) / float64(len(values))
}

func stdDev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	m := mean(values)
	var ss float64
	for _, v := range values {
		ss += (v - m) * (v - m)
	}
	return math.Sqrt(ss / float64(len(values)-1))
}

// checkRange holds the log's one fight to the sim's iterations of it: inside their range, widened by two of
// their standard deviations, taken as at least floor. Over a thousand iterations the range reaches about
// three deviations from the mean, so a normal spread fails past about five, the parity rules' |z|; a rare
// event the iterations saw, like a binary spell's miss taking a dot's whole duration, passes as it should.
func checkRange(name string, log float64, sim []float64, floor float64, unit string) Check {
	if len(sim) == 0 {
		return notCompared(name, "the sim never had it")
	}
	lo, hi := slices.Min(sim), slices.Max(sim)
	widen := 2 * max(stdDev(sim), floor)
	detail := fmt.Sprintf("log %.4g%s, sim %.4g%s (%.4g to %.4g over %d fights)", log, unit, mean(sim), unit, lo, hi, len(sim))
	if log >= lo-widen && log <= hi+widen {
		return pass(name, detail)
	}
	return fail(name, detail)
}

// replayFight builds the sim for one fight and compares it with the log.
func replayFight(s goldcharSetup, fl *fightLog, iterations int) (*fightComparison, error) {
	f, err := newFightSim(s, fl)
	if err != nil {
		return nil, err
	}
	if err := f.run(iterations); err != nil {
		return nil, err
	}
	return compareFight(fl, fl.tally(), f.tally), nil
}

// applyDrops marks the checks a drop lets through, and returns the drops for this setup nothing matched.
func (c *fightComparison) applyDrops(drops []fightDrop) (stale []fightDrop) {
	for _, drop := range drops {
		if drop.Setup != c.fl.setup {
			continue
		}
		used := false
		for i := range c.checks {
			if c.checks[i].Subject == drop.Subject && c.checks[i].Name == drop.Check {
				c.checks[i].Dropped = true
				used = used || !c.checks[i].Passed
			}
		}
		if !used {
			stale = append(stale, drop)
		}
	}
	return stale
}

// failed is how many checks fail without a drop.
func (c *fightComparison) failed() int {
	var n int
	for _, check := range c.checks {
		if !check.Passed && !check.Dropped {
			n++
		}
	}
	return n
}

func (c *fightComparison) report(out io.Writer, verbose bool) {
	fl := c.fl
	fmt.Fprintf(out, "%s: %s, %s, %.1f s, %d casts; log %.0f DPS, sim %.0f DPS over %d iterations\n",
		fl.path, fl.setup, fl.player.Name, fl.duration().Seconds(), len(fl.casts), c.logDPS, c.simDPS,
		len(c.sim.samples))
	var passed, dropped int
	for _, check := range c.checks {
		status := "PASS"
		switch {
		case check.Dropped:
			status = "DROP"
			dropped++
		case !check.Passed:
			status = "FAIL"
		default:
			passed++
		}
		if verbose || status != "PASS" {
			fmt.Fprintf(out, "  %s %-44s %-22s %s\n", status, trim(check.Label, 44), check.Name, check.Detail)
		}
	}
	fmt.Fprintf(out, "%d of %d checks passed, %d dropped\n", passed, len(c.checks), dropped)
}

// trimFight cuts an e2e fight log down to what the replay reads: the header, the player's and its minions'
// unit lines, what they did in the fight to themselves or the dummy but a spell's plain HIT, a heal, a summon
// or a cast other than the bot's, and before it only their auras going on and off. It's written gzipped to
// dir as <setup>.log.gz.
func trimFight(rawPath, dir, source string, cycles []fightCycle) (string, error) {
	c, err := fightCycleFor(cycles, fightSetupName(rawPath))
	if err != nil {
		return "", err
	}
	fl, err := readFightLog(rawPath, c, source)
	if err != nil {
		return "", err
	}
	raw, err := readMaybeGzipped(rawPath)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		e, err := parseChronicleEvent(line)
		if err != nil {
			return "", fmt.Errorf("%s: %w", rawPath, err)
		}
		if fl.keeps(e) {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	dest := filepath.Join(dir, c.Setup+".log.gz")
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	if _, err := w.Write(out.Bytes()); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return dest, os.WriteFile(dest, gz.Bytes(), 0o644)
}

func (fl *fightLog) keeps(e chronicleEvent) bool {
	switch e.Type {
	case "CHRONICLE_HEADER", "CHRONICLE_ZONE_INFO":
		return true
	case "CHRONICLE_UNIT_INFO", "CHRONICLE_COMBATANT_INFO":
		return fl.mine[arg(e.Args, 0)]
	}
	if !fl.mine[e.Src.GUID] || e.TimeMs > fl.endMs {
		return false
	}
	switch {
	case e.Type == spellTargetResult && e.missType() == "HIT":
		return false
	case e.Type == "SPELL_HEAL" || e.Type == "SPELL_PERIODIC_HEAL" || e.Type == "SPELL_SUMMON":
		return false
	// only the bot's own casts start, time or end anything the replay reads
	case e.Type == "SPELL_CAST_SUCCESS" && (e.Src.GUID != fl.player.GUID ||
		e.Spell.ID != autoShotSpell && !fl.cycle.casts()[e.Spell.ID]):
		return false
	}
	aura := e.Type == "SPELL_AURA_APPLIED" || e.Type == "SPELL_AURA_REMOVED"
	if (aura || e.isDamage() || e.isMiss()) && !fl.mine[e.Dst.GUID] && e.Dst.GUID != fl.target {
		return false
	}
	if e.TimeMs >= fl.startMs {
		return true
	}
	// what was up when the fight started
	return e.Type == "SPELL_AURA_APPLIED" || e.Type == "SPELL_AURA_REMOVED"
}

func readMaybeGzipped(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasSuffix(path, ".gz") {
		return data, err
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer r.Close()
	return io.ReadAll(r)
}

// loadFight reads a fight log and the setup and cycle it was fought with.
func loadFight(path, source string, setups []goldcharSetup, cycles []fightCycle) (goldcharSetup, *fightLog, error) {
	name := fightSetupName(path)
	i := slices.IndexFunc(setups, func(s goldcharSetup) bool { return s.Name == name })
	if i < 0 {
		return goldcharSetup{}, nil, fmt.Errorf("%s: no golden setup %q", path, name)
	}
	c, err := fightCycleFor(cycles, name)
	if err != nil {
		return goldcharSetup{}, nil, fmt.Errorf("%s: %w", path, err)
	}
	fl, err := readFightLog(path, c, source)
	return setups[i], fl, err
}

func runFight(args []string) int {
	flags := flag.NewFlagSet("simval fight", flag.ExitOnError)
	iterations := flags.Int("iterations", defaultFightIterations, "sim iterations per fight")
	cyclesPath := flags.String("cycles", filepath.Join(fightsDir, fightCyclesFile), "the setups' cycles")
	source := flags.String("source", "", "the player; by default the one the file name or the log names")
	verbose := flags.Bool("v", false, "print passing checks too")
	trimTo := flags.String("trim", "", "write each log trimmed to this directory as <setup>.log.gz instead of replaying it")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: simval fight [flags] <fight log>...\n\n"+
			"Replays a golden setup's fight (TestSimvalFight's <setup>_<player>_<unix>.log, or a committed\n"+
			"%s/<setup>.log.gz) in the sim and compares it ability by ability. Run from the repo root,\n"+
			"built with --tags=with_db.\n\n", fightsDir)
		flags.PrintDefaults()
	}
	_ = flags.Parse(args)
	if flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	cycles, err := readFightCycles(*cyclesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if *trimTo != "" {
		for _, path := range flags.Args() {
			dest, err := trimFight(path, *trimTo, *source, cycles)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			info, _ := os.Stat(dest)
			fmt.Printf("saved %s (%d bytes)\n", dest, info.Size())
		}
		return 0
	}

	if !core.WITH_DB {
		fmt.Fprintln(os.Stderr, "the sim has no item database: go run --tags=with_db ./tools/simval fight <log>")
		return 1
	}
	setups, err := readGoldcharSetups(filepath.Join(fixtureDir, goldcharSetupsFile))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	failed := 0
	for _, path := range flags.Args() {
		s, fl, err := loadFight(path, *source, setups, cycles)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		c, err := replayFight(s, fl, *iterations)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			return 1
		}
		c.applyDrops(droppedFightChecks)
		c.report(os.Stdout, *verbose)
		failed += c.failed()
	}
	if failed > 0 {
		return 1
	}
	return 0
}
