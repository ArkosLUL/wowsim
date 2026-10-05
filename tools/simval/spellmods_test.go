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
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// droppedSpellMods are spell steps of spellModFixture, or checks of them, that disagree with the sim, each
// with a row in the fixture README. The lines stay, since a spell's after probes are the next talent's
// before probes.
var droppedSpellMods = slices.Concat(
	// the sim has no Improved Healing Wave (a filler step of the path), so Healing Wave's 2600 ms is 3000 there
	spellModDrops("ShamanRestoration", "naturesSwiftness", "cast time", 49273),
	spellModDrops("ShamanRestoration", "naturesSwiftness", "cast time before haste", 49273),
	// not sim gaps: under Berserk the sim resets Mangle (Bear)'s cooldown after each cast, and Dispersion
	// never rolls to hit, so the sim's Shadow Focus hit leaves it out
	spellModDrops("DruidFeralCat", "berserk", "cooldown", 48564),
	spellModDrops("PriestShadow", "shadowFocus", "miss", 47585),
	// the server takes a percent cost mod off the cost after the flat ones, the sim before them
	spellModDrops("DruidFeralCat", "berserk", "cost", 48566, 48572),
)

func spellModDrops(path, talent, check string, spells ...int32) []spellModDrop {
	drops := make([]spellModDrop, len(spells))
	for i, s := range spells {
		drops[i] = spellModDrop{path, talent, s, check}
	}
	return drops
}

// TestSpellModRecords replays a live TestSimvalSpellModSweep run, its SPELLMOD lines: each probed number's
// change across a spell-mod talent against the sim's. Skips without the capture. Every spell step is
// logged on a SPELLMODSIM line.
func TestSpellModRecords(t *testing.T) {
	needsItemDB(t)
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, fixtureDir, spellModFixture)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalSpellModSweep and keep its SPELLMOD lines", file)
	}
	raw, err := readJSONLines[spellModLine](file)
	if err != nil {
		t.Fatal(err)
	}
	lines, notes, err := latestSpellModLines(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range notes {
		t.Log(note)
	}
	report := replaySpellMods(allTalentPaths(t), spellModSteps(t), lines, droppedSpellMods)
	for _, line := range report.logs {
		t.Log(line)
	}
	for _, e := range report.errors {
		t.Error(e)
	}
}

func spellModSteps(t *testing.T) map[int32]spellModStep {
	t.Helper()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	steps, err := readSpellModList(filepath.Join(root, fixtureDir, spellModSpellsFile))
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

// spellModWeapon is whether the server rolls the spell on the yellow table: a melee or ranged class spell.
func spellModWeapon(spellID int32) bool {
	s := serverdata.SpellByID(spellID)
	return s != nil && (s.DmgClass == serverdata.DmgClassMelee || s.DmgClass == serverdata.DmgClassRanged)
}

// serverPowerType is the server's power type of the sim character's bar a spell costs.
func serverPowerType(ts *timingSim) uint32 {
	switch {
	case ts.char.HasRunicPowerBar():
		return 6
	case ts.char.HasEnergyBar():
		return 3
	case ts.char.HasRageBar():
		return 1
	}
	return 0
}

// spellModProbedKind is the kind `.simval damage` gives an effect, empty for one it doesn't probe: weapon
// damage among them.
func spellModProbedKind(e serverdata.Effect) string {
	switch {
	case e.Effect == 2 || e.Effect == 9:
		return "damage"
	case e.Effect == 10:
		return "heal"
	case e.Aura == 3 || e.Aura == 53:
		return "dot"
	case e.Aura == 8:
		return "hot"
	}
	return ""
}

// spellModLineFrom is the SPELLMOD line a server agreeing with the sim would log for the spell after the
// path's first steps, as spec: the sim's numbers in the server's units, its amounts rounded. Without the
// spell in that spec the line has no probes.
func spellModLineFrom(t *testing.T, sims *spellModSims, steps int, cast, spellID int32, spec, teach string) spellModLine {
	t.Helper()
	path := sims.path
	line := spellModLine{Path: path.Name, Bot: "Svsleadabcde", Steps: steps, Cast: cast, Spell: spellID, Name: "Spell"}
	sim := sims.get(steps, cast, spec, teach)
	if sim.err != nil {
		t.Fatalf("%s after %d steps as %s: %v", path.Name, steps, spec, sim.err)
	}
	spell := spellBySpell(&sim.d.char.Unit, spellID)
	if spell == nil {
		return line
	}
	ts := &timingSim{damageSim: sim.d, spell: spell}

	rec := &line.Timing
	rec.Cost.PowerType = serverPowerType(ts)
	if s := serverdata.SpellByID(spellID); s != nil {
		rec.Aura.OnCaster = s.Flags&serverdata.FlagPositive != 0
	}
	if dot := ts.dot(rec.Aura.OnCaster); dot != nil {
		rec.Aura.Effects = []timingEffect{{Index: dot.Tick.Effect, Periodic: true}}
	}
	// any base duration, so an aura that isn't a dot gives its duration too
	rec.Aura.BaseMs = 1
	f := ts.spellModTiming(*rec)
	rec.Cast.AfterSpellModsMs, rec.Cast.Ms, rec.Cast.Haste = int32(f["cast time before haste"]), int32(f["cast time"]), "cast"
	rec.GCD.Category, rec.GCD.Ms = 133, int32(f["gcd"])
	rec.Cooldown = &struct {
		StartsOnEvent bool  `json:"startsOnEvent"`
		Ms            int32 `json:"ms"`
		CategoryMs    int32 `json:"categoryMs"`
	}{Ms: int32(f["cooldown"]), CategoryMs: int32(f["category cooldown"])}
	if cost, ok := f["cost"]; ok {
		rec.Cost.Amount = int32(cost)
	} else {
		rec.Cost.Amount = int32(f["runic power cost"])
		rec.Cost.Runes = &struct {
			Cost           []int32 `json:"cost"`
			RunicPowerGain int32   `json:"runicPowerGain"`
			BaseCooldownMs []int32 `json:"baseCooldownMs"`
		}{Cost: []int32{int32(f["blood rune cost"]), int32(f["unholy rune cost"]), int32(f["frost rune cost"])},
			RunicPowerGain: int32(f["runic power gain"])}
	}
	if duration, ok := f["aura duration"]; ok {
		rec.Aura.Applies, rec.Aura.BaseMs, rec.Aura.Ms = true, 1, int32(duration)
		for i := range rec.Aura.Effects {
			e := &rec.Aura.Effects[i]
			e.AmplitudeMs = int32(f[fmt.Sprintf("e%d amplitude", e.Index)])
			e.Ticks = int32(f[fmt.Sprintf("e%d ticks", e.Index)])
		}
	} else {
		rec.Aura.Effects = nil
	}

	ts.start()
	c := &ts.char.Unit
	dr := &line.Damage
	dr.SpellPower.Damage = math.Floor(c.GetStat(stats.SpellPower))
	dr.SpellPower.Healing = dr.SpellPower.Damage
	dr.AttackPower.Type, dr.AttackPower.Value = "mainhand", math.Floor(c.GetStat(stats.AttackPower))
	dr.Attacker.Attacks = []attackSnapshot{{Type: "mainhand", AttackPower: dr.AttackPower.Value},
		{Type: "ranged", AttackPower: math.Floor(c.GetStat(stats.RangedAttackPower))}}
	if s := serverdata.SpellByID(spellID); s != nil {
		for i := range s.Effects {
			e := damageEffect{Index: int32(i), Kind: spellModProbedKind(s.Effects[i])}
			ts.useDamageRecord(path, line)
			decl := ts.declaration(e)
			if e.Kind == "" || decl == nil {
				continue
			}
			declared := decl.effect()
			e.Base = damageRange{Min: declared.Min, Max: declared.Max}
			e.Coefficient = declared.SP
			// an amount the sim can't work out (Holy Wrath on a giant) is left out of the comparison
			m := ts.spellModEffect(e)
			e.Taken = damageRange{Min: math.Round(m.lo), Max: math.Round(m.hi)}
			if m.crits {
				e.Crit.Amount, e.Crit.Result = 10000, math.Round(10000*m.crit)
			}
			dr.Effects = append(dr.Effects, e)
		}
	}

	weapon := spellModWeapon(spellID)
	y := ts.spellModYellow(path, line, weapon)
	line.Yellow.Derived.CritDone = y["crit chance"]
	if weapon {
		line.Yellow.Derived.Yellow = &yellowTable{MissBp: int32(math.Round(800 + y["miss"]))}
	} else {
		line.Yellow.Derived.Magic = &magicTable{MissThreshold: int32(math.Round(y["miss"])) + 1}
	}
	return line
}

// A server that agrees with the sim agrees on every listed spell of every spell-mod step the sim can
// build, which checks the units, the record fields and the sim side's numbers.
func TestSpellModAgreesWithTheSim(t *testing.T) {
	needsItemDB(t)
	steps := spellModSteps(t)
	counts := map[string]int{}
	for _, path := range allTalentPaths(t) {
		sims := newSpellModSims(path)
		for n, step := range path.Steps {
			listed, ok := steps[step.Spell]
			if !ok {
				continue
			}
			var cast int32
			if !listed.passive {
				cast = step.Spell
			}
			for _, s := range listed.group.Spells {
				pair := sims.pair(n+1, cast, s.ID)
				if pair.err != nil {
					t.Fatalf("%s step %d: %v", path.Name, n+1, pair.err)
				}
				before, after := spellModLine{}, spellModLine{}
				if pair.spec != "" {
					before = spellModLineFrom(t, sims, n, 0, s.ID, pair.spec, pair.teach)
					after = spellModLineFrom(t, sims, n+1, cast, s.ID, pair.spec, pair.teach)
				}
				result := compareSpellMod(path, before, after, pair)
				counts[result.class]++
				if result.class == spellModDisagrees {
					t.Errorf("%s %s %d %s%s: %s", path.Name, step.Talent, s.ID, s.Name, pair.label(path),
						strings.Join(failedChecks(result.checks), "; "))
				}
			}
		}
	}
	if counts[spellModAgrees] == 0 || counts[spellModMovesNothing] == 0 || counts[spellModNotCompared] == 0 {
		t.Errorf("classes %v; want spells that agree, move nothing and aren't compared", counts)
	}
	t.Logf("classes %v", counts)
}

// spellModFrostbolt is a SPELLMOD line as the e2e logs it, made from live Frostbolt records of a mage.
const spellModFrostbolt = `{"path":"MageFrost","bot":"Svsleadabcde","steps":3,"spell":42842,"name":"Frostbolt",` +
	`"timing":{"cast":{"baseMs":3000,"afterSpellModsMs":3000,"ms":3000,"haste":"cast","castSpeed":1,"rangedSpeed":1},` +
	`"gcd":{"baseMs":1500,"category":133,"ms":1500,"hasted":true,"cheatCooldown":false},` +
	`"cooldown":{"category":0,"baseMs":0,"baseCategoryMs":0,"startsOnEvent":false,"ms":0,"categoryMs":0},` +
	`"cost":{"powerType":0,"base":0,"percent":11,"amount":359},` +
	`"aura":{"applies":true,"onCaster":false,"baseMs":9000,"maxMs":9000,"comboPoints":0,"immune":false,` +
	`"afterSpellModsMs":9000,"diminishGroup":0,"diminishLevel":0,"diminishMod":1,"afterModsMs":9000,` +
	`"periodicHaste":false,"ms":9000,"effects":[{"index":2,"auraType":118,"periodic":false,"amplitudeMs":0,"ticks":1}]}},` +
	`"damage":{"shaped":"","comboPoints":0,"spellPower":{"damage":520,"healing":520},` +
	`"attackPower":{"type":"mainhand","value":802},"bonus":{"direct":0.857,"dot":0,"ap":0,"apDot":0},` +
	`"effects":[{"index":0,"effect":6,"aura":33,"kind":"other"},{"index":1,"effect":2,"aura":0,"kind":"damage",` +
	`"victim":"target","coefficient":0.857,"pctDone":1,"base":{"min":803,"max":865},"done":{"min":1248,"max":1310},` +
	`"taken":{"min":1248,"max":1310},"crit":{"amount":10000,"result":15000}},{"index":2,"effect":6,"aura":118,"kind":"other"}],` +
	`"attacker":{"attacks":[{"type":"mainhand","attackPower":802},{"type":"offhand","attackPower":802},` +
	`{"type":"ranged","attackPower":33}]}},` +
	`"yellow":{"derived":{"magic":{"rollMin":1,"rollMax":10000,"autoHit":false,"levelDiff":3,"missMultiplier":11,` +
	`"modHitChance":83,"hitChanceBp":8300,"missThreshold":1700,"mechanicResistBp":0,"debuffResistBp":0,` +
	`"binaryResistBp":0,"resistThreshold":1700,"deflectThreshold":1700},"critDone":2.0654998,"critTaken":2.0654998,` +
	`"partialResistsApply":true,"effectiveResist":0.036144577,"effectiveResistWithSpell":0.036144577}}}`

// The fixture's lines read into the replay's records, and give the numbers the sweep compares.
func TestSpellModLineShape(t *testing.T) {
	file := filepath.Join(t.TempDir(), spellModFixture)
	if err := os.WriteFile(file, []byte(spellModFrostbolt+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := readJSONLines[spellModLine](file)
	if err != nil || len(lines) != 1 {
		t.Fatalf("%d lines, %v", len(lines), err)
	}
	l := lines[0]
	if l.key() != (spellModKey{"MageFrost", 3, 0, 42842}) || l.Bot != "Svsleadabcde" {
		t.Errorf("key %+v, bot %s", l.key(), l.Bot)
	}

	timing := spellModServerTiming(l.Timing)
	want := map[string]float64{"cast time before haste": 3000, "cast time": 3000, "gcd": 1500, "cooldown": 0, "cost": 359,
		"aura duration": 9000}
	if !maps.Equal(timing, want) {
		t.Errorf("timing %v, want %v", timing, want)
	}
	if d := l.Damage; len(d.Effects) != 3 || !d.Effects[1].probed() || d.Effects[1].Taken != (damageRange{1248, 1310}) ||
		d.Effects[1].Coefficient != 0.857 || d.Effects[1].Crit.Result != 15000 || d.SpellPower.Damage != 520 ||
		d.Attacker.attack("ranged").AttackPower != 33 || d.Bonus == nil {
		t.Errorf("damage %+v", d)
	}
	yellow := spellModServerYellow(l.Yellow.Derived)
	if want := map[string]float64{"crit chance": 2.0654998, "miss": 1700}; !maps.Equal(yellow, want) {
		t.Errorf("yellow %v, want %v", yellow, want)
	}

	// a weapon spell's yellow table, an aura the server skipped, and runes
	l.Yellow.Derived = spellDerived{Yellow: &yellowTable{MissBp: 800}, CritDone: 5}
	l.Timing.Aura.Skipped = "the aura is already on the target"
	l.Timing.Cost.Runes = &struct {
		Cost           []int32 `json:"cost"`
		RunicPowerGain int32   `json:"runicPowerGain"`
		BaseCooldownMs []int32 `json:"baseCooldownMs"`
	}{Cost: []int32{1, 0, 1}, RunicPowerGain: 150}
	timing = spellModServerTiming(l.Timing)
	for name, v := range map[string]float64{"blood rune cost": 1, "unholy rune cost": 0, "frost rune cost": 1,
		"runic power gain": 150, "runic power cost": 359} {
		if timing[name] != v {
			t.Errorf("%s %v, want %v", name, timing[name], v)
		}
	}
	if _, ok := timing["aura duration"]; ok {
		t.Errorf("a skipped aura's duration is compared")
	}
	if _, ok := timing["cost"]; ok {
		t.Errorf("a rune spell's cost is compared as mana")
	}
	if yellow := spellModServerYellow(l.Yellow.Derived); yellow["miss"] != 800 || yellow["crit chance"] != 5 {
		t.Errorf("yellow %v", yellow)
	}

	// Bloodrage costs health, which the module writes as an unsigned -2
	health := strings.Replace(spellModFrostbolt, `"powerType":0`, `"powerType":4294967294`, 1)
	var bloodrage spellModLine
	if err := json.Unmarshal([]byte(health), &bloodrage); err != nil {
		t.Fatal(err)
	}
	if c := bloodrage.Timing.Cost; c.PowerType != 4294967294 || c.Amount != 359 || bloodrage.Spell != 42842 || len(bloodrage.Damage.Effects) != 3 {
		t.Errorf("health cost read as %+v, spell %d, %d effects", c, bloodrage.Spell, len(bloodrage.Damage.Effects))
	}
}

// A rerun's bot replaces a path's lines, other paths keep theirs, and a bot can't probe a spell twice at
// one state.
func TestSpellModLatestLines(t *testing.T) {
	line := func(path, bot string, steps int, spell int32) spellModLine {
		return spellModLine{Path: path, Bot: bot, Steps: steps, Spell: spell}
	}
	lines := []spellModLine{
		line("MageFire", "Svsleadaaaaa", 0, 1), line("MageFire", "Svsleadaaaaa", 1, 1),
		line("MageFrost", "Svsleadbbbbb", 0, 2),
		line("MageFire", "Svsleadccccc", 1, 1),
	}
	keyed, notes, err := latestSpellModLines(lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(keyed) != 2 || keyed[spellModKey{"MageFire", 1, 0, 1}].Bot != "Svsleadccccc" ||
		keyed[spellModKey{"MageFrost", 0, 0, 2}].Bot != "Svsleadbbbbb" {
		t.Errorf("kept %+v", keyed)
	}
	if want := []string{"MageFire: Svsleadccccc's lines replace Svsleadaaaaa's"}; !slices.Equal(notes, want) {
		t.Errorf("notes %q, want %q", notes, want)
	}
	if _, _, err := latestSpellModLines(append(lines, line("MageFire", "Svsleadccccc", 1, 1))); err == nil {
		t.Error("a spell probed twice at one state is kept")
	}
}

// spellModCase is one listed spell of a path step, with the lines a server agreeing with the sim logs before
// and after the step.
type spellModCase struct {
	path          talentPath
	cast          int32
	pair          spellModPair
	before, after spellModLine
}

func newSpellModCase(t *testing.T, steps map[int32]spellModStep, pathName, talent string, spellID int32) spellModCase {
	t.Helper()
	path := talentPathNamed(t, pathName)
	k := stepNamed(t, path, talent)
	listed, ok := steps[path.Steps[k-1].Spell]
	if !ok {
		t.Fatalf("%s isn't in %s", talent, spellModSpellsFile)
	}
	c := spellModCase{path: path}
	if !listed.passive {
		c.cast = path.Steps[k-1].Spell
	}
	sims := newSpellModSims(path)
	if c.pair = sims.pair(k, c.cast, spellID); c.pair.err != nil {
		t.Fatal(c.pair.err)
	}
	if c.pair.spec != "" {
		c.before = spellModLineFrom(t, sims, k-1, 0, spellID, c.pair.spec, c.pair.teach)
		c.after = spellModLineFrom(t, sims, k, c.cast, spellID, c.pair.spec, c.pair.teach)
	}
	return c
}

func spellModEffectAt(t *testing.T, l *spellModLine, index int32) *damageEffect {
	t.Helper()
	for i := range l.Damage.Effects {
		if l.Damage.Effects[i].Index == index {
			return &l.Damage.Effects[i]
		}
	}
	t.Fatalf("no effect %d in %+v", index, l.Damage.Effects)
	return nil
}

func TestSpellModCatchesMismatches(t *testing.T) {
	needsItemDB(t)
	steps := spellModSteps(t)
	// what the case's line must say it left out
	skipped := map[string]string{
		"a shaped damage record":        "damage: shaped",
		"a spell the sim only triggers": "cast time, GCD, cost and cooldown",
		"a finisher's duration":         "aura duration and ticks",
	}
	for _, c := range []struct {
		name, path, talent string
		spell              int32
		change             func(t *testing.T, after *spellModLine)
		class              string
		failed             []string
		moved              string
	}{
		{"cast time as the sim has it", "WarriorArms", "improvedSlam", 47475, nil, spellModAgrees, nil, "cast time before haste"},
		{"half the cast time cut", "WarriorArms", "improvedSlam", 47475, func(t *testing.T, l *spellModLine) {
			l.Timing.Cast.AfterSpellModsMs += 500
			l.Timing.Cast.Ms += 500
		}, spellModDisagrees, []string{"cast time", "cast time before haste"}, "cast time"},
		{"rage cost", "WarriorArms", "improvedHeroicStrike", 47450, func(t *testing.T, l *spellModLine) { l.Timing.Cost.Amount += 10 },
			spellModDisagrees, []string{"cost"}, "cost"},
		{"cooldown", "WarriorArms", "improvedMortalStrike", 47486, func(t *testing.T, l *spellModLine) { l.Timing.Cooldown.Ms += 1000 },
			spellModDisagrees, []string{"cooldown"}, "cooldown"},
		{"dot duration and ticks", "DeathknightUnholy", "epidemic", 55078, func(t *testing.T, l *spellModLine) {
			l.Timing.Aura.Ms -= 3000
			l.Timing.Aura.Effects[0].Ticks--
		}, spellModDisagrees, []string{"aura duration", "e0 ticks"}, "aura duration"},
		{"crit chance", "WarriorArms", "improvedOverpower", 7384, func(t *testing.T, l *spellModLine) { l.Yellow.Derived.CritDone -= 25 },
			spellModDisagrees, []string{"crit chance"}, "crit chance"},
		// the death knight's frost spells have no crit before Deathchill, which the server logs as 0
		{"crit chance from 0", "DeathknightFrost", "deathchill", 49909, func(t *testing.T, l *spellModLine) {
			l.Yellow.Derived.CritDone = 50
		}, spellModDisagrees, []string{"crit chance"}, "crit chance"},
		{"spell hit", "PriestShadow", "shadowFocus", 48125, func(t *testing.T, l *spellModLine) { l.Yellow.Derived.Magic.MissThreshold += 300 },
			spellModDisagrees, []string{"miss"}, "miss"},
		// the sim works its amounts out at the server's base, so they drop with it
		{"a flat base the server doesn't add", "WarriorArms", "improvedRend", 47465, func(t *testing.T, l *spellModLine) {
			e := spellModEffectAt(t, l, 0)
			e.Base.Min -= 15
			e.Base.Max -= 15
		}, spellModDisagrees, []string{"e0 dot base min", "e0 dot base max", "e0 dot min", "e0 dot max"}, "e0 dot base min"},
		{"5% more damage", "WarriorArms", "improvedRend", 47465, func(t *testing.T, l *spellModLine) {
			e := spellModEffectAt(t, l, 0)
			e.Taken.Min, e.Taken.Max = math.Round(e.Taken.Min*1.05), math.Round(e.Taken.Max*1.05)
		}, spellModDisagrees, []string{"e0 dot min", "e0 dot max"}, "e0 dot min"},
		{"coefficient", "PriestHoly", "empoweredHealing", 48063, func(t *testing.T, l *spellModLine) {
			spellModEffectAt(t, l, 0).Coefficient -= 0.4
		}, spellModDisagrees, []string{"e0 heal coefficient"}, "e0 heal min"},
		{"crit damage", "WarriorArms", "impale", 47502, func(t *testing.T, l *spellModLine) { spellModEffectAt(t, l, 0).Crit.Result = 15000 },
			spellModDisagrees, []string{"e0 damage crit"}, "e0 damage crit"},
		{"a talent that moves nothing", "WarriorArms", "impale", 47465, nil, spellModMovesNothing, nil, ""},
		{"a talent that moves what the sim doesn't", "WarriorArms", "impale", 47465, func(t *testing.T, l *spellModLine) {
			l.Timing.Cost.Amount -= 50
		}, spellModDisagrees, []string{"cost"}, "cost"},
		{"a shaped damage record", "WarriorArms", "improvedRend", 47465, func(t *testing.T, l *spellModLine) {
			l.Damage.Shaped = "script spell_warr_rend"
			spellModEffectAt(t, l, 0).Taken.Min = 1
		}, spellModMovesNothing, nil, ""},
		{"the image's spell", "MageArcane", "spellImpact", 59637, nil, spellModNotCompared, nil, ""},
		// Cold Blood's +100 stops the server adding Puncturing Wounds, which the sim adds
		{"crit chance past 100", "RogueAssassination", "coldBlood", 48657, func(t *testing.T, l *spellModLine) {
			l.Yellow.Derived.CritDone = 108.6
		}, spellModAgrees, nil, "crit chance"},
		{"a cast that hits through other spells", "RogueAssassination", "coldBlood", 51723, nil, spellModAgrees, nil, "crit chance"},
		{"a cast talent the sim has no aura for", "MageArcane", "presenceOfMind", 42842, nil, spellModNotCompared, nil, ""},
		// the server's Slam hit 50783 keeps the DBC's cast time and cost, which its triggered cast never uses
		{"a spell the sim only triggers", "WarriorArms", "improvedSlam", 50783, func(t *testing.T, l *spellModLine) {
			l.Timing.Cast.AfterSpellModsMs += 500
			l.Timing.Cast.Ms += 500
			l.Timing.Cost.Amount += 150
		}, spellModMovesNothing, nil, ""},
		// the server probes Slice and Dice without combo points, the sim's aura holds its 5 point duration
		{"a finisher's duration", "RogueCombat", "improvedSliceAndDice", 6774, func(t *testing.T, l *spellModLine) {
			l.Timing.Aura.Ms = 9000
		}, spellModMovesNothing, nil, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := newSpellModCase(t, steps, c.path, c.talent, c.spell)
			if c.change != nil {
				c.change(t, &sc.after)
			}
			result := compareSpellMod(sc.path, sc.before, sc.after, sc.pair)
			if _, failed := checkNames(result.checks); result.class != c.class || !slices.Equal(failed, c.failed) {
				t.Errorf("%s, failed %v; want %s, %v: %s", result.class, failed, c.class, c.failed,
					strings.Join(failedChecks(result.checks), "; "))
			}
			if c.moved != "" && !slices.ContainsFunc(result.moved, func(m string) bool { return strings.HasPrefix(m, c.moved+" ") }) {
				t.Errorf("moved %v, want %s among them", result.moved, c.moved)
			}
			if want, ok := skipped[c.name]; ok && !slices.ContainsFunc(result.skipped, func(s string) bool { return strings.HasPrefix(s, want) }) {
				t.Errorf("skipped %v, want %q among them", result.skipped, want)
			}
		})
	}
}

// A number is held to the sim only when all four sides have it; one the server moved without that is noted.
func TestSpellModCompare(t *testing.T) {
	server := map[string]float64{"n": 30000}
	moved := map[string]float64{"n": 45000}
	none := map[string]float64{}
	for _, c := range []struct {
		name           string
		sb, sa, mb, ma map[string]float64
		checks         int
		skipped        string
	}{
		{"all four sides", server, moved, server, moved, 1, ""},
		{"the sim has none, the server didn't move it", server, server, none, none, 0, ""},
		{"the sim has none", server, moved, none, server, 0, "n: the server's 30000 -> 45000, the sim has none"},
		{"the server has it before only", server, none, server, server, 0, "n: the server has it before only"},
		{"the server has it after only", none, moved, server, server, 0, "n: the server has it after only"},
		{"nobody has it", none, none, server, server, 0, ""},
	} {
		r := spellModResult{class: spellModMovesNothing}
		r.compare("n", c.sb, c.sa, c.mb, c.ma, 0)
		if len(r.checks) != c.checks || strings.Join(r.skipped, "; ") != c.skipped {
			t.Errorf("%s: %d checks, skipped %q; want %d, %q", c.name, len(r.checks), r.skipped, c.checks, c.skipped)
		}
	}
}

// The replay holds every listed spell of a step to both its lines and to the drops: a missing line fails,
// a dropped one is logged, and a drop that matches nothing or keeps an agreeing spell fails.
func TestSpellModReplay(t *testing.T) {
	needsItemDB(t)
	all := spellModSteps(t)
	path := talentPathNamed(t, "WarriorArms")
	steps := map[int32]spellModStep{}
	for _, talent := range []string{"improvedSlam", "improvedHeroicStrike"} {
		spell := path.Steps[stepNamed(t, path, talent)-1].Spell
		steps[spell] = all[spell]
	}
	slam := newSpellModCase(t, all, "WarriorArms", "improvedSlam", 47475)
	// Improved Slam lists both of Slam's spells
	slam2 := newSpellModCase(t, all, "WarriorArms", "improvedSlam", 50783)
	strike := newSpellModCase(t, all, "WarriorArms", "improvedHeroicStrike", 47450)
	lines := func(skip *spellModLine, change func(*spellModLine)) map[spellModKey]spellModLine {
		keyed := map[spellModKey]spellModLine{}
		for _, l := range []spellModLine{slam.before, slam.after, slam2.before, slam2.after, strike.before, strike.after} {
			if skip != nil && l.key() == skip.key() {
				continue
			}
			keyed[l.key()] = l
		}
		if change != nil {
			l := keyed[strike.after.key()]
			change(&l)
			keyed[l.key()] = l
		}
		return keyed
	}
	costMore := func(l *spellModLine) { l.Timing.Cost.Amount += 10 }

	for _, c := range []struct {
		name   string
		lines  map[spellModKey]spellModLine
		drops  []spellModDrop
		errors []string
		logs   []string
	}{
		{"agreeing lines", lines(nil, nil), nil, nil, []string{"2 spell steps agree"}},
		{"a missing line", lines(&slam.after, nil), nil, []string{"47475 Slam: no probe before or after it"}, nil},
		{"a missing line dropped", lines(&slam.after, nil), []spellModDrop{{"WarriorArms", "improvedSlam", 47475, ""}},
			nil, []string{"47475 Slam (dropped): no probe"}},
		{"a disagreeing check", lines(nil, costMore), nil, []string{"47450 Heroic Strike: cost: server"}, nil},
		{"a disagreeing check dropped", lines(nil, costMore), []spellModDrop{{"WarriorArms", "improvedHeroicStrike", 47450, "cost"}},
			nil, []string{"47450 Heroic Strike: cost (dropped)"}},
		{"a disagreeing spell dropped", lines(nil, costMore), []spellModDrop{{"WarriorArms", "improvedHeroicStrike", 47450, ""}},
			nil, []string{"47450 Heroic Strike: cost (dropped)"}},
		{"a disagreeing step dropped", lines(nil, costMore), []spellModDrop{{"WarriorArms", "improvedHeroicStrike", 0, ""}},
			nil, []string{"47450 Heroic Strike: cost (dropped)"}},
		{"a passing check dropped", lines(nil, nil), []spellModDrop{{"WarriorArms", "improvedHeroicStrike", 47450, "cost"}},
			[]string{"cost passes, so it shouldn't be dropped"}, nil},
		{"an agreeing spell dropped", lines(nil, nil), []spellModDrop{{"WarriorArms", "improvedSlam", 47475, ""}},
			[]string{"47475 Slam agrees, so it shouldn't be dropped"}, nil},
		{"an agreeing step dropped", lines(nil, nil), []spellModDrop{{"WarriorArms", "improvedSlam", 0, ""}},
			[]string{"no spell disagrees, so it shouldn't be dropped"}, nil},
		{"a drop no step matches", lines(nil, nil), []spellModDrop{{"WarriorArms9", "improvedSlam", 0, ""}},
			[]string{"no step of the paths matches it"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			report := replaySpellMods([]talentPath{path}, steps, c.lines, c.drops)
			if len(report.errors) != len(c.errors) {
				t.Errorf("errors %q, want %d like %q", report.errors, len(c.errors), c.errors)
			}
			for _, want := range c.errors {
				if !slices.ContainsFunc(report.errors, func(e string) bool { return strings.Contains(e, want) }) {
					t.Errorf("errors %q, want one with %q", report.errors, want)
				}
			}
			for _, want := range c.logs {
				if !slices.ContainsFunc(report.logs, func(l string) bool { return strings.Contains(l, want) }) {
					t.Errorf("logs %q, want one with %q", report.logs, want)
				}
			}
		})
	}
}
