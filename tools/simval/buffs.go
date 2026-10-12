package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	simpkg "github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// mod-sim-validation's TestSimvalBuffs puts each switch of the full buffs and debuffs
// (sim/core/test_utils.go) on a golden setup's character or its boss dummy, alone and then in pairs
// within the sim's exclusive categories, and logs a BUFFS line per state. The replay holds each state's
// change from the bare character to the sim's change from no buffs to the same ones.

const (
	buffsFixture = "buffs.jsonl"
	// the golden setup the e2e builds
	buffsSetup = "RogueAssassination"
	buffsBare  = "bare"
)

// buffPrefixes name the full messages a state key's field is in, e.g. raid.battle_shout.
var buffPrefixes = []string{"raid", "party", "individual", "debuff"}

var buffMessages = map[string]googleProto.Message{
	"raid": core.FullRaidBuffs, "party": core.FullPartyBuffs, "individual": core.FullIndividualBuffs,
	"debuff": core.FullDebuffs,
}

// unprobedBuffs are the full switches the e2e's buffsEntries leave out, each with why.
var unprobedBuffs = map[string]string{
	"raid.demonic_pact_sp": "the spell power is 10% of the warlock's own (spell_warl_demonic_pact casts 48090 " +
		"with it), which the sim takes as a setting",
	"raid.thorns":            "Thorns (53307) only hurts melee attackers, and the dummy never attacks",
	"party.mana_tide_totems": "Mana Tide Totem (16190) gives mana over 12 s, nothing a probe reads",
	"individual.judgements_of_the_wise": "Replenishment (57669) is a periodic energize (spell_gen_replenishment), " +
		"outside the snapshot's regen",
	"individual.vampiric_touch": "Replenishment (57669) is a periodic energize (spell_gen_replenishment), " +
		"outside the snapshot's regen",
	"debuff.judgement_of_light":  "heals the attacker on a hit (20185): no sheet, damage or crit change",
	"debuff.judgement_of_wisdom": "gives the attacker mana on a hit (20186): no sheet, damage or crit change",
	"debuff.shadow_embrace":      "applyDebuffEffects never reads it, only the warlock's own code applies Shadow Embrace",
}

// buffPairsFile lists the pairs the e2e puts on together, from the sim's exclusive categories with two full
// switches in them: ExclusiveEffect categories and the either/or branches of applyBuffEffects. The e2e's
// TestSimvalBuffs reads the same file.
const buffPairsFile = "buffs_pairs.json"

var loadBuffPairs = sync.OnceValues(func() ([][2]string, error) {
	root, err := checkoutRoot()
	if err != nil {
		return nil, err
	}
	return readBuffPairs(filepath.Join(root, fixtureDir, buffPairsFile))
})

func readBuffPairs(path string) ([][2]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Pairs [][]string `json:"pairs"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	pairs := make([][2]string, len(file.Pairs))
	for i, pair := range file.Pairs {
		if len(pair) != 2 {
			return nil, fmt.Errorf("%s: pair %v isn't two keys", path, pair)
		}
		pairs[i] = [2]string{pair[0], pair[1]}
	}
	return pairs, nil
}

// buffHeldAuras are sim auras a key needs up from the start at full stacks, as the server's probes see
// them: the sim casts Bloodlust as a cooldown, puts Sunder Armor up over the fight's first seconds and
// Expose Armor 3 s in.
var buffHeldAuras = map[string]int32{"raid.bloodlust": 2825, "debuff.sunder_armor": 47467, "debuff.expose_armor": 8647}

// buffField is the full message's field a key names, which has to be on there.
func buffField(key string) (protoreflect.FieldDescriptor, error) {
	prefix, name, ok := strings.Cut(key, ".")
	full, known := buffMessages[prefix]
	if !ok || !known {
		return nil, fmt.Errorf("%s: want %s, a dot and a field", key, strings.Join(buffPrefixes, ", "))
	}
	m := full.ProtoReflect()
	fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	switch {
	case fd == nil:
		return nil, fmt.Errorf("%s: %s has no field %s", key, m.Descriptor().Name(), name)
	case !m.Has(fd):
		return nil, fmt.Errorf("%s is off in the full %s", key, m.Descriptor().Name())
	}
	return fd, nil
}

// fullBuffKeys are the keys of every switch the full messages turn on, in field order.
func fullBuffKeys() []string {
	var keys []string
	for _, prefix := range buffPrefixes {
		m := buffMessages[prefix].ProtoReflect()
		fields := m.Descriptor().Fields()
		for i := range fields.Len() {
			if fd := fields.Get(i); m.Has(fd) {
				keys = append(keys, prefix+"."+string(fd.Name()))
			}
		}
	}
	return keys
}

// buffState is one state the e2e records besides bare: the switches it turns on.
type buffState struct {
	name string
	keys []string
}

func newBuffState(keys ...string) buffState {
	return buffState{stateName(keys), keys}
}

// stateName is the keys joined with +, bare for none.
func stateName(keys []string) string {
	if len(keys) == 0 {
		return buffsBare
	}
	return strings.Join(keys, "+")
}

// buffStates are every full switch alone, unprobedBuffs left out, then the pairs.
func buffStates(pairs [][2]string) []buffState {
	var states []buffState
	for _, key := range fullBuffKeys() {
		if _, ok := unprobedBuffs[key]; !ok {
			states = append(states, newBuffState(key))
		}
	}
	for _, pair := range pairs {
		states = append(states, newBuffState(pair[0], pair[1]))
	}
	return states
}

// buffSet is the sim's buff and debuff messages with the keys' switches on, at their full values.
type buffSet struct {
	raid       *proto.RaidBuffs
	party      *proto.PartyBuffs
	individual *proto.IndividualBuffs
	debuffs    *proto.Debuffs
}

func buffSetOf(keys []string) (buffSet, error) {
	b := buffSet{&proto.RaidBuffs{}, &proto.PartyBuffs{}, &proto.IndividualBuffs{}, &proto.Debuffs{}}
	into := map[string]protoreflect.Message{
		"raid": b.raid.ProtoReflect(), "party": b.party.ProtoReflect(), "individual": b.individual.ProtoReflect(),
		"debuff": b.debuffs.ProtoReflect(),
	}
	for _, key := range keys {
		fd, err := buffField(key)
		if err != nil {
			return b, err
		}
		prefix, _, _ := strings.Cut(key, ".")
		into[prefix].Set(fd, buffMessages[prefix].ProtoReflect().Get(fd))
	}
	return b, nil
}

// newBuffSim is the setup's character as newGoldcharSim builds it, in a raid with the keys' buffs and
// against a dummy with their debuffs.
func newBuffSim(rec damageRecord, s goldcharSetup, keys []string) (d *damageSim, err error) {
	set, err := buffSetOf(keys)
	if err != nil {
		return nil, err
	}
	player, err := goldcharPlayer(s)
	if err != nil {
		return nil, err
	}
	player.Buffs = set.individual
	mirror := s.Auras
	// the sim registers Sniper Training's aura only for an uptime above 0
	if hunter := player.GetHunter(); hunter != nil && slices.ContainsFunc(rec.Attacker.Auras, func(a aura) bool { return a.ID == 64420 }) {
		hunter.Options.SniperTrainingUptime = 1
		mirror = append(slices.Clone(mirror), 53304)
	}

	defer func() {
		if r := recover(); r != nil {
			d, err = nil, fmt.Errorf("building the sim character: %v", r)
		}
	}()
	simpkg.RegisterAll()
	var targetStats stats.Stats
	if len(rec.Target.Resistances) > 0 {
		targetStats[stats.Armor] = rec.Target.Resistances[0]
	}
	worldBoss := rec.Target.WorldBoss
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(player, set.party, set.raid, set.debuffs),
		Encounter: &proto.Encounter{Duration: 180, Targets: []*proto.Target{{
			Level: rec.Target.Level, MobType: mobTypes[rec.Target.CreatureType], Stats: targetStats.ToFloatArray(),
			WorldBoss: &worldBoss, NoXp: true, MechanicImmuneMask: dummyMechanicImmunities(rec.Target.Entry),
		}}},
		SimOptions: &proto.SimOptions{RandomSeed: 1, Iterations: 1},
	})
	d = &damageSim{rec: rec, sim: sim, char: sim.Raid.Parties[0].Players[0].GetCharacter(), target: sim.Encounter.TargetUnits[0]}
	for _, id := range mirror {
		if aura := auraBySpell(d.char.GetAuras(), id); aura != nil {
			d.mirrored = append(d.mirrored, aura)
		} else {
			d.others = append(d.others, id)
		}
	}
	for _, key := range keys {
		id, ok := buffHeldAuras[key]
		if !ok {
			continue
		}
		aura := cmp.Or(auraBySpell(d.char.GetAuras(), id), auraBySpell(d.target.GetAuras(), id))
		if aura == nil {
			return nil, fmt.Errorf("%s: no sim aura %d on the character or the dummy", key, id)
		}
		holdUp(aura)
	}
	return d, nil
}

// holdUp puts the aura on at every start, at full stacks. A stronger aura of its exclusive category can
// still keep it off.
func holdUp(aura *core.Aura) {
	aura.Duration = core.NeverExpires
	aura.OnReset = func(aura *core.Aura, sim *core.Simulation) {
		aura.Activate(sim)
		if aura.IsActive() && aura.MaxStacks > 0 {
			aura.SetStacks(sim, aura.MaxStacks)
		}
	}
}

// buffLine is one BUFFS line: a state's info record and the setup's probes in it. The e2e keeps only
// the parts of the damage, yellow and armor records the replay reads; the info record has the units.
type buffLine struct {
	Setup string   `json:"setup"`
	Bot   string   `json:"bot"`
	State string   `json:"state"`
	Keys  []string `json:"keys"`
	// the state's auras the e2e didn't find on, and auras on that it didn't expect
	Missing []int32        `json:"missing"`
	Extra   []int32        `json:"extra"`
	Info    buffInfo       `json:"info"`
	Damage  []damageRecord `json:"damage"`
	Yellow  []buffYellow   `json:"yellow"`
	Armor   *buffArmor     `json:"armor"`
}

// buffYellow is a `.simval yellow <id> 1` record's derived table.
type buffYellow struct {
	Spell struct {
		ID int32 `json:"id"`
	} `json:"spell"`
	Derived spellDerived `json:"derived"`
}

// buffArmor is a `.simval armor` record. Its first scenario is the dummy as it is.
type buffArmor struct {
	ProbeDamage float64 `json:"probeDamage"`
	Scenarios   []struct {
		Armor      float64 `json:"armor"`
		Multiplier float64 `json:"multiplier"`
	} `json:"scenarios"`
}

// buffInfo is the state's info record, with the regen and the dummy's attack speed besides what the
// golden characters read.
type buffInfo struct {
	goldcharRecord
	regen *timingRegen
	// the dummy's main hand attack time multiplier, 1 / its haste
	dummySpeed float64
}

func (b *buffInfo) UnmarshalJSON(data []byte) error {
	if err := b.goldcharRecord.UnmarshalJSON(data); err != nil {
		return err
	}
	var extra struct {
		Attacker struct {
			Regen *timingRegen `json:"regen"`
		} `json:"attacker"`
		Target struct {
			Attacks []struct {
				Type            string  `json:"type"`
				SpeedMultiplier float64 `json:"speedMultiplier"`
			} `json:"attacks"`
		} `json:"target"`
	}
	if err := json.Unmarshal(data, &extra); err != nil {
		return err
	}
	b.regen = extra.Attacker.Regen
	for _, a := range extra.Target.Attacks {
		if a.Type == "mainhand" {
			b.dummySpeed = a.SpeedMultiplier
		}
	}
	return nil
}

// mp5 is the regen while casting times 5: the mp5 auras plus the base, with no spirit part for a
// character without regen while casting.
func (b buffInfo) mp5() (float64, bool) {
	if b.regen == nil || len(b.regen.Interrupted) == 0 {
		return 0, false
	}
	return 5 * b.regen.Interrupted[0], true
}

// buffDummyTolerance is how far each dummy number may move apart on the two sides: armor and attack power
// are whole numbers on the server.
var buffDummyTolerance = map[string]float64{
	"dummy armor": 1, "dummy attack power": 1, "dummy melee haste": 1e-4, "dummy melee hit %": 1e-3,
	"dummy fire resistance": 1e-3, "dummy nature resistance": 1e-3, "dummy frost resistance": 1e-3,
	"dummy shadow resistance": 1e-3, "dummy arcane resistance": 1e-3,
}

// dummy is the dummy's numbers a debuff moves, from the record's target.
func (b buffInfo) dummy() map[string]float64 {
	t := b.Target
	v := map[string]float64{
		"dummy attack power": t.attack("mainhand").AttackPower,
		"dummy melee hit %":  t.HitMods.Melee,
	}
	if len(t.Resistances) == len(magicSchools)+1 {
		v["dummy armor"] = t.Resistances[0]
		for i, name := range sweepResistances {
			v["dummy "+name] = t.Resistances[2+i]
		}
	}
	if b.dummySpeed > 0 {
		v["dummy melee haste"] = 1 / b.dummySpeed
	}
	return v
}

// buffSimSide is a state's sim numbers that don't need a probe's record.
type buffSimSide struct {
	sheet sweepSim
	mp5   float64
	dummy map[string]float64
}

func (d *damageSim) buffSide() buffSimSide {
	side := buffSimSide{sheet: d.sweepSim()}
	if side.sheet.err != nil {
		return side
	}
	d.start()
	u, s := d.target, d.target.GetStats()
	side.mp5 = d.char.GetStat(stats.MP5)
	side.dummy = map[string]float64{
		"dummy armor": u.Armor(), "dummy attack power": s[stats.AttackPower],
		// an enemy has no haste rating, so its speed is the multiplier alone
		"dummy melee haste": u.PseudoStats.MeleeSpeedMultiplier,
		"dummy melee hit %": -100 * u.PseudoStats.IncreasedMissChance,
	}
	// the server's resistance field stops at 0 (Object::SetStatInt32Value), so Curse of the Elements' -165
	// doesn't show on a dummy without resistance; the resist roll clamps at 0 the same on both sides
	for i, stat := range []stats.Stat{stats.FireResistance, stats.NatureResistance, stats.FrostResistance,
		stats.ShadowResistance, stats.ArcaneResistance} {
		side.dummy["dummy "+sweepResistances[i]] = max(s[stat], 0)
	}
	return side
}

// withRecord points the sim at a probe's record, the line's info giving the units.
func (d *damageSim) withRecord(rec damageRecord, info buffInfo) *timingSim {
	rec.Command = "damage"
	rec.Attacker, rec.Target = info.Attacker, info.Target
	d.rec = rec
	return &timingSim{damageSim: d}
}

// buffHit is the sim's crit chance (percent) and miss (basis points, only its change means anything) of
// the probe's spell against the dummy: the spell declaring the record's first probed effect, through the
// melee table when the server's derived table is a yellow one.
func (d *damageSim) buffHit(rec damageRecord, info buffInfo, yellow bool) (crit, miss float64, ok bool) {
	ts := d.withRecord(rec, info)
	var spell *core.Spell
	for _, e := range rec.Effects {
		if !e.probed() {
			continue
		}
		if decl := ts.declaration(e); decl != nil {
			spell = decl.spell
			break
		}
	}
	if spell == nil {
		return 0, 0, false
	}
	d.start()
	table := d.char.AttackTables[d.target.UnitIndex]
	if yellow {
		return spell.PhysicalCritChance(table) * 100, -spell.PhysicalHitChance(table) * core.MaxRollBP, true
	}
	return spell.SpellCritChance(d.target) * 100, spell.SpellChanceToMiss(table) * core.MaxRollBP, true
}

// addSheet adds a sheet comparison's checks and moved fields.
func (r *spellModResult) addSheet(step sweepStep) {
	for _, c := range step.checks {
		r.add(c, "")
	}
	r.moved = append(r.moved, step.moved...)
	if len(step.moved) > 0 && r.class == spellModMovesNothing {
		r.class = spellModAgrees
	}
}

// buffPair is a bot's bare line and one of its states, each with its sim.
type buffPair struct {
	bare, state       buffLine
	simBare, simState *damageSim
	sideBare, side    buffSimSide
}

// compareBuffState holds the state's change from bare to the sim's: the character's sheet and mp5, the
// dummy's numbers, each probe's amounts as ratios, the crit chance taken and miss of its derived table,
// and the armor probe's multiplier.
func compareBuffState(p buffPair) (r spellModResult) {
	r.class = spellModMovesNothing
	defer func() {
		if e := recover(); e != nil {
			r.add(fail("sim", fmt.Sprintf("panic: %v", e)), "")
		}
	}()
	if len(p.state.Missing) > 0 || len(p.state.Extra) > 0 {
		r.add(fail("server auras", fmt.Sprintf("the state's auras %v weren't on, %v were that it doesn't have",
			p.state.Missing, p.state.Extra)), "")
	}
	if err := cmp.Or(p.sideBare.sheet.err, p.side.sheet.err); err != nil {
		r.add(fail("sim character", err.Error()), "")
		return r
	}

	serverBare, errBare := p.bare.Info.sheet()
	serverState, errState := p.state.Info.sheet()
	if err := cmp.Or(errBare, errState); err != nil {
		r.add(fail("server sheet", err.Error()), "")
	} else {
		r.addSheet(compareSweepSheets(serverBare, serverState, p.sideBare.sheet, p.side.sheet))
	}
	if mb, ok1 := p.bare.Info.mp5(); ok1 {
		if ma, ok2 := p.state.Info.mp5(); ok2 {
			r.delta("mp5", mb, ma, p.sideBare.mp5, p.side.mp5, 0.01)
		}
	}
	db, da := p.bare.Info.dummy(), p.state.Info.dummy()
	for _, name := range slices.Sorted(maps.Keys(buffDummyTolerance)) {
		r.compare(name, db, da, p.sideBare.dummy, p.side.dummy, buffDummyTolerance[name])
	}

	compareBuffDamage(&r, p)
	compareBuffHits(&r, p)
	if a, b := p.bare.Armor, p.state.Armor; a != nil && b != nil && len(a.Scenarios) > 0 && len(b.Scenarios) > 0 {
		view := func(info buffInfo) record {
			return record{Attacker: info.Attacker.unitSnapshot, Target: info.Target.unitSnapshot}
		}
		r.delta("armor multiplier", a.Scenarios[0].Multiplier, b.Scenarios[0].Multiplier,
			armorMultiplier(view(p.bare.Info), p.sideBare.dummy["dummy armor"]),
			armorMultiplier(view(p.state.Info), p.side.dummy["dummy armor"]), maxMultiplier)
	}
	return r
}

// damageOf is the line's damage record of the spell.
func (l buffLine) damageOf(spell int32) (damageRecord, bool) {
	i := slices.IndexFunc(l.Damage, func(rec damageRecord) bool { return rec.Spell.ID == spell })
	if i < 0 {
		return damageRecord{}, false
	}
	return l.Damage[i], true
}

// compareBuffDamage holds each probed effect's amount after the done and taken bonuses, as a ratio of the
// state to bare, to the sim's.
func compareBuffDamage(r *spellModResult, p buffPair) {
	for _, sa := range p.state.Damage {
		sb, ok := p.bare.damageOf(sa.Spell.ID)
		switch {
		case !ok:
			r.skipped = append(r.skipped, fmt.Sprintf("damage %d: bare has no probe of it", sa.Spell.ID))
			continue
		case sb.Shaped != "" || sa.Shaped != "":
			r.skipped = append(r.skipped, fmt.Sprintf("damage %d: shaped %s", sa.Spell.ID, cmp.Or(sa.Shaped, sb.Shaped)))
			continue
		}
		for _, eb := range sb.Effects {
			i := slices.IndexFunc(sa.Effects, func(e damageEffect) bool { return e.Index == eb.Index && e.Kind == eb.Kind })
			if !eb.probed() || i < 0 {
				continue
			}
			ea := sa.Effects[i]
			name := fmt.Sprintf("%d e%d %s", sa.Spell.ID, eb.Index, eb.Kind)
			mb := p.simBare.withRecord(sb, p.bare.Info).spellModEffect(eb)
			ma := p.simState.withRecord(sa, p.state.Info).spellModEffect(ea)
			switch {
			case mb.decl == nil || ma.decl == nil:
				r.skipped = append(r.skipped, name+": no sim declaration")
				continue
			case mb.err != nil || ma.err != nil:
				r.skipped = append(r.skipped, fmt.Sprintf("%s: sim amount: %v", name, cmp.Or(mb.err, ma.err)))
				continue
			}
			r.ratio(name+" min", eb.Taken.Min, ea.Taken.Min, mb.lo, ma.lo, mb.boundLo, ma.boundLo)
			r.ratio(name+" max", eb.Taken.Max, ea.Taken.Max, mb.hi, ma.hi, mb.boundHi, ma.boundHi)
		}
	}
}

// compareBuffHits holds each probe's crit chance taken and miss to the sim's. The miss is the magic
// table's threshold or the yellow table's miss.
func compareBuffHits(r *spellModResult, p buffPair) {
	for _, ya := range p.state.Yellow {
		id := ya.Spell.ID
		i := slices.IndexFunc(p.bare.Yellow, func(y buffYellow) bool { return y.Spell.ID == id })
		recB, okB := p.bare.damageOf(id)
		recA, okA := p.state.damageOf(id)
		if i < 0 || !okB || !okA {
			r.skipped = append(r.skipped, fmt.Sprintf("yellow %d: no bare yellow or no damage probe to find the sim spell by", id))
			continue
		}
		yb := p.bare.Yellow[i]
		yellow := yb.Derived.Yellow != nil
		critB, missB, ok1 := p.simBare.buffHit(recB, p.bare.Info, yellow)
		critA, missA, ok2 := p.simState.buffHit(recA, p.state.Info, yellow)
		if !ok1 || !ok2 {
			r.skipped = append(r.skipped, fmt.Sprintf("yellow %d: no sim declaration", id))
			continue
		}
		name := fmt.Sprintf("%d ", id)
		// 0 is a spell the server never crits
		if yb.Derived.CritDone != 0 || ya.Derived.CritDone != 0 {
			r.delta(name+"crit taken", yb.Derived.CritTaken, ya.Derived.CritTaken, critB, critA, 0.02)
		}
		switch db, da := yb.Derived, ya.Derived; {
		case db.Magic != nil && da.Magic != nil && !db.Magic.AutoHit && !da.Magic.AutoHit:
			r.delta(name+"miss", float64(db.Magic.MissThreshold), float64(da.Magic.MissThreshold), missB, missA, maxBPDiff)
		case db.Yellow != nil && da.Yellow != nil && !db.Yellow.AlwaysHit && !da.Yellow.AlwaysHit:
			r.delta(name+"miss", float64(db.Yellow.MissBp), float64(da.Yellow.MissBp), missB, missA, maxBPDiff)
		}
	}
}

// buffDrop is a state of a setup, or one check of it, that disagrees with the sim, each with a row in the
// fixture README. An empty check drops the state.
type buffDrop struct {
	setup, state, check string
}

// replayBuffs compares every state of each bot's lines with the sim, against the bot's first bare line,
// and holds the bot to every state of buffStates. A later bare line is compared too, as a state with no
// buffs, so it catches a drift. A dropped state or check only logs its failures, and one that fails
// nothing fails the replay.
func replayBuffs(lines []buffLine, setups []goldcharSetup, pairs [][2]string, drops []buffDrop) spellModReport {
	report := spellModReport{counts: map[string]int{}}
	used := map[buffDrop]bool{}
	dropped := func(d buffDrop) bool {
		if slices.Contains(drops, d) {
			used[d] = true
			return true
		}
		return false
	}
	expected := buffStates(pairs)
	var bots []string
	byBot := map[string][]buffLine{}
	for _, l := range lines {
		if _, ok := byBot[l.Bot]; !ok {
			bots = append(bots, l.Bot)
		}
		byBot[l.Bot] = append(byBot[l.Bot], l)
	}

	var nothing []string
	for _, bot := range bots {
		botLines := byBot[bot]
		i := slices.IndexFunc(setups, func(s goldcharSetup) bool { return s.Name == botLines[0].Setup })
		if i < 0 {
			report.errorf("%s: no setup %s", bot, botLines[0].Setup)
			continue
		}
		setup := setups[i]
		b := slices.IndexFunc(botLines, func(l buffLine) bool { return l.State == buffsBare })
		if b < 0 {
			report.errorf("%s: no bare line", bot)
			continue
		}
		bare := botLines[b]
		info := bare.Info
		if info.Character == nil || !setup.builds(info.Attacker.Class, info.Attacker.Race, info.Character) {
			report.errorf("%s: the bare character isn't %s's", bot, setup.Name)
			continue
		}
		simBare, err := newBuffSim(info.damageRecord, setup, nil)
		if err != nil {
			report.errorf("%s: sim character: %v", bot, err)
			continue
		}
		sideBare := simBare.buffSide()

		seen := map[string]bool{}
		for j, l := range botLines {
			if j == b {
				continue
			}
			label := fmt.Sprintf("%s %s", setup.Name, l.State)
			if l.Setup != setup.Name || l.State != stateName(l.Keys) {
				report.errorf("%s: %s: setup %s, keys %v", bot, label, l.Setup, l.Keys)
				continue
			}
			known := l.State == buffsBare || slices.ContainsFunc(expected, func(s buffState) bool { return s.name == l.State })
			switch {
			case !known:
				report.errorf("%s: %s isn't a state the sim expects", bot, label)
				continue
			case l.State != buffsBare && seen[l.State]:
				report.errorf("%s: %s twice", bot, label)
				continue
			}
			seen[l.State] = true

			// the dummy's armor before any debuff is bare's
			simState, err := newBuffSim(info.damageRecord, setup, l.Keys)
			var result spellModResult
			if err != nil {
				result = spellModResult{class: spellModDisagrees, checks: []Check{fail("sim character", err.Error())}}
			} else {
				result = compareBuffState(buffPair{bare: bare, state: l, simBare: simBare, simState: simState,
					sideBare: sideBare, side: simState.buffSide()})
			}
			report.counts[result.class]++
			if result.class == spellModMovesNothing && l.State != buffsBare {
				nothing = append(nothing, label)
			}
			text := fmt.Sprintf("BUFFSIM %s: %s", label, result.class)
			if len(result.moved) > 0 {
				text += "; " + strings.Join(result.moved, ", ")
			}
			if len(result.skipped) > 0 {
				text += "; not compared: " + strings.Join(result.skipped, ", ")
			}
			report.logf("%s", text)

			stateDropped := dropped(buffDrop{setup.Name, l.State, ""})
			for _, c := range result.checks {
				checkDropped := dropped(buffDrop{setup.Name, l.State, c.Name})
				switch {
				case checkDropped && c.Passed:
					report.errorf("%s: %s passes, so it shouldn't be dropped: %s", label, c.Name, c.Detail)
				case !c.Passed && (checkDropped || stateDropped):
					report.logf("%s: %s (dropped): %s", label, c.Name, c.Detail)
				case !c.Passed:
					report.errorf("%s: %s: %s", label, c.Name, c.Detail)
				}
			}
			if stateDropped && result.class != spellModDisagrees {
				report.errorf("%s %s, so it shouldn't be dropped", label, result.class)
			}
		}
		for _, s := range expected {
			if !seen[s.name] && !dropped(buffDrop{setup.Name, s.name, ""}) {
				report.errorf("%s: no line for %s %s", bot, setup.Name, s.name)
			}
		}
	}
	for _, d := range drops {
		if !used[d] {
			report.errorf("%+v is dropped, but no state of the lines matches it", d)
		}
	}
	report.logf("%d states agree, %d move nothing, %d disagree", report.counts[spellModAgrees],
		report.counts[spellModMovesNothing], report.counts[spellModDisagrees])
	report.logf("moving nothing on either side: %s", strings.Join(nothing, ", "))
	return report
}
