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

// droppedBuffs are states of buffsFixture, or checks of them, that disagree with the sim, each with a row
// in the fixture README.
var droppedBuffs = []buffDrop{
	{buffsSetup, "debuff.curse_of_weakness+debuff.vindication", "dummy armor"},
	{buffsSetup, "debuff.curse_of_weakness+debuff.vindication", "armor multiplier"},
	{buffsSetup, "debuff.insect_swarm+debuff.scorpid_sting", "dummy melee hit %"},
}

// TestBuffRecords replays a live TestSimvalBuffs run, its BUFFS lines: each state's change from the bare
// character against the sim's. Skips without the capture. Every state is logged on a BUFFSIM line.
func TestBuffRecords(t *testing.T) {
	needsItemDB(t)
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, fixtureDir, buffsFixture)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalBuffs and keep its BUFFS lines", file)
	}
	lines, err := readJSONLines[buffLine](file)
	if err != nil {
		t.Fatal(err)
	}
	report := replayBuffs(lines, allGoldcharSetups(t), droppedBuffs)
	for _, line := range report.logs {
		t.Log(line)
	}
	for _, e := range report.errors {
		t.Error(e)
	}
}

// Every key names a switch the full messages turn on, every probed one is a state, and the lists don't
// overlap.
func TestBuffStatesCoverTheFullBuffs(t *testing.T) {
	full := fullBuffKeys()
	for key, why := range unprobedBuffs {
		if _, err := buffField(key); err != nil {
			t.Errorf("unprobed: %v", err)
		}
		if strings.TrimSpace(why) == "" {
			t.Errorf("unprobed %s needs a reason", key)
		}
	}
	probed := func(key string) bool {
		_, unprobed := unprobedBuffs[key]
		return slices.Contains(full, key) && !unprobed
	}
	seen := map[[2]string]bool{}
	for _, pair := range buffPairs {
		if !probed(pair[0]) || !probed(pair[1]) || pair[0] == pair[1] {
			t.Errorf("pair %v needs two different probed switches", pair)
		}
		if seen[pair] || seen[[2]string{pair[1], pair[0]}] {
			t.Errorf("pair %v twice", pair)
		}
		seen[pair] = true
	}
	for key := range buffHeldAuras {
		if !probed(key) {
			t.Errorf("held aura of %s, which isn't probed", key)
		}
	}

	states := buffStates()
	names := map[string]bool{}
	for _, s := range states {
		if names[s.name] {
			t.Errorf("state %s twice", s.name)
		}
		names[s.name] = true
	}
	if want := len(full) - len(unprobedBuffs) + len(buffPairs); len(states) != want {
		t.Errorf("%d states, want %d", len(states), want)
	}
}

// buffTarget is the boss dummy for a sim of the setup's character.
func buffTarget(s goldcharSetup) damageRecord {
	var rec damageRecord
	rec.Command = "info"
	rec.Attacker.Class, rec.Attacker.Race, rec.Attacker.Level, rec.Attacker.IsPlayer = s.Class, s.Race, 80, true
	rec.Target.Entry, rec.Target.Level, rec.Target.WorldBoss, rec.Target.CreatureType = boss, 83, true, 5
	rec.Target.Resistances = []float64{10643, 0, 0, 0, 0, 0, 0}
	return rec
}

func newTestBuffSim(t *testing.T, s goldcharSetup, keys []string) *damageSim {
	t.Helper()
	d, err := newBuffSim(buffTarget(s), s, keys)
	if err != nil {
		t.Fatalf("%v: %v", keys, err)
	}
	return d
}

// buffFingerprint is what the sim's state moves: the sheet, mp5, the dummy's numbers and the damage
// multipliers on both sides.
func buffFingerprint(t *testing.T, s goldcharSetup, keys []string) map[string]float64 {
	t.Helper()
	d := newTestBuffSim(t, s, keys)
	side := d.buffSide()
	if side.sheet.err != nil {
		t.Fatal(side.sheet.err)
	}
	v := maps.Clone(side.sheet.values)
	maps.Copy(v, side.dummy)
	v["mp5"] = side.mp5
	d.start()
	c, u := d.char.PseudoStats, d.target.PseudoStats
	v["damage dealt"] = c.DamageDealtMultiplier
	v["dummy physical taken"] = u.SchoolDamageTakenMultiplier[stats.SchoolIndexPhysical]
	v["dummy nature taken"] = u.SchoolDamageTakenMultiplier[stats.SchoolIndexNature]
	v["dummy bleed taken"] = u.PeriodicPhysicalDamageTakenMultiplier
	v["dummy disease taken"] = u.DiseaseDamageTakenMultiplier
	v["dummy flat physical taken"] = u.BonusPhysicalDamageTaken
	v["dummy crit taken"] = u.BonusCritRatingTaken
	v["dummy spell crit taken"] = u.BonusSpellCritRatingTaken
	v["dummy spell hit taken"] = u.BonusSpellHitRatingTaken
	return v
}

// Each pair is exclusive in the sim: on some number both move, the pair moves it as far as one of them
// alone does.
func TestBuffPairsAreExclusiveInTheSim(t *testing.T) {
	needsItemDB(t)
	s := goldcharSetupNamed(t, buffsSetup)
	none := buffFingerprint(t, s, nil)
	delta := func(keys ...string) map[string]float64 {
		v := buffFingerprint(t, s, keys)
		for name := range v {
			v[name] -= none[name]
		}
		return v
	}
	near := func(a, b float64) bool { return math.Abs(a-b) <= 1e-6 }
	for _, pair := range buffPairs {
		a, b, both := delta(pair[0]), delta(pair[1]), delta(pair[0], pair[1])
		var shared []string
		for _, name := range slices.Sorted(maps.Keys(both)) {
			if !near(a[name], 0) && !near(b[name], 0) && (near(both[name], a[name]) || near(both[name], b[name])) {
				shared = append(shared, name)
			}
		}
		if len(shared) == 0 {
			t.Errorf("%s and %s stack on everything they both move", pair[0], pair[1])
		} else {
			t.Logf("%s and %s: one wins on %s", pair[0], pair[1], strings.Join(shared, ", "))
		}
	}
}

// buffLineFrom is the BUFFS line a server agreeing with the sim would log for the setup's character with
// the keys on, its whole numbers truncated the way the module's are.
func buffLineFrom(t *testing.T, s goldcharSetup, keys []string) buffLine {
	t.Helper()
	d := newTestBuffSim(t, s, keys)
	side := d.buffSide()
	if side.sheet.err != nil {
		t.Fatal(side.sheet.err)
	}
	v, dummy := side.sheet.values, side.dummy
	floor := func(x float64) float64 { return math.Floor(x + 1e-6) }

	d.start()
	spellPower := floor(d.char.GetStat(stats.SpellPower))
	schools := map[string]float64{"physical": 0}
	spellCrit := []float64{0}
	for _, school := range magicSchools {
		schools[school] = spellPower
		crit, ok := v[school+" spell crit %"]
		if !ok {
			crit = v["spell crit %"]
		}
		spellCrit = append(spellCrit, crit)
	}
	var attacks []map[string]any
	for _, kind := range sweepAttacks {
		ap, haste := v["attack power"], v["melee haste"]
		if kind == "ranged" {
			ap, haste = v["ranged attack power"], v["ranged haste"]
		}
		attacks = append(attacks, map[string]any{"type": kind, "attackPower": floor(ap), "speedMultiplier": 1 / haste,
			"expertiseReduction": v[kind+" expertise %"], "missChanceTaken": v[kind+" miss taken %"]})
	}
	var powerType uint32
	var maxPower float64
	for _, pt := range []uint32{6, 3, 1, 0} {
		if p := sweepPowerFields[pt]; v[p.name] > 0 {
			powerType, maxPower = pt, floor(v[p.name]*p.scale)
			break
		}
	}
	auras := []map[string]any{}
	for _, id := range s.Auras {
		auras = append(auras, map[string]any{"id": id, "durationMs": -1})
	}
	info := map[string]any{
		"schema": 1, "command": "info",
		"attacker": map[string]any{
			"name": "Svbufabcde", "level": 80, "isPlayer": true, "class": s.Class, "race": s.Race,
			"maxHealth": floor(v["max health"]), "powerType": powerType, "maxPower": maxPower,
			"stats": map[string]float64{"strength": floor(v["strength"]), "agility": floor(v["agility"]),
				"stamina": floor(v["stamina"]), "intellect": floor(v["intellect"]), "spirit": floor(v["spirit"])},
			"resistances": []float64{floor(v["armor"]), 0, floor(v["fire resistance"]), floor(v["nature resistance"]),
				floor(v["frost resistance"]), floor(v["shadow resistance"]), floor(v["arcane resistance"])},
			"spellPower":              schools,
			"hitMods":                 map[string]float64{"melee": v["melee hit %"], "ranged": v["ranged hit %"], "spell": v["spell hit %"]},
			"castSpeedMultiplier":     1 / v["spell haste"],
			"armorPenetrationAuraPct": v["armor penetration %"],
			"defense":                 map[string]float64{"dodge": v["dodge %"], "parry": v["parry %"], "block": v["block %"]},
			"attacks":                 attacks,
			"sheet": map[string]any{"critMainhand": v["mainhand crit %"], "critOffhand": v["offhand crit %"],
				"critRanged": v["ranged crit %"], "spellCrit": spellCrit, "shieldBlock": floor(v["block value"])},
			"regen": map[string]any{"flat": make([]float64, 7), "interrupted": []float64{side.mp5 / 5, 0, 0, 0, 0, 0, 0},
				"rune": make([]float64, 4), "manaRate": 1, "energyRate": 1},
			"auras": auras,
		},
		"target": map[string]any{
			"name": "SimVal Boss Dummy", "entry": boss, "level": 83, "worldBoss": true, "creatureType": 5,
			"resistances": []float64{floor(dummy["dummy armor"]), 0, dummy["dummy fire resistance"],
				dummy["dummy nature resistance"], dummy["dummy frost resistance"], dummy["dummy shadow resistance"],
				dummy["dummy arcane resistance"]},
			"hitMods": map[string]float64{"melee": dummy["dummy melee hit %"]},
			"attacks": []map[string]any{{"type": "mainhand", "attackPower": dummy["dummy attack power"],
				"speedMultiplier": 1 / dummy["dummy melee haste"]}},
			"auras": []any{},
		},
		"character": goldcharCharacterJSON(s),
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var parsed buffInfo
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}

	var damage []damageRecord
	var yellow []buffYellow
	for _, p := range s.Probes {
		if p.Shaped != "" {
			continue
		}
		var rec damageRecord
		rec.Spell.ID, rec.Spell.Name = p.ID, p.Name
		d.start()
		rec.SpellPower.Damage = floor(d.char.GetStat(stats.SpellPower))
		rec.SpellPower.Healing = rec.SpellPower.Damage
		rec.AttackPower.Type, rec.AttackPower.Value = "mainhand", floor(d.char.GetStat(stats.AttackPower))
		ts := d.withRecord(rec, parsed)
		spell := serverdata.SpellByID(p.ID)
		if spell == nil {
			t.Fatalf("%d %s isn't in serverdata", p.ID, p.Name)
		}
		for i := range spell.Effects {
			e := damageEffect{Index: int32(i), Kind: spellModProbedKind(spell.Effects[i])}
			if e.Kind == "" {
				continue
			}
			decl := ts.declaration(e)
			if decl == nil {
				continue
			}
			declared := decl.effect()
			e.Base, e.Coefficient = damageRange{Min: declared.Min, Max: declared.Max}, declared.SP
			m := ts.spellModEffect(e)
			if m.err != nil {
				t.Fatalf("%d %s e%d: %v", p.ID, p.Name, i, m.err)
			}
			e.Taken = damageRange{Min: math.Round(m.lo), Max: math.Round(m.hi)}
			rec.Effects = append(rec.Effects, e)
		}
		damage = append(damage, rec)

		weapon := spellModWeapon(p.ID)
		crit, miss, ok := d.buffHit(rec, parsed, weapon)
		if !ok {
			t.Fatalf("%d %s: no sim spell for the hit table", p.ID, p.Name)
		}
		y := buffYellow{Derived: spellDerived{CritDone: crit, CritTaken: crit}}
		y.Spell.ID = p.ID
		if weapon {
			y.Derived.Yellow = &yellowTable{MissBp: int32(math.Round(800 + miss))}
		} else {
			y.Derived.Magic = &magicTable{MissThreshold: int32(math.Round(miss)) + 1}
		}
		yellow = append(yellow, y)
	}

	armor := floor(dummy["dummy armor"])
	view := record{Attacker: parsed.Attacker.unitSnapshot, Target: parsed.Target.unitSnapshot}
	line := map[string]any{
		"setup": s.Name, "bot": "Svbufabcde", "state": stateName(keys), "keys": keys,
		"info": json.RawMessage(raw), "damage": damage, "yellow": yellow,
		"armor": map[string]any{"probeDamage": 10000, "scenarios": []map[string]float64{
			{"armor": armor, "multiplier": armorMultiplier(view, armor)}}},
	}
	text, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	var out buffLine
	if err := json.Unmarshal(text, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// buffLinesFrom is a whole run as the sim would record it: bare, every state, then bare again.
func buffLinesFrom(t *testing.T, s goldcharSetup) []buffLine {
	t.Helper()
	lines := []buffLine{buffLineFrom(t, s, nil)}
	for _, state := range buffStates() {
		lines = append(lines, buffLineFrom(t, s, state.keys))
	}
	return append(lines, buffLineFrom(t, s, nil))
}

// A server that agrees with the sim agrees on every state, which checks the units, the line's fields
// and the sim side's numbers.
func TestBuffReplayAgreesWithTheSim(t *testing.T) {
	needsItemDB(t)
	s := goldcharSetupNamed(t, buffsSetup)
	report := replayBuffs(buffLinesFrom(t, s), []goldcharSetup{s}, nil)
	for _, line := range report.logs {
		t.Log(line)
	}
	for _, e := range report.errors {
		t.Error(e)
	}
	if report.counts[spellModDisagrees] > 0 || report.counts[spellModAgrees] < len(buffStates())/2 {
		t.Errorf("classes %v; want most states to agree and none to disagree", report.counts)
	}
}

// buffCase is a bare line and one state's, both from the sim, for a check to go wrong in.
type buffCase struct {
	t           *testing.T
	s           goldcharSetup
	bare, state buffLine
}

func newBuffCase(t *testing.T, keys ...string) buffCase {
	s := goldcharSetupNamed(t, buffsSetup)
	return buffCase{t, s, buffLineFrom(t, s, nil), buffLineFrom(t, s, keys)}
}

func (c buffCase) failed() []string {
	c.t.Helper()
	simBare := newTestBuffSim(c.t, c.s, nil)
	simState := newTestBuffSim(c.t, c.s, c.state.Keys)
	result := compareBuffState(buffPair{bare: c.bare, state: c.state, simBare: simBare, simState: simState,
		sideBare: simBare.buffSide(), side: simState.buffSide()})
	return failedChecks(result.checks)
}

// expect fails unless exactly the named checks fail, in any order.
func (c buffCase) expect(names ...string) {
	c.t.Helper()
	var got []string
	for _, f := range c.failed() {
		name, _, _ := strings.Cut(f, ":")
		got = append(got, name)
	}
	slices.Sort(got)
	if !slices.Equal(got, slices.Sorted(slices.Values(names))) {
		c.t.Errorf("failed %v, want %v", c.failed(), names)
	}
}

// firstProbed is the name prefix of the line's first probed effect of the spell, and its index in
// Damage and Effects.
func firstProbed(t *testing.T, l buffLine, spell int32) (string, int, int) {
	t.Helper()
	for i, rec := range l.Damage {
		if rec.Spell.ID != spell {
			continue
		}
		for j, e := range rec.Effects {
			if e.probed() {
				return fmt.Sprintf("%d e%d %s", spell, e.Index, e.Kind), i, j
			}
		}
	}
	t.Fatalf("no probed effect of %d", spell)
	return "", 0, 0
}

// Each part of a state's comparison catches a server number that moved differently from the sim's.
func TestBuffReplayCatchesMismatches(t *testing.T) {
	needsItemDB(t)
	const instantPoison, garrote = 57965, 48676

	t.Run("agreeing", func(t *testing.T) {
		newBuffCase(t, "raid.battle_shout").expect()
	})
	t.Run("attack power", func(t *testing.T) {
		c := newBuffCase(t, "raid.battle_shout")
		for i, a := range c.state.Info.raw.Attacker.Attacks {
			if a.Type == "mainhand" {
				c.state.Info.raw.Attacker.Attacks[i].AttackPower += 40
			}
		}
		c.expect("attack power")
	})
	t.Run("mp5", func(t *testing.T) {
		c := newBuffCase(t, "individual.blessing_of_wisdom")
		c.state.Info.regen.Interrupted[0] += 2
		c.expect("mp5")
	})
	t.Run("damage taken", func(t *testing.T) {
		c := newBuffCase(t, "debuff.curse_of_elements")
		name, i, j := firstProbed(t, c.state, instantPoison)
		e := &c.state.Damage[i].Effects[j]
		e.Taken.Min, e.Taken.Max = math.Round(e.Taken.Min*1.05), math.Round(e.Taken.Max*1.05)
		c.expect(name+" min", name+" max")
	})
	t.Run("bleed", func(t *testing.T) {
		c := newBuffCase(t, "debuff.mangle")
		name, i, j := firstProbed(t, c.state, garrote)
		e := &c.state.Damage[i].Effects[j]
		e.Taken.Min, e.Taken.Max = c.bare.Damage[i].Effects[j].Taken.Min, c.bare.Damage[i].Effects[j].Taken.Max
		c.expect(name+" min", name+" max")
	})
	t.Run("crit taken", func(t *testing.T) {
		c := newBuffCase(t, "debuff.shadow_mastery")
		i := slices.IndexFunc(c.state.Yellow, func(y buffYellow) bool { return y.Spell.ID == instantPoison })
		c.state.Yellow[i].Derived.CritTaken += 2
		c.expect(fmt.Sprintf("%d crit taken", instantPoison))
	})
	t.Run("spell hit taken", func(t *testing.T) {
		c := newBuffCase(t, "debuff.misery")
		i := slices.IndexFunc(c.state.Yellow, func(y buffYellow) bool { return y.Spell.ID == instantPoison })
		c.state.Yellow[i].Derived.Magic.MissThreshold += 100
		c.expect(fmt.Sprintf("%d miss", instantPoison))
	})
	t.Run("armor", func(t *testing.T) {
		c := newBuffCase(t, "debuff.sunder_armor")
		c.state.Info.Target.Resistances[0] += 50
		c.state.Armor.Scenarios[0].Multiplier += 0.01
		c.expect("armor multiplier", "dummy armor")
	})
	t.Run("dummy haste", func(t *testing.T) {
		c := newBuffCase(t, "debuff.thunder_clap")
		c.state.Info.dummySpeed = c.bare.Info.dummySpeed
		c.expect("dummy melee haste")
	})
	t.Run("server auras", func(t *testing.T) {
		c := newBuffCase(t, "raid.battle_shout")
		c.state.Missing = []int32{47436}
		c.expect("server auras")
	})
}

// The replay holds a run to every state and its drops to what fails.
func TestBuffReplayDrops(t *testing.T) {
	needsItemDB(t)
	s := goldcharSetupNamed(t, buffsSetup)
	const state = "raid.battle_shout"
	lines := []buffLine{buffLineFrom(t, s, nil)}
	agreeing := buffLineFrom(t, s, []string{state})
	wrong := buffLineFrom(t, s, []string{state})
	wrong.Missing = []int32{47436}
	// every other state dropped, so only the one under test counts
	var others []buffDrop
	for _, st := range buffStates() {
		if st.name != state {
			others = append(others, buffDrop{s.Name, st.name, ""})
		}
	}
	replay := func(state buffLine, drops ...buffDrop) spellModReport {
		return replayBuffs(append(slices.Clone(lines), state), []goldcharSetup{s}, append(slices.Clone(others), drops...))
	}

	if r := replay(agreeing); len(r.errors) > 0 {
		t.Errorf("agreeing: %v", r.errors)
	}
	if r := replay(wrong); len(r.errors) != 1 || !strings.Contains(r.errors[0], "server auras") {
		t.Errorf("wrong: %v", r.errors)
	}
	if r := replay(wrong, buffDrop{s.Name, state, "server auras"}); len(r.errors) > 0 {
		t.Errorf("dropped check: %v", r.errors)
	}
	if r := replay(wrong, buffDrop{s.Name, state, ""}); len(r.errors) > 0 {
		t.Errorf("dropped state: %v", r.errors)
	}
	if r := replay(agreeing, buffDrop{s.Name, state, ""}); len(r.errors) != 1 || !strings.Contains(r.errors[0], "shouldn't be dropped") {
		t.Errorf("agreeing state dropped: %v", r.errors)
	}
	if r := replay(agreeing, buffDrop{s.Name, state, "attack power"}); len(r.errors) != 1 || !strings.Contains(r.errors[0], "passes") {
		t.Errorf("passing check dropped: %v", r.errors)
	}
	if r := replay(agreeing, buffDrop{s.Name, "no.such_state", ""}); len(r.errors) != 1 || !strings.Contains(r.errors[0], "no state") {
		t.Errorf("stale drop: %v", r.errors)
	}
	r := replayBuffs(lines, []goldcharSetup{s}, others)
	if len(r.errors) != 1 || !strings.Contains(r.errors[0], "no line for") {
		t.Errorf("missing state: %v", r.errors)
	}
}

// buffBattleShout is a BUFFS line as the e2e logs it, cut down to the fields the replay reads.
const buffBattleShout = `{"setup":"RogueAssassination","bot":"Svbufxyzab","state":"raid.battle_shout",` +
	`"keys":["raid.battle_shout"],"missing":[47436],"extra":[],` +
	`"info":{"schema":1,"command":"info","attacker":{"name":"Svbufxyzab","level":80,"class":4,"race":1,` +
	`"isPlayer":true,"maxHealth":18054,"powerType":3,"maxPower":100,` +
	`"stats":{"strength":113,"agility":1053,"stamina":1063,"intellect":43,"spirit":69},` +
	`"resistances":[5641,0,0,0,0,0,0],"hitMods":{"melee":12.776761,"ranged":12.776761,"spell":14.720951},` +
	`"castSpeedMultiplier":0.92548686,"spellPower":{"physical":0,"holy":0,"fire":0,"nature":0,"frost":0,` +
	`"shadow":0,"arcane":0},"defense":{"dodge":28.257072,"parry":0,"block":0,"blockValue":46},` +
	`"attacks":[{"type":"mainhand","unhastedMs":1800,"speedMultiplier":0.84135175,"hastedMs":1514,` +
	`"attackPower":4029.5598,"missChanceTaken":5,"expertiseReduction":6.3121376},` +
	`{"type":"offhand","unhastedMs":1400,"speedMultiplier":0.84135175,"hastedMs":1177,"attackPower":4029.5598,` +
	`"missChanceTaken":5,"expertiseReduction":6.3121376},{"type":"ranged","unhastedMs":2900,` +
	`"speedMultiplier":0.92548686,"hastedMs":2683,"attackPower":3718,"missChanceTaken":5,"expertiseReduction":0}],` +
	`"sheet":{"critMainhand":36.28253,"critOffhand":36.28253,"critRanged":31.282534,` +
	`"spellCrit":[0,13.941534,13.941534,13.941534,13.941534,13.941534,13.941534],"shieldBlock":46},` +
	`"regen":{"flat":[0,0,0,0,0,0,0],"interrupted":[18.4,0,0,0,0,0,0],"rune":[0,0,0,0],"manaRate":1,"energyRate":1},` +
	`"armorPenetrationAuraPct":0,"auras":[{"id":47436,"durationMs":120000,"fromSelf":false}]},` +
	`"target":{"name":"SimVal Boss Dummy","entry":999000,"level":83,"worldBoss":true,"creatureType":5,` +
	`"resistances":[8514,0,-165,-165,-165,-165,-165],"hitMods":{"melee":-3,"ranged":-3,"spell":0},` +
	`"attacks":[{"type":"mainhand","speedMultiplier":1.2,"attackPower":232},` +
	`{"type":"offhand","speedMultiplier":1.2,"attackPower":232},{"type":"ranged","speedMultiplier":1,"attackPower":135}],` +
	`"auras":[]},"character":{"talents":[],"glyphs":[0,0,0,0,0,0],"equipment":[]}},` +
	`"damage":[{"command":"damage","spell":{"id":57965,"name":"Instant Poison IX"},"shaped":"","comboPoints":0,` +
	`"spellPower":{"damage":0,"healing":0},"attackPower":{"type":"mainhand","value":4029},` +
	`"bonus":{"direct":0,"dot":0,"ap":0.1,"apDot":0},"effects":[{"index":0,"effect":2,"aura":0,"kind":"damage",` +
	`"victim":"target","coefficient":0,"pctDone":1.1,"base":{"min":300,"max":400},"done":{"min":773,"max":883},` +
	`"taken":{"min":873,"max":997},"crit":{"amount":10000,"result":15000}}]}],` +
	`"yellow":[{"command":"yellow","spell":{"id":57965},"derived":{"magic":{"autoHit":false,"missThreshold":1700},` +
	`"critDone":13.94,"critTaken":18.94}}],` +
	`"armor":{"command":"armor","probeDamage":10000,"scenarios":[{"auras":[],"armor":8514,"damageAfterArmor":6843,` +
	`"multiplier":0.6843}]}}`

// A line as the e2e logs it reads into the replay's numbers.
func TestBuffLineShape(t *testing.T) {
	file := filepath.Join(t.TempDir(), buffsFixture)
	if err := os.WriteFile(file, []byte(buffBattleShout+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := readJSONLines[buffLine](file)
	if err != nil || len(lines) != 1 {
		t.Fatalf("%d lines, %v", len(lines), err)
	}
	l := lines[0]
	if l.State != stateName(l.Keys) || l.Setup != buffsSetup || !slices.Equal(l.Missing, []int32{47436}) {
		t.Errorf("state %s, keys %v, setup %s, missing %v", l.State, l.Keys, l.Setup, l.Missing)
	}
	sheet, err := l.Info.sheet()
	if err != nil {
		t.Fatal(err)
	}
	if mp5, ok := l.Info.mp5(); !ok || math.Abs(mp5-92) > 1e-9 {
		t.Errorf("mp5 %v %v, want 92", mp5, ok)
	}
	want := map[string]float64{"attack power": 4029.5598, "melee haste": 1 / 0.84135175, "spell haste": 1 / 0.92548686,
		"agility": 1053, "max energy": 100}
	for name, value := range want {
		if math.Abs(sheet[name]-value) > 1e-9 {
			t.Errorf("sheet %s %v, want %v", name, sheet[name], value)
		}
	}
	dummy := l.Info.dummy()
	wantDummy := map[string]float64{"dummy armor": 8514, "dummy attack power": 232, "dummy melee haste": 1 / 1.2,
		"dummy melee hit %": -3, "dummy fire resistance": -165, "dummy arcane resistance": -165}
	for name, value := range wantDummy {
		if math.Abs(dummy[name]-value) > 1e-9 {
			t.Errorf("%s %v, want %v", name, dummy[name], value)
		}
	}
	rec, ok := l.damageOf(57965)
	if !ok || len(rec.Effects) != 1 || rec.Effects[0].Taken != (damageRange{Min: 873, Max: 997}) || rec.AttackPower.Value != 4029 {
		t.Errorf("damage %+v", rec)
	}
	if len(l.Yellow) != 1 || l.Yellow[0].Derived.CritTaken != 18.94 || l.Yellow[0].Derived.Magic.MissThreshold != 1700 {
		t.Errorf("yellow %+v", l.Yellow)
	}
	if l.Armor == nil || len(l.Armor.Scenarios) != 1 || l.Armor.Scenarios[0].Multiplier != 0.6843 {
		t.Errorf("armor %+v", l.Armor)
	}
}
