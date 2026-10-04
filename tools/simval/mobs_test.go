package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// droppedMobs are lines of mobsFixture, or checks of them, that disagree with the sim, each with a row in the
// fixture README.
var droppedMobs = slices.Concat(
	// the server's Elixir of Demonslaying gives +105, the sim's 265
	mobDrops("HumanWarrior", "elixirOfDemonslaying", 999005,
		append(mobMinMax("47450 weapon", "47486 weapon"), "white normal damage")...),
	// the server multiplies the set's three auras
	mobDrops("HumanWarrior", "undeadSlaying3", 999004, mobMinMax("47450 weapon", "47486 weapon")...),
	mobDrops("HumanWarrior", "undeadSlaying4", 999004,
		append(mobMinMax("47450 weapon", "47486 weapon"), "white normal damage")...),
	// versus attack power only reaches weapon damage on the server, once, after the weapon percent
	mobDrops("HumanPaladin", "scourgebane", 999004, mobMinMax("48801 e0 damage", "48817 e0 damage", "35395 weapon")...),
	// the server's Predatory Strikes ignores weapon enchants
	mobDrops("NightElfDruid", "scourgebane", 999004, "attack power"),
)

func mobDrops(bot, state string, dummy uint32, checks ...string) []mobDrop {
	drops := make([]mobDrop, len(checks))
	for i, c := range checks {
		drops[i] = mobDrop{bot, state, dummy, c}
	}
	return drops
}

// mobMinMax is each check's min and max end.
func mobMinMax(checks ...string) []string {
	var ends []string
	for _, c := range checks {
		ends = append(ends, c+" min", c+" max")
	}
	return ends
}

// TestMobRecords replays a live TestSimvalMobs run, its MOBS lines: each line's change from the mechanical dummy
// against the sim's. Skips without the capture. Every line is logged on a MOBSIM line.
func TestMobRecords(t *testing.T) {
	needsItemDB(t)
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, fixtureDir, mobsFixture)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalMobs and keep its MOBS lines", file)
	}
	lines, err := readJSONLines[mobLine](file)
	if err != nil {
		t.Fatal(err)
	}
	report := replayMobs(lines, droppedMobs)
	for _, line := range report.logs {
		t.Log(line)
	}
	for _, e := range report.errors {
		t.Error(e)
	}
}

// mobTestState is one of the e2e's states: the auras it puts on the character (talent ranks, glyphs and casts
// included) and the items it wears beyond the weapons.
type mobTestState struct {
	name  string
	auras []int32
	items []int32
}

// mobTestBot is one of TestSimvalMobs' bots and what it probes.
type mobTestBot struct {
	name                           string
	class, race                    uint8
	hands                          []strikeHand
	damage, yellow, spells, threat []int32
	states                         []mobTestState
}

var mobUndeadSet = []int32{43068, 43069, 43070, 43071}

const mobNightElf, mobTroll = 4, 8

var mobTestBots = []mobTestBot{
	{"HumanWarrior", warrior, 1, []strikeHand{{"mainhand", 40384}},
		[]int32{47450, 47486}, []int32{47450}, nil, []int32{44622}, []mobTestState{
			{name: mobsBare},
			{"scourgebane", []int32{44594}, nil},
			{"demonSlaying", []int32{41433}, nil},
			{"elixirOfDemonslaying", []int32{11406}, nil},
			{"undeadSlaying2", []int32{57943}, mobUndeadSet[:2]},
			{"undeadSlaying3", []int32{57943, 29068}, mobUndeadSet[:3]},
			{"undeadSlaying4", []int32{57943, 29068, 57944}, mobUndeadSet},
		}},
	{"HumanPaladin", paladin, 1, []strikeHand{{"mainhand", 40384}},
		[]int32{48801, 48817, 35395}, []int32{48801, 35395}, []int32{48801}, []int32{48817}, []mobTestState{
			{name: mobsBare},
			{"crusade", []int32{31868}, nil},
			{"senseUndead", []int32{57947, 5502}, nil},
			{"scourgebane", []int32{44594}, nil},
		}},
	{"TrollHunter", hunter, mobTroll, []strikeHand{{"mainhand", 40388}, {"ranged", 40385}},
		[]int32{49045, 49050, 48996}, []int32{49045, 49050}, nil, nil, []mobTestState{
			{name: mobsBare},
			{"improvedTracking", []int32{52788, 19883}, nil},
			{"scourgebane", []int32{44594}, nil},
		}},
	{"NightElfDruid", druid, mobNightElf, []strikeHand{{"mainhand", 40388}},
		[]int32{48566, 48572}, []int32{48566}, nil, nil, []mobTestState{
			{mobsBare, []int32{16975, 768}, nil},
			{"scourgebane", []int32{16975, 768, 44594}, nil},
		}},
}

// The replay expects the test bots' states, in order.
func TestMobBotsAreTheTestBots(t *testing.T) {
	var want []mobBot
	for _, b := range mobTestBots {
		var states []string
		for _, s := range b.states {
			states = append(states, s.name)
		}
		bot := mobBot{name: b.name, states: states}
		for _, h := range b.hands {
			if h.kind == "mainhand" {
				bot.mainHand = h.item
			}
		}
		want = append(want, bot)
	}
	if !slices.EqualFunc(mobBots, want, func(a, b mobBot) bool {
		return a.name == b.name && slices.Equal(a.states, b.states) && a.mainHand == b.mainHand
	}) {
		t.Errorf("mobBots %v, the test's %v", mobBots, want)
	}
	for state := range mobMirrors {
		if !slices.ContainsFunc(mobBots, func(b mobBot) bool { return slices.Contains(b.states, state) }) {
			t.Errorf("mirror for %s, which no bot has", state)
		}
	}
}

func mobTestBotNamed(t *testing.T, name string) mobTestBot {
	t.Helper()
	i := slices.IndexFunc(mobTestBots, func(b mobTestBot) bool { return b.name == name })
	if i < 0 {
		t.Fatalf("no test bot %s", name)
	}
	return mobTestBots[i]
}

func (b mobTestBot) state(t *testing.T, name string) mobTestState {
	t.Helper()
	i := slices.IndexFunc(b.states, func(s mobTestState) bool { return s.name == name })
	if i < 0 {
		t.Fatalf("%s has no state %s", b.name, name)
	}
	return b.states[i]
}

const mobTestSpellPower = 520

// mobTestInfo is the info record of the bot in the state at the dummy, with that main-hand and ranged attack
// power.
func mobTestInfo(t *testing.T, b mobTestBot, s mobTestState, dummy uint32, ap, rap float64) damageRecord {
	t.Helper()
	auras := []map[string]any{}
	for _, id := range slices.Concat([]int32{54192, 54161, 47436}, s.auras) {
		auras = append(auras, map[string]any{"id": id, "durationMs": -1})
	}
	weapons := map[string]int32{}
	for _, h := range b.hands {
		weapons[h.kind] = h.item
	}
	// the server's info shows no weapon in cat form
	if slices.Contains(s.auras, 768) {
		delete(weapons, "mainhand")
	}
	var attacks []map[string]any
	for _, kind := range []string{"mainhand", "ranged"} {
		a := map[string]any{"type": kind, "attackPower": ap, "expertiseReduction": 0}
		if kind == "ranged" {
			a["attackPower"] = rap
		}
		if id, ok := weapons[kind]; ok {
			a["weapon"] = map[string]any{"itemId": id}
		}
		attacks = append(attacks, a)
	}
	level, armor := 83, 10643.0
	if dummy == level80 {
		level, armor = 80, 9729
	}
	raw, err := json.Marshal(map[string]any{
		"schema": 1, "command": "info",
		"attacker": map[string]any{"name": "Svmobabcde", "level": 80, "isPlayer": true, "class": b.class, "race": b.race,
			"levelForOther": 80, "attacks": attacks, "auras": auras},
		"target": map[string]any{"name": "SimVal Dummy", "entry": dummy, "level": level, "levelForOther": level,
			"worldBoss": false, "creatureType": mobDummies[dummy], "otherInFront": true,
			"resistances": []float64{armor, 0, 0, 0, 0, 0, 0}, "auras": []any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var info damageRecord
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	return info
}

// mobTestProbe is the probe's damage record before its amounts: a weapon part with the strike's fixed bonus and
// percent from serverdata, or the spell's probed effects.
func mobTestProbe(t *testing.T, id int32, ap, rap float64) damageRecord {
	t.Helper()
	s := serverdata.SpellByID(id)
	if s == nil {
		t.Fatalf("spell %d isn't in serverdata", id)
	}
	var rec damageRecord
	rec.Command, rec.Spell.ID, rec.Spell.Name = "damage", id, fmt.Sprintf("spell %d", id)
	rec.SpellPower.Damage, rec.SpellPower.Healing = mobTestSpellPower, mobTestSpellPower
	rec.AttackPower.Type, rec.AttackPower.Value = "mainhand", ap
	if s.DmgClass == serverdata.DmgClassRanged {
		rec.AttackPower.Type, rec.AttackPower.Value = "ranged", rap
	}
	fixed, percent, normalized, weapon := 0.0, 100.0, false, false
	for i, e := range s.Effects {
		lo, hi := s.EffectRange(i)
		switch {
		case strikeWeaponEffects[e.Effect]:
			weapon = true
			if e.Effect == 31 {
				percent = percent * float64(lo) / 100
			} else {
				fixed += float64(lo)
				normalized = normalized || e.Effect == 121
			}
			rec.Effects = append(rec.Effects, damageEffect{Index: int32(i), Effect: e.Effect, Kind: "unprobed",
				Base: rng(float64(lo), float64(hi))})
		case spellModProbedKind(e) != "":
			rec.Effects = append(rec.Effects, damageEffect{Index: int32(i), Effect: e.Effect, Kind: spellModProbedKind(e)})
		}
	}
	if weapon {
		rec.Shaped = "no direct, periodic or heal effect to probe"
		rec.Effects = slices.DeleteFunc(rec.Effects, func(e damageEffect) bool { return e.Kind != "unprobed" })
		rec.Weapon = &weaponStrike{AttackType: rec.AttackPower.Type, Physical: s.SchoolMask == 1, Normalized: normalized,
			AttackPower: rec.AttackPower.Value, TotalPct: 1, Fixed: rng(fixed, fixed), Percent: rng(percent, percent)}
	}
	return rec
}

// mobLineFrom is the MOBS line a server agreeing with the sim would log for the bot in the state at the dummy:
// the sim's own attack power, its amounts rounded, its crit chances, parry and mean plain swing.
func mobLineFrom(t *testing.T, b mobTestBot, s mobTestState, dummy uint32) mobLine {
	t.Helper()
	build := func(ap, rap float64) mobLine {
		l := mobLine{Bot: b.name, Dummy: dummy, CreatureType: mobDummies[dummy], State: s.name, Items: s.items,
			Info: mobTestInfo(t, b, s, dummy, ap, rap)}
		for _, id := range b.damage {
			l.Damage = append(l.Damage, mobTestProbe(t, id, ap, rap))
		}
		return l
	}
	l := build(1000, 900)
	probe, _ := l.characterProbe()
	first := newMobReplay([]mobLine{l}).sim(l, probe)
	if first.err != nil {
		t.Fatalf("%s %s on %d: %v", b.name, s.name, dummy, first.err)
	}
	ap, errAP := first.d.mobStat(stats.AttackPower)
	rap, errRAP := first.d.mobStat(stats.RangedAttackPower)
	if err := firstErr(errAP, errRAP); err != nil {
		t.Fatal(err)
	}
	l = build(math.Floor(ap+1e-6), math.Floor(rap+1e-6))
	m := newMobReplay([]mobLine{l})
	label := fmt.Sprintf("%s %s on %d", b.name, s.name, dummy)

	for i, rec := range l.Damage {
		built := m.sim(l, rec)
		if built.err != nil {
			t.Fatalf("%s: %d: %v", label, rec.Spell.ID, built.err)
		}
		if w := rec.Weapon; w != nil {
			lo, hi, _, _, err := built.strikeAmounts(w)
			if err != nil {
				t.Fatalf("%s: %d: %v", label, rec.Spell.ID, err)
			}
			w.Base, w.Done, w.Taken = rng(lo, hi), rng(lo, hi), rng(math.Round(lo), math.Round(hi))
			continue
		}
		ts := &timingSim{damageSim: built.d}
		for j, e := range rec.Effects {
			decl := built.d.declaration(e)
			if decl == nil {
				t.Fatalf("%s: %d e%d: no sim declaration", label, rec.Spell.ID, e.Index)
			}
			declared := decl.effect()
			e.Base, e.Coefficient = rng(declared.Min, declared.Max), declared.SP
			if amounts := ts.spellModEffect(e); amounts.err == nil {
				e.Taken = rng(math.Round(amounts.lo), math.Round(amounts.hi))
			} else {
				// Holy Wrath off its types: the sim lands nothing, the replay leaves it out
				e.Taken = e.Base
			}
			l.Damage[i].Effects[j] = e
		}
	}

	hit := func(id int32) buffYellow {
		rec, ok := l.damageOf(id)
		if !ok {
			t.Fatalf("%s: no damage probe of %d", label, id)
		}
		weapon := spellModWeapon(id)
		crit, err := m.crit(l, rec, weapon)
		if err != nil {
			t.Fatalf("%s: %d: %v", label, id, err)
		}
		y := buffYellow{Derived: spellDerived{CritDone: min(100, crit), CritTaken: min(100, crit)}}
		y.Spell.ID = id
		if weapon {
			y.Derived.Yellow = &yellowTable{}
		} else {
			y.Derived.Magic = &magicTable{}
		}
		return y
	}
	for _, id := range b.yellow {
		l.Yellow = append(l.Yellow, hit(id))
	}

	l.Melee = &mobMelee{}
	if character := m.sim(l, probe); character.d.char.AutoAttacks.MHAuto() != nil {
		parry, err := character.d.mobParry(0)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		mean, _, err := character.d.mobWhiteMean(l, 30000)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		l.Melee = &mobMelee{Derived: whiteTable{ParryBp: parry}, Outcomes: map[string]counted{
			"normal": {Count: 30000, MeanDamage: mean}}}
	}

	if s.name != mobsBare {
		return l
	}
	for _, id := range b.spells {
		l.Spell = append(l.Spell, hit(id))
	}
	for _, id := range b.threat {
		selected, err := mobSelections[id](m, l)
		if err != nil {
			t.Fatalf("%s: %d: %v", label, id, err)
		}
		var tr threatRecord
		tr.Command, tr.Spell.ID, tr.Bonus.Selected.Dummy = "threat", id, selected
		l.Threat = append(l.Threat, tr)
	}
	return l
}

var (
	mobTestLinesMu sync.Mutex
	mobTestLines   = map[mobKey]mobLine{}
)

// mobTestLine is mobLineFrom, made once per key; the caller gets its own copy to change.
func mobTestLine(t *testing.T, bot, state string, dummy uint32) mobLine {
	t.Helper()
	k := mobKey{bot, state, dummy}
	mobTestLinesMu.Lock()
	defer mobTestLinesMu.Unlock()
	l, ok := mobTestLines[k]
	if !ok {
		b := mobTestBotNamed(t, bot)
		l = mobLineFrom(t, b, b.state(t, state), dummy)
		mobTestLines[k] = l
	}
	return cloneMobLine(t, l)
}

func cloneMobLine(t *testing.T, l mobLine) mobLine {
	t.Helper()
	raw, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var out mobLine
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// mobTestRun is every line of the run, as a server agreeing with the sim would log it.
func mobTestRun(t *testing.T) []mobLine {
	t.Helper()
	var lines []mobLine
	for _, b := range mobTestBots {
		for _, s := range b.states {
			for _, entry := range slices.Sorted(maps.Keys(mobDummies)) {
				lines = append(lines, mobTestLine(t, b.name, s.name, entry))
			}
		}
	}
	return lines
}

// A server that agrees with the sim agrees on every line, which checks the units, the line's fields and the sim
// side's numbers.
func TestMobReplayAgreesWithTheSim(t *testing.T) {
	needsItemDB(t)
	report := replayMobs(mobTestRun(t), nil)
	for _, line := range report.logs {
		t.Log(line)
	}
	for _, e := range report.errors {
		t.Error(e)
	}
	if report.counts[spellModDisagrees] > 0 || report.counts[spellModAgrees] == 0 {
		t.Errorf("classes %v; want none to disagree", report.counts)
	}
}

// The sim lands the type-only probes where the server's Spell.dbc lets it: Holy Wrath (TargetCreatureType 36) on
// demons and undead, Tendon Rip (16) on giants.
func TestMobSelectionsFollowTheServerTypes(t *testing.T) {
	needsItemDB(t)
	for _, c := range []struct {
		bot   string
		spell int32
		types []uint32
	}{
		{"HumanPaladin", 48817, []uint32{mobDemon, mobUndead}},
		{"HumanWarrior", 44622, []uint32{mobGiant}},
	} {
		for _, entry := range slices.Sorted(maps.Keys(mobDummies)) {
			l := mobTestLine(t, c.bot, mobsBare, entry)
			got, err := mobSelections[c.spell](newMobReplay([]mobLine{l}), l)
			if want := slices.Contains(c.types, mobDummies[entry]); err != nil || got != want {
				t.Errorf("%d on %d: lands %v, want %v (%v)", c.spell, entry, got, want, err)
			}
		}
	}
}

// failedMobChecks runs the line's comparison against the other lines and gives its failed checks' names.
func failedMobChecks(t *testing.T, target mobLine, others ...mobLine) []string {
	t.Helper()
	m := newMobReplay(append(others, target))
	result := m.compare(target)
	if len(result.checks) == 0 {
		t.Errorf("%s: no checks; not compared: %v", target.key(), result.skipped)
	}
	var failed []string
	for _, c := range result.checks {
		t.Logf("%s: %v %s", c.Name, c.Passed, c.Detail)
		if !c.Passed {
			failed = append(failed, c.Name)
		}
	}
	slices.Sort(failed)
	return failed
}

// mobTaken moves each end of the probe's strike or first probed effect on the line by the function of the same end
// on the reference line.
func mobTaken(t *testing.T, l, ref *mobLine, spell int32, at func(ref float64) float64) {
	t.Helper()
	rec, ok := l.damageOf(spell)
	refRec, okRef := ref.damageOf(spell)
	if !ok || !okRef {
		t.Fatalf("no probe of %d", spell)
	}
	if rec.Weapon != nil {
		rec.Weapon.Taken = rng(at(refRec.Weapon.Taken.Min), at(refRec.Weapon.Taken.Max))
		return
	}
	for j, e := range rec.Effects {
		if e.probed() {
			rec.Effects[j].Taken = rng(at(refRec.Effects[j].Taken.Min), at(refRec.Effects[j].Taken.Max))
			return
		}
	}
	t.Fatalf("no probed effect of %d", spell)
}

// Each part of a line's comparison catches a server number that moved differently from the sim's, and the
// server's own arithmetic for a versus attack power bonus agrees where the sim follows it: MeleeDamageBonusDone
// adds int(bonus / 14 × the weapon's speed, or the normalized one) after the weapon percent.
func TestMobReplayCatchesMismatches(t *testing.T) {
	needsItemDB(t)
	const undead, demon, mechanical = 999004, 999005, 999008
	pair := func(bot, state string, dummy uint32) (mobLine, mobLine) {
		return mobTestLine(t, bot, state, dummy), mobTestLine(t, bot, state, mechanical)
	}
	plus := func(points float64) func(float64) float64 { return func(v float64) float64 { return v + points } }
	for _, c := range []struct {
		name   string
		line   func() []mobLine
		failed []string
	}{
		{name: "agreeing", line: func() []mobLine {
			l, ref := pair("HumanWarrior", "scourgebane", undead)
			return []mobLine{l, ref}
		}},
		{name: "Heroic Strike's Scourgebane at the server's int(140 / 14 × 3.4)", line: func() []mobLine {
			l, ref := pair("HumanWarrior", "scourgebane", undead)
			mobTaken(t, &l, &ref, 47450, plus(34))
			return []mobLine{l, ref}
		}},
		{name: "Mortal Strike's Scourgebane at the server's int(140 / 14 × 3.3), normalized", line: func() []mobLine {
			l, ref := pair("HumanWarrior", "scourgebane", undead)
			mobTaken(t, &l, &ref, 47486, plus(33))
			return []mobLine{l, ref}
		}},
		{name: "Elixir of Demonslaying at the server's 105", line: func() []mobLine {
			l, ref := pair("HumanWarrior", "elixirOfDemonslaying", demon)
			mobTaken(t, &l, &ref, 47450, plus(25))
			return []mobLine{l, ref}
		}, failed: []string{"47450 weapon max", "47450 weapon min"}},
		{name: "the Blessed set's three auras multiplied", line: func() []mobLine {
			l, ref := pair("HumanWarrior", "undeadSlaying4", undead)
			mobTaken(t, &l, &ref, 47450, func(v float64) float64 { return math.Floor(v * 1.01 * 1.02 * 1.03) })
			return []mobLine{l, ref}
		}, failed: []string{"47450 weapon max", "47450 weapon min"}},
		{name: "Crusader Strike's Scourgebane after its 75%", line: func() []mobLine {
			l, ref := pair("HumanPaladin", "scourgebane", undead)
			mobTaken(t, &l, &ref, 35395, plus(33))
			return []mobLine{l, ref}
		}, failed: []string{"35395 weapon max", "35395 weapon min"}},
		{name: "Exorcism without the versus attack power", line: func() []mobLine {
			l, ref := pair("HumanPaladin", "scourgebane", undead)
			mobTaken(t, &l, &ref, 48801, plus(0))
			return []mobLine{l, ref}
		}, failed: []string{"48801 e0 damage max", "48801 e0 damage min"}},
		{name: "Holy Wrath without the versus attack power, held to the demon's", line: func() []mobLine {
			l := mobTestLine(t, "HumanPaladin", "scourgebane", undead)
			ref := mobTestLine(t, "HumanPaladin", "scourgebane", demon)
			mobTaken(t, &l, &ref, 48817, plus(0))
			return []mobLine{l, ref, mobTestLine(t, "HumanPaladin", "scourgebane", mechanical)}
		}, failed: []string{"48817 e0 damage max", "48817 e0 damage min"}},
		{name: "Predatory Strikes ignoring the enchant", line: func() []mobLine {
			l, ref := pair("NightElfDruid", "scourgebane", undead)
			for i, a := range l.Info.Attacker.Attacks {
				if a.Type == "mainhand" {
					l.Info.Attacker.Attacks[i].AttackPower = ref.Info.Attacker.attack("mainhand").AttackPower
				}
			}
			return []mobLine{l, ref}
		}, failed: []string{"attack power"}},
		{name: "a parrying beast", line: func() []mobLine {
			l, ref := pair("HumanWarrior", mobsBare, 999002)
			l.Melee.Derived.ParryBp = 500
			return []mobLine{l, ref}
		}, failed: []string{"white parry"}},
		{name: "plain swings 2% up", line: func() []mobLine {
			l, ref := pair("HumanWarrior", mobsBare, 999002)
			l.Melee.Outcomes["normal"] = counted{Count: 30000, MeanDamage: ref.Melee.Outcomes["normal"].MeanDamage * 1.02}
			return []mobLine{l, ref}
		}, failed: []string{"white normal damage"}},
		{name: "Exorcism's crit on undead short of certain", line: func() []mobLine {
			l, ref := pair("HumanPaladin", mobsBare, undead)
			l.Yellow[0].Derived.CritTaken = 50
			return []mobLine{l, ref}
		}, failed: []string{"48801 crit taken"}},
		{name: "Holy Wrath selecting a humanoid", line: func() []mobLine {
			l := mobTestLine(t, "HumanPaladin", mobsBare, 999001)
			l.Threat[0].Bonus.Selected.Dummy = true
			return []mobLine{l, mobTestLine(t, "HumanPaladin", mobsBare, mechanical)}
		}, failed: []string{"48817 selects the dummy"}},
		{name: "an aura missing", line: func() []mobLine {
			l, ref := pair("HumanWarrior", "scourgebane", undead)
			l.Missing = []int32{44594}
			return []mobLine{l, ref}
		}, failed: []string{"server auras"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			lines := c.line()
			if failed := failedMobChecks(t, lines[0], lines[1:]...); !slices.Equal(failed, c.failed) {
				t.Errorf("failed %v, want %v", failed, c.failed)
			}
		})
	}
}

// A plain swing's server mean off by less than its sampling bounds moves nothing.
func TestMobWhiteNoiseMovesNothing(t *testing.T) {
	needsItemDB(t)
	l, ref := mobTestLine(t, "HumanWarrior", mobsBare, 999002), mobTestLine(t, "HumanWarrior", mobsBare, 999008)
	normal := l.Melee.Outcomes["normal"]
	normal.MeanDamage *= 1.0005
	l.Melee.Outcomes["normal"] = normal
	if r := newMobReplay([]mobLine{l, ref}).compare(l); r.class != spellModMovesNothing {
		t.Errorf("class %s, want %s; moved %v", r.class, spellModMovesNothing, r.moved)
	}
}

// The replay holds a run to every line and its drops to what fails.
func TestMobReplayDrops(t *testing.T) {
	needsItemDB(t)
	const bot, state, dummy, mechanical = "HumanWarrior", "scourgebane", 999004, 999008
	ref := mobTestLine(t, bot, state, mechanical)
	agreeing := mobTestLine(t, bot, state, dummy)
	wrong := mobTestLine(t, bot, state, dummy)
	wrong.Missing = []int32{44594}
	// every other line dropped, so only these count
	var others []mobDrop
	for _, b := range mobBots {
		for _, s := range b.states {
			for entry := range mobDummies {
				if k := (mobKey{b.name, s, entry}); k != ref.key() && k != agreeing.key() {
					others = append(others, mobDrop{k.bot, k.state, k.dummy, ""})
				}
			}
		}
	}
	replay := func(l mobLine, drops ...mobDrop) spellModReport {
		return replayMobs([]mobLine{ref, l}, append(slices.Clone(others), drops...))
	}
	one := func(name string, r spellModReport, want string) {
		t.Helper()
		if len(r.errors) != 1 || !strings.Contains(r.errors[0], want) {
			t.Errorf("%s: %v, want one error with %q", name, r.errors, want)
		}
	}

	if r := replay(agreeing); len(r.errors) > 0 {
		t.Errorf("agreeing: %v", r.errors)
	}
	one("wrong", replay(wrong), "server auras")
	if r := replay(wrong, mobDrop{bot, state, dummy, "server auras"}); len(r.errors) > 0 {
		t.Errorf("dropped check: %v", r.errors)
	}
	if r := replay(wrong, mobDrop{bot, state, dummy, ""}); len(r.errors) > 0 {
		t.Errorf("dropped line: %v", r.errors)
	}
	one("agreeing line dropped", replay(agreeing, mobDrop{bot, state, dummy, ""}), "shouldn't be dropped")
	one("passing check dropped", replay(agreeing, mobDrop{bot, state, dummy, "47450 weapon min"}), "passes")
	one("stale drop", replay(agreeing, mobDrop{bot, "noSuchState", dummy, ""}), "no line matches")
	one("missing line", replayMobs([]mobLine{ref}, others), "no line for")
	twice := replayMobs([]mobLine{ref, agreeing, agreeing}, others)
	one("line twice", twice, "twice")
	moved := agreeing
	moved.CreatureType = mobDemon
	one("type off its dummy", replayMobs([]mobLine{ref, moved}, append(slices.Clone(others),
		mobDrop{bot, state, dummy, ""})), "the line says type")
}

// mobsPaladinUndead is a MOBS line as the e2e logs it, cut down to the fields the replay reads.
const mobsPaladinUndead = `{"bot":"HumanPaladin","dummy":999004,"creatureType":6,"state":"bare",` +
	`"auras":[54192,54161,47436],` +
	`"info":{"schema":1,"command":"info","attacker":{"name":"Svmobxyzab","level":80,"class":2,"race":1,` +
	`"isPlayer":true,"levelForOther":80,"resistances":[1630,0,0,0,0,0,0],` +
	`"attacks":[{"type":"mainhand","unhastedMs":3400,"attackPower":1298,"expertiseReduction":0.75,` +
	`"weapon":{"itemId":40384}},{"type":"ranged","attackPower":856}],` +
	`"auras":[{"id":54192,"durationMs":-1},{"id":54161,"durationMs":-1},{"id":47436,"durationMs":120000}]},` +
	`"target":{"name":"SimVal Undead Dummy","entry":999004,"level":83,"levelForOther":83,"worldBoss":false,` +
	`"creatureType":6,"otherInFront":true,"resistances":[10643,0,0,0,0,0,0],"auras":[]}},` +
	`"damage":[{"command":"damage","spell":{"id":48801,"name":"Exorcism"},"shaped":"","comboPoints":0,` +
	`"spellPower":{"damage":520,"healing":520},"attackPower":{"type":"mainhand","value":1298},` +
	`"bonus":{"direct":0.15,"dot":0,"ap":0.15,"apDot":0},"effects":[{"index":0,"effect":2,"kind":"damage",` +
	`"coefficient":0.15,"base":{"min":1033,"max":1151},"taken":{"min":1305,"max":1423},` +
	`"crit":{"amount":10000,"result":15000}}]},` +
	`{"command":"damage","spell":{"id":35395,"name":"Crusader Strike"},"shaped":"no direct, periodic or heal effect to probe",` +
	`"effects":[{"index":0,"effect":31,"kind":"unprobed","base":{"min":75,"max":75}}],` +
	`"weapon":{"index":0,"shaped":"","attackType":"mainhand","physical":true,"normalized":true,"apMultiplier":3.3,` +
	`"attackPower":1298,"totalPct":1,"weaponDamage":{"min":912,"max":1216},"fixed":{"min":0,"max":0},` +
	`"percent":{"min":75,"max":75},"base":{"min":684,"max":912},"done":{"min":684,"max":912},"taken":{"min":684,"max":912},` +
	`"crit":{"amount":10000,"result":20000}}}],` +
	`"yellow":[{"command":"yellow","spell":{"id":48801},"derived":{"magic":{"autoHit":false,"missThreshold":1700},` +
	`"critDone":5.3,"critTaken":100}}],` +
	`"melee":{"command":"melee","attackType":"mainhand","iterations":50000,"derived":{"missBp":800,"dodgeBp":575,` +
	`"parryBp":0,"blockBp":500,"glancingBp":2400,"crushingBp":0,"critBp":480,"skillBonus":0},` +
	`"outcomes":{"normal":{"count":21000,"rate":42,"meanDamage":612.5,"meanBlocked":0}}},` +
	`"spell":[{"command":"spell","spell":{"id":48801},"iterations":20000,"derived":{"critDone":5.3,"critTaken":100},` +
	`"crit":20000}],` +
	`"threat":[{"command":"threat","spell":{"id":48817,"name":"Holy Wrath"},"bonus":{"selected":{"count":1,` +
	`"dummy":true,"caster":false},"threat":0}}]}`

// A line as the e2e logs it reads into the replay's numbers.
func TestMobLineShape(t *testing.T) {
	file := filepath.Join(t.TempDir(), mobsFixture)
	if err := os.WriteFile(file, []byte(mobsPaladinUndead+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := readJSONLines[mobLine](file)
	if err != nil || len(lines) != 1 {
		t.Fatalf("%d lines, %v", len(lines), err)
	}
	l := lines[0]
	if l.key() != (mobKey{"HumanPaladin", mobsBare, 999004}) || l.CreatureType != mobUndead ||
		l.Info.Target.CreatureType != mobUndead || l.Info.Attacker.Class != paladin {
		t.Errorf("key %v, type %d, target %+v", l.key(), l.CreatureType, l.Info.Target)
	}
	if a := l.Info.Attacker.attack("mainhand"); a.AttackPower != 1298 || a.ExpertiseReduction != 0.75 || a.Weapon.ItemID != 40384 {
		t.Errorf("main hand %+v", a)
	}
	exo, ok := l.damageOf(48801)
	if !ok || exo.Effects[0].Taken != rng(1305, 1423) || exo.Weapon != nil {
		t.Errorf("Exorcism %+v", exo)
	}
	cs, ok := l.damageOf(35395)
	if !ok || cs.Weapon == nil || cs.Weapon.Taken != rng(684, 912) || cs.Weapon.Percent.Min != 75 {
		t.Errorf("Crusader Strike %+v", cs)
	}
	if probe, _ := l.characterProbe(); probe.Spell.ID != 35395 {
		t.Errorf("character probe %d, want the strike", probe.Spell.ID)
	}
	if len(l.Yellow) != 1 || l.Yellow[0].Derived.CritTaken != 100 || l.Yellow[0].Derived.Magic == nil {
		t.Errorf("yellow %+v", l.Yellow)
	}
	if l.Melee == nil || l.Melee.Derived.ParryBp != 0 || l.Melee.Outcomes["normal"].MeanDamage != 612.5 {
		t.Errorf("melee %+v", l.Melee)
	}
	if len(l.Spell) != 1 || l.Spell[0].Derived.CritTaken != 100 {
		t.Errorf("spell %+v", l.Spell)
	}
	if len(l.Threat) != 1 || l.Threat[0].Spell.ID != 48817 || !l.Threat[0].Bonus.Selected.Dummy {
		t.Errorf("threat %+v", l.Threat)
	}
}
