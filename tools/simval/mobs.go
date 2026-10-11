package main

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// mod-sim-validation's TestSimvalMobs puts a warrior, a paladin, a hunter and a druid through the creature-type
// effects the sim implements, one state each, on a dummy of every type, and logs a MOBS line per state and dummy.
// The replay holds each line's change from the same state on the mechanical dummy, a type none of the effects
// names, to the sim's change between targets of those two types.

const (
	mobsFixture = "mobs.jsonl"
	mobsBare    = "bare"
	// a line on the reference dummy that fails nothing
	mobsReferenceClass = "reference"
)

// The server's CreatureType ids of the dummies' types.
const (
	mobBeast      uint32 = 1
	mobDragonkin  uint32 = 2
	mobDemon      uint32 = 3
	mobElemental  uint32 = 4
	mobGiant      uint32 = 5
	mobUndead     uint32 = 6
	mobHumanoid   uint32 = 7
	mobMechanical uint32 = 9
)

// mobDummies are the e2e's dummies by entry. The giant is the level 80 one, since the giant boss adds world boss
// rules.
var mobDummies = map[uint32]uint32{
	999001: mobHumanoid, 999002: mobBeast, 999003: mobGiant, 999004: mobUndead, 999005: mobDemon,
	999006: mobElemental, 999007: mobDragonkin, 999008: mobMechanical,
}

// mobReference is the type each line's change is taken from.
const mobReference = mobMechanical

// mobTypedProbes are probes the server lands on some types only, each with the type its changes are taken from.
// Holy Wrath (TargetCreatureType 36) hits demons and undead; the bare `threat` selection covers the rest.
var mobTypedProbes = map[int32]struct {
	types     []uint32
	reference uint32
}{
	48817: {[]uint32{mobDemon, mobUndead}, mobDemon},
}

// mobBot is one of the e2e's bots and its states.
type mobBot struct {
	name   string
	states []string
	// the main hand the e2e gives it. The info record leaves it out in a druid form (no weapon used there), but
	// Predatory Strikes still reads it
	mainHand int32
}

var mobBots = []mobBot{
	{"HumanWarrior", []string{mobsBare, "scourgebane", "demonSlaying", "elixirOfDemonslaying", "undeadSlaying2",
		"undeadSlaying3", "undeadSlaying4"}, 40384},
	{"HumanPaladin", []string{mobsBare, "crusade", "senseUndead", "scourgebane"}, 40384},
	{"TrollHunter", []string{mobsBare, "improvedTracking", "scourgebane"}, 40388},
	{"NightElfDruid", []string{mobsBare, "scourgebane"}, 40388},
}

func mobBotNamed(name string) (mobBot, bool) {
	i := slices.IndexFunc(mobBots, func(b mobBot) bool { return b.name == name })
	if i < 0 {
		return mobBot{}, false
	}
	return mobBots[i], true
}

// mobMirror is what the sim character needs for a state beyond its line, which gives the talents and glyphs (the
// info record's auras), the race and the worn items.
type mobMirror struct {
	player func(*proto.Player)
	// runs at every reset, after the character's pseudo stats are put back
	reset func(*core.Character)
}

var mobMirrors = map[string]mobMirror{
	// the sim only has Scourgebane as the main hand's enchant; the server state is its aura 44594, or for the
	// druid the enchant itself
	"scourgebane": {player: mobMainHandEnchant(3247)},
	// the sim gives Demon Slaying 200 only with the Twin Blades of Azzinoth's two glaives in hand, which would
	// replace the probed weapon, so the state adds what that set bonus adds
	"demonSlaying": {reset: func(c *core.Character) {
		if c.CurrentTarget.MobType == proto.MobType_MobTypeDemon {
			c.PseudoStats.MobTypeAttackPower += 200
		}
	}},
	"elixirOfDemonslaying": {player: func(p *proto.Player) {
		p.Consumes.BattleElixir = proto.BattleElixir_ElixirOfDemonslaying
	}},
}

func mobMainHandEnchant(id int32) func(*proto.Player) {
	return func(p *proto.Player) { p.Equipment.Items[proto.ItemSlot_ItemSlotMainHand].Enchant = id }
}

// wearMobItems puts the items in the player's empty slots: the sim places each by its type, whatever the slot.
func wearMobItems(p *proto.Player, items []int32) {
	free := 0
	for _, id := range items {
		for free < len(p.Equipment.Items) && p.Equipment.Items[free].Id != 0 {
			free++
		}
		if free == len(p.Equipment.Items) {
			p.Equipment.Items = append(p.Equipment.Items, &proto.ItemSpec{})
		}
		p.Equipment.Items[free].Id = id
	}
}

// mobLine is one MOBS line: a state of a bot on a dummy, with the parts of each probe's record the replay reads.
// The info record has the units.
type mobLine struct {
	Bot          string `json:"bot"`
	Dummy        uint32 `json:"dummy"`
	CreatureType uint32 `json:"creatureType"`
	State        string `json:"state"`
	// the item ids it wears beyond its weapons
	Items []int32 `json:"items"`
	// the state's auras the e2e didn't find on, and other states' it did
	Missing []int32        `json:"missing"`
	Extra   []int32        `json:"extra"`
	Info    damageRecord   `json:"info"`
	Damage  []damageRecord `json:"damage"`
	Yellow  []buffYellow   `json:"yellow"`
	Melee   *mobMelee      `json:"melee"`
	// bare only: the `spell` crit roll and the `threat` target selection
	Spell  []buffYellow   `json:"spell"`
	Threat []threatRecord `json:"threat"`
}

// mobMelee is a `.simval melee` record's white table and outcomes.
type mobMelee struct {
	Derived  whiteTable         `json:"derived"`
	Outcomes map[string]counted `json:"outcomes"`
}

type mobKey struct {
	bot, state string
	dummy      uint32
}

func (l mobLine) key() mobKey { return mobKey{l.Bot, l.State, l.Dummy} }

func (k mobKey) String() string {
	return fmt.Sprintf("%s %s on %d (%s)", k.bot, k.state, k.dummy, mobTypeName(mobDummies[k.dummy]))
}

func mobTypeName(creatureType uint32) string {
	return strings.TrimPrefix(mobTypes[creatureType].String(), "MobType")
}

func (l mobLine) damageOf(spell int32) (damageRecord, bool) {
	i := slices.IndexFunc(l.Damage, func(rec damageRecord) bool { return rec.Spell.ID == spell })
	if i < 0 {
		return damageRecord{}, false
	}
	return l.Damage[i], true
}

// record is the probe's record with the line's units in it.
func (l mobLine) record(probe damageRecord) damageRecord {
	probe.Command = "damage"
	probe.Attacker, probe.Target = l.Info.Attacker, l.Info.Target
	return probe
}

// characterProbe is the probe whose sim character the white swings and attack power are read off: the first
// weapon strike, whose spec swings the weapons, else the first probe.
func (l mobLine) characterProbe() (damageRecord, bool) {
	for _, rec := range l.Damage {
		if rec.Weapon != nil {
			return rec, true
		}
	}
	if len(l.Damage) > 0 {
		return l.Damage[0], true
	}
	return damageRecord{}, false
}

// mobBuilt is a line's sim character for one probe, with the strike's sim spell for a weapon strike.
type mobBuilt struct {
	d      *damageSim
	strike *core.Spell
	err    error
}

type mobSimKey struct {
	mobKey
	spell int32
	extra string
}

// mobReplay holds a run's lines by key and the sim characters built for them.
type mobReplay struct {
	lines map[mobKey]mobLine
	sims  map[mobSimKey]mobBuilt
}

func newMobReplay(lines []mobLine) *mobReplay {
	m := &mobReplay{lines: map[mobKey]mobLine{}, sims: map[mobSimKey]mobBuilt{}}
	for _, l := range lines {
		m.lines[l.key()] = l
	}
	return m
}

func (m *mobReplay) sim(l mobLine, probe damageRecord) mobBuilt {
	return m.simWith(l, probe, "", nil)
}

// simWith is sim with extra put on the player too, cached under its name.
func (m *mobReplay) simWith(l mobLine, probe damageRecord, name string, extra func(*proto.Player)) mobBuilt {
	k := mobSimKey{l.key(), probe.Spell.ID, name}
	if b, ok := m.sims[k]; ok {
		return b
	}
	d, strike, err := buildMobSim(l, probe, extra)
	b := mobBuilt{d: d, strike: strike, err: err}
	m.sims[k] = b
	return b
}

// buildMobSim builds the line's character for one of its probes: its class, race, talents and glyphs, the
// line's weapons and items, the state's mirror, against a dummy of the line's type. Specs and teaching talents
// are tried as the damage and strike replays try them, keeping a spec that swings its weapons.
func buildMobSim(l mobLine, probe damageRecord, extra func(*proto.Player)) (*damageSim, *core.Spell, error) {
	rec := l.record(probe)
	setup, err := damageSetupFor(rec)
	if err != nil {
		return nil, nil, err
	}
	strike := rec.Weapon != nil
	taught := talentTaughtSpells
	if strike {
		taught = strikeTaughtSpells
	}
	setups := []damageSetup{setup}
	if field, ok := taught[rec.Spell.ID]; ok {
		if s, ok := setup.withTalent(rec.Attacker.Class, field); ok {
			setups = append(setups, s)
		}
	}
	class := classes[rec.Attacker.Class]
	specs := slices.Concat([]protoreflect.Name{class.spec}, class.otherSpecs, strikeExtraSpecs[rec.Attacker.Class])
	mirror := mobMirrors[l.State]
	for _, setup := range setups {
		for _, spec := range specs {
			player, err := strikePlayer(rec, setup, spec)
			if err != nil {
				return nil, nil, err
			}
			// a white swing's rage asks the rotation for its next action
			player.Rotation = &proto.APLRotation{}
			if mh := player.Equipment.Items[proto.ItemSlot_ItemSlotMainHand]; mh.Id == 0 {
				if b, ok := mobBotNamed(l.Bot); ok {
					mh.Id = b.mainHand
				}
			}
			wearMobItems(player, l.Items)
			if mirror.player != nil {
				mirror.player(player)
			}
			if extra != nil {
				extra(player)
			}
			d, err := newDamageSim(rec, player, setup.others, 1)
			if err != nil {
				return nil, nil, err
			}
			if mirror.reset != nil {
				d.char.RegisterResetEffect(func(*core.Simulation) { mirror.reset(d.char) })
			}
			// a spec that never swings leaves its weapons empty
			if d.strikeHand().AttackPowerPerDPS == 0 {
				continue
			}
			if strike {
				if spell := d.strikeDeclaration(); spell != nil {
					return d, spell, nil
				}
				continue
			}
			if slices.ContainsFunc(rec.probedEffects(), func(e damageEffect) bool { return d.declaration(e) != nil }) {
				return d, nil, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("no %v spec declares %d with a weapon it swings", specs, rec.Spell.ID)
}

// hitSpell is the sim spell that deals the probe: the strike, else the one declaring its first probed effect.
func (b mobBuilt) hitSpell(rec damageRecord) *core.Spell {
	if b.strike != nil {
		return b.strike
	}
	for _, e := range rec.probedEffects() {
		if decl := b.d.declaration(e); decl != nil {
			return decl.spell
		}
	}
	return nil
}

// reference is the same bot's line in the same state on the dummy of that type.
func (m *mobReplay) reference(l mobLine, creatureType uint32) (mobLine, bool) {
	for entry, t := range mobDummies {
		if t == creatureType {
			ref, ok := m.lines[mobKey{l.Bot, l.State, entry}]
			return ref, ok
		}
	}
	return mobLine{}, false
}

// probeReference is the line a probe's change on l is taken from, or why there's none.
func (m *mobReplay) probeReference(l mobLine, spell int32) (mobLine, string) {
	refType := mobReference
	if typed, ok := mobTypedProbes[spell]; ok {
		if !slices.Contains(typed.types, l.CreatureType) {
			names := make([]string, len(typed.types))
			for i, t := range typed.types {
				names[i] = mobTypeName(t)
			}
			return mobLine{}, fmt.Sprintf("%d: lands on %s only", spell, strings.Join(names, " and "))
		}
		refType = typed.reference
	}
	if l.CreatureType == refType {
		return mobLine{}, ""
	}
	ref, ok := m.reference(l, refType)
	if !ok {
		return mobLine{}, fmt.Sprintf("%d: no %s line on the %s dummy", spell, l.State, mobTypeName(refType))
	}
	return ref, ""
}

// compare holds the line's change from its reference line to the sim's: each probe's amounts as ratios, the
// crit chance taken, white parry and plain swing damage, and attack power. A bare line's target selections are
// held to the sim's as they are.
func (m *mobReplay) compare(l mobLine) (r spellModResult) {
	r.class = spellModMovesNothing
	defer func() {
		if e := recover(); e != nil {
			r.add(fail("sim", fmt.Sprintf("panic: %v", e)), "")
		}
	}()
	if len(l.Missing) > 0 || len(l.Extra) > 0 {
		r.add(fail("server auras", fmt.Sprintf("the state's auras %v weren't on, other states' %v were",
			l.Missing, l.Extra)), "")
	}
	m.compareSelections(&r, l)
	m.compareDamage(&r, l)
	m.compareCrits(&r, l)
	if l.CreatureType == mobReference {
		return r
	}
	ref, ok := m.reference(l, mobReference)
	if !ok {
		r.skipped = append(r.skipped, fmt.Sprintf("white and attack power: no %s line on the %s dummy", l.State,
			mobTypeName(mobReference)))
		return r
	}
	m.compareCharacter(&r, l, ref)
	return r
}

func (m *mobReplay) compareDamage(r *spellModResult, l mobLine) {
	for _, rec := range l.Damage {
		id := rec.Spell.ID
		ref, why := m.probeReference(l, id)
		if why != "" {
			r.skipped = append(r.skipped, why)
		}
		if ref.Bot == "" {
			continue
		}
		refRec, ok := ref.damageOf(id)
		switch {
		case !ok:
			r.skipped = append(r.skipped, fmt.Sprintf("%d: its reference line has no probe of it", id))
		case rec.Weapon != nil && refRec.Weapon != nil:
			m.compareStrike(r, l, ref, rec, refRec)
		case rec.Shaped != "" || refRec.Shaped != "":
			r.skipped = append(r.skipped, fmt.Sprintf("%d: shaped %s", id, cmp.Or(rec.Shaped, refRec.Shaped)))
		default:
			m.compareEffects(r, l, ref, rec, refRec)
		}
	}
}

// strikeAmounts is the sim's strike at both ends of the roll and how far the server's may sit from each.
func (b mobBuilt) strikeAmounts(w *weaponStrike) (lo, hi, boundLo, boundHi float64, err error) {
	lo, errLo := b.d.strikeAmount(b.strike, w, 0)
	hi, errHi := b.d.strikeAmount(b.strike, w, 1)
	if err := firstErr(errLo, errHi); err != nil {
		return 0, 0, 0, 0, err
	}
	return lo, hi, amountTolerance(w.Taken.Min), amountTolerance(w.Taken.Max), nil
}

func (m *mobReplay) compareStrike(r *spellModResult, l, ref mobLine, rec, refRec damageRecord) {
	name := fmt.Sprintf("%d weapon", rec.Spell.ID)
	w, wr := rec.Weapon, refRec.Weapon
	if w.Shaped != "" || wr.Shaped != "" {
		r.skipped = append(r.skipped, fmt.Sprintf("%s: shaped %s", name, cmp.Or(w.Shaped, wr.Shaped)))
		return
	}
	t, b := m.sim(l, rec), m.sim(ref, refRec)
	if err := cmp.Or(t.err, b.err); err != nil {
		r.add(fail(name+" sim", err.Error()), "")
		return
	}
	lo, hi, boundLo, boundHi, errT := t.strikeAmounts(w)
	refLo, refHi, refBoundLo, refBoundHi, errR := b.strikeAmounts(wr)
	if err := firstErr(errT, errR); err != nil {
		r.add(fail(name+" sim", err.Error()), "")
		return
	}
	r.ratio(name+" min", wr.Taken.Min, w.Taken.Min, refLo, lo, refBoundLo, boundLo)
	r.ratio(name+" max", wr.Taken.Max, w.Taken.Max, refHi, hi, refBoundHi, boundHi)
}

func (m *mobReplay) compareEffects(r *spellModResult, l, ref mobLine, rec, refRec damageRecord) {
	t, b := m.sim(l, rec), m.sim(ref, refRec)
	if err := cmp.Or(t.err, b.err); err != nil {
		r.add(fail(fmt.Sprintf("%d sim", rec.Spell.ID), err.Error()), "")
		return
	}
	for _, eb := range refRec.Effects {
		i := slices.IndexFunc(rec.Effects, func(e damageEffect) bool { return e.Index == eb.Index && e.Kind == eb.Kind })
		if !eb.probed() || i < 0 {
			continue
		}
		ea := rec.Effects[i]
		name := fmt.Sprintf("%d e%d %s", rec.Spell.ID, eb.Index, eb.Kind)
		mb := (&timingSim{damageSim: b.d}).spellModEffect(eb)
		ma := (&timingSim{damageSim: t.d}).spellModEffect(ea)
		switch {
		case mb.decl == nil || ma.decl == nil:
			r.skipped = append(r.skipped, name+": no sim declaration")
			continue
		case mb.err != nil || ma.err != nil:
			r.add(fail(name+" sim", cmp.Or(mb.err, ma.err).Error()), "")
			continue
		}
		r.ratio(name+" min", eb.Taken.Min, ea.Taken.Min, mb.lo, ma.lo, mb.boundLo, ma.boundLo)
		r.ratio(name+" max", eb.Taken.Max, ea.Taken.Max, mb.hi, ma.hi, mb.boundHi, ma.boundHi)
	}
}

// mobCritTolerance is how far the crit chance taken may sit from the sim's, in percent.
const mobCritTolerance = 0.02

// compareCrits holds each `yellow` and `spell` probe's crit chance taken to the server's on the reference line
// moved by the sim's change. Both stop at 100 and 0, so Exorcism's certain crit on undead reads 100 on both.
func (m *mobReplay) compareCrits(r *spellModResult, l mobLine) {
	for _, list := range []struct {
		suffix string
		hits   func(mobLine) []buffYellow
	}{
		{"crit taken", func(l mobLine) []buffYellow { return l.Yellow }},
		{"spell crit taken", func(l mobLine) []buffYellow { return l.Spell }},
	} {
		for _, h := range list.hits(l) {
			id := h.Spell.ID
			name := fmt.Sprintf("%d %s", id, list.suffix)
			ref, why := m.probeReference(l, id)
			if why != "" {
				r.skipped = append(r.skipped, why)
			}
			if ref.Bot == "" {
				continue
			}
			i := slices.IndexFunc(list.hits(ref), func(y buffYellow) bool { return y.Spell.ID == id })
			rec, okT := l.damageOf(id)
			refRec, okR := ref.damageOf(id)
			if i < 0 || !okT || !okR {
				r.skipped = append(r.skipped, name+": no reference probe or no damage probe to find the sim spell by")
				continue
			}
			hr := list.hits(ref)[i]
			if h.Derived.CritDone == 0 && hr.Derived.CritDone == 0 {
				continue
			}
			yellow := h.Derived.Yellow != nil
			simT, errT := m.crit(l, rec, yellow)
			simR, errR := m.crit(ref, refRec, yellow)
			if err := firstErr(errT, errR); err != nil {
				r.add(fail(name, "sim: "+err.Error()), "")
				continue
			}
			sb, sa := hr.Derived.CritTaken, h.Derived.CritTaken
			want := min(100, max(0, sb+simT-simR))
			detail := fmt.Sprintf("server %g -> %g, sim %.3f -> %.3f, so %.3f (within %g)", sb, sa, simR, simT, want,
				mobCritTolerance)
			c := fail(name, detail)
			if math.Abs(want-sa) <= mobCritTolerance+1e-9 {
				c = pass(name, detail)
			}
			moved := ""
			if math.Abs(sa-sb) > mobCritTolerance || math.Abs(simT-simR) > mobCritTolerance {
				moved = fmt.Sprintf("%s server %+.4g sim %+.4g", name, sa-sb, simT-simR)
			}
			r.add(c, moved)
		}
	}
}

// crit is the sim's crit chance (percent, not capped) for the probe's spell against the line's dummy.
func (m *mobReplay) crit(l mobLine, rec damageRecord, yellow bool) (float64, error) {
	b := m.sim(l, rec)
	if b.err != nil {
		return 0, b.err
	}
	spell := b.hitSpell(rec)
	if spell == nil {
		return 0, fmt.Errorf("no sim spell deals %d", rec.Spell.ID)
	}
	return b.d.mobCrit(spell, yellow)
}

// mobCrit is the crit chance the spell rolls against the dummy (percent), read while its own effects run, so a
// bonus a spell adds for its target's type (Exorcism) counts: melee crit for a yellow table, spell crit
// otherwise.
func (d *damageSim) mobCrit(spell *core.Spell, yellow bool) (crit float64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	hooks := d.target.DynamicDamageTakenModifiers
	defer func() { d.target.DynamicDamageTakenModifiers = hooks }()
	read := false
	d.target.DynamicDamageTakenModifiers = append(slices.Clip(hooks), func(_ *core.Simulation, s *core.Spell, result *core.SpellResult) {
		if s != spell || read {
			return
		}
		read = true
		if yellow {
			crit = s.PhysicalCritChance(s.Unit.AttackTables[result.Target.UnitIndex])
		} else {
			crit = s.SpellCritChance(result.Target)
		}
	})
	d.prepare(d.rec.SpellPower.Damage)
	spell.ApplyEffects(d.sim, d.target, spell)
	if !read {
		return 0, fmt.Errorf("%s dealt nothing to read its crit by", spell.ActionID)
	}
	return crit * 100, nil
}

// compareCharacter holds the white table's parry, a plain white swing's mean damage and the attack power to the
// sim's, as changes from the reference line.
func (m *mobReplay) compareCharacter(r *spellModResult, l, ref mobLine) {
	probe, okT := l.characterProbe()
	refProbe, okR := ref.characterProbe()
	if !okT || !okR {
		r.skipped = append(r.skipped, "white and attack power: no probe to build the sim character by")
		return
	}
	t, b := m.sim(l, probe), m.sim(ref, refProbe)
	if err := cmp.Or(t.err, b.err); err != nil {
		r.add(fail("character sim", err.Error()), "")
		return
	}

	for _, a := range []struct {
		name, kind string
		stat       stats.Stat
	}{{"attack power", "mainhand", stats.AttackPower}, {"ranged attack power", "ranged", stats.RangedAttackPower}} {
		simT, errT := t.d.mobStat(a.stat)
		simR, errR := b.d.mobStat(a.stat)
		if err := firstErr(errT, errR); err != nil {
			r.add(fail(a.name, "sim: "+err.Error()), "")
			continue
		}
		// the server's attack power is a whole number
		r.delta(a.name, ref.Info.Attacker.attack(a.kind).AttackPower, l.Info.Attacker.attack(a.kind).AttackPower, simR,
			simT, 1)
	}

	switch {
	case l.Melee == nil || ref.Melee == nil:
		return
	case t.d.char.AutoAttacks.MHAuto() == nil || b.d.char.AutoAttacks.MHAuto() == nil:
		r.skipped = append(r.skipped, "white: the sim character never swings its main hand")
		return
	}
	parryT, errT := t.d.mobParry(l.Info.Attacker.attack("mainhand").ExpertiseReduction)
	parryR, errR := b.d.mobParry(ref.Info.Attacker.attack("mainhand").ExpertiseReduction)
	if err := firstErr(errT, errR); err != nil {
		r.add(fail("white parry", "sim: "+err.Error()), "")
	} else {
		r.delta("white parry", float64(ref.Melee.Derived.ParryBp), float64(l.Melee.Derived.ParryBp), float64(parryR),
			float64(parryT), maxBPDiff)
	}

	nT, nR := l.Melee.Outcomes["normal"], ref.Melee.Outcomes["normal"]
	if nT.Count < 30 || nR.Count < 30 {
		r.skipped = append(r.skipped, "white normal damage: under 30 plain hits")
		return
	}
	meanT, boundT, errT := t.d.mobWhiteMean(l, nT.Count)
	meanR, boundR, errR := b.d.mobWhiteMean(ref, nR.Count)
	if err := firstErr(errT, errR); err != nil {
		r.add(fail("white normal damage", "sim: "+err.Error()), "")
		return
	}
	var white spellModResult
	white.ratio("white normal damage", nR.MeanDamage, nT.MeanDamage, meanR, meanT, boundR, boundT)
	// the server's means are samples, so a change inside their bounds is noise, not a move
	server := nT.MeanDamage / nR.MeanDamage
	moved := ""
	if len(white.moved) > 0 && (math.Abs(meanT/meanR-1) > 1e-6 ||
		math.Abs(server-1) > server*(boundT/nT.MeanDamage+boundR/nR.MeanDamage)) {
		moved = white.moved[0]
	}
	r.add(white.checks[0], moved)
}

// mobStat is the character's own stat on the dummy, nothing pinned.
func (d *damageSim) mobStat(stat stats.Stat) (value float64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	d.start()
	return d.char.GetStat(stat), nil
}

// mobParry is the sim's white main-hand parry chance from the front (basis points), at the server's expertise.
func (d *damageSim) mobParry(expertisePct float64) (bp int32, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	mh := d.char.AutoAttacks.MHAuto()
	if mh == nil {
		return 0, fmt.Errorf("no main-hand swing")
	}
	d.start()
	in := mh.WhiteTableInput(d.char.AttackTables[d.target.UnitIndex])
	in.InFront = true
	in.ExpertisePct = float32(expertisePct)
	return core.WhiteMeleeTableBP(in).Parry, nil
}

// mobWhiteMean is the sim's mean plain main-hand swing after the dummy's armor, and how far the server's mean of
// count rolls may sit from it: 5 standard errors of the even roll, plus 2 points for its truncations.
func (d *damageSim) mobWhiteMean(l mobLine, count float64) (mean, bound float64, err error) {
	if len(l.Info.Target.Resistances) == 0 {
		return 0, 0, fmt.Errorf("the dummy has no armor in the record")
	}
	lo, errLo := d.mobSwing(0)
	hi, errHi := d.mobSwing(1)
	if err := firstErr(errLo, errHi); err != nil {
		return 0, 0, err
	}
	view := record{Attacker: l.Info.Attacker.unitSnapshot, Target: l.Info.Target.unitSnapshot}
	armor := armorMultiplier(view, l.Info.Target.Resistances[0])
	stderr := (hi - lo) * armor / math.Sqrt(12*count)
	return (lo + hi) / 2 * armor, maxZ*stderr + 2, nil
}

// mobSwing is a plain main-hand swing (no crit, glance or block) at one end of the weapon's roll, before armor,
// read when the damage is calculated.
func (d *damageSim) mobSwing(end int) (amount float64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	mh := d.char.AutoAttacks.MHAuto()
	if mh == nil {
		return 0, fmt.Errorf("no main-hand swing")
	}
	flags, hooks := mh.Flags, d.target.DynamicDamageTakenModifiers
	hit, crit := mh.BonusHitRating, mh.BonusCritRating
	defer func() {
		mh.Flags, d.target.DynamicDamageTakenModifiers = flags, hooks
		mh.BonusHitRating, mh.BonusCritRating = hit, crit
	}()
	mh.Flags |= core.SpellFlagIgnoreResists
	landed := false
	d.target.DynamicDamageTakenModifiers = append(slices.Clip(hooks), func(_ *core.Simulation, s *core.Spell, result *core.SpellResult) {
		if s == mh && !landed && result.Outcome == core.OutcomeHit {
			amount, landed = result.Damage, true
		}
	})
	unit := &d.char.Unit
	for range maxHitAttempts {
		d.prepare(d.rec.SpellPower.Damage)
		mh.BonusHitRating, mh.BonusCritRating = hit+forcedHitRating, crit-forcedCritRating
		restore := pinWeapons(unit, end)
		mh.ApplyEffects(d.sim, d.target, mh)
		restore()
		if landed {
			return amount, nil
		}
	}
	return 0, fmt.Errorf("no plain hit in %d swings", maxHitAttempts)
}

// mobSelections tell whether the sim lands a bare `threat` probe's spell on the line's dummy.
var mobSelections = map[int32]func(m *mobReplay, l mobLine) (bool, error){
	// Holy Wrath: the sim's cast misses all but demons and undead
	48817: func(m *mobReplay, l mobLine) (bool, error) {
		rec, ok := l.damageOf(48817)
		if !ok {
			return false, fmt.Errorf("no damage probe of 48817 to build the sim spell by")
		}
		b := m.sim(l, rec)
		if b.err != nil {
			return false, b.err
		}
		spell := b.hitSpell(rec)
		if spell == nil {
			return false, fmt.Errorf("no sim spell deals 48817")
		}
		return b.d.mobLands(spell)
	},
	// Tendon Rip: Giant Slayer's proc, which the sim fires on giants only
	44622: func(m *mobReplay, l mobLine) (bool, error) {
		probe, ok := l.characterProbe()
		if !ok {
			return false, fmt.Errorf("no probe to build the sim character by")
		}
		b := m.simWith(l, probe, "giantSlayer", mobMainHandEnchant(3251))
		if b.err != nil {
			return false, b.err
		}
		return b.d.mobProcs(44622, 200)
	},
}

// mobLands is whether the spell lands on the dummy at all with a forced hit.
func (d *damageSim) mobLands(spell *core.Spell) (landed bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	hooks, hit := d.target.DynamicDamageTakenModifiers, spell.BonusHitRating
	defer func() { d.target.DynamicDamageTakenModifiers, spell.BonusHitRating = hooks, hit }()
	d.target.DynamicDamageTakenModifiers = append(slices.Clip(hooks), func(_ *core.Simulation, s *core.Spell, result *core.SpellResult) {
		landed = landed || s == spell && result.Landed()
	})
	for range maxHitAttempts {
		d.prepare(d.rec.SpellPower.Damage)
		spell.BonusHitRating = hit + forcedHitRating
		spell.ApplyEffects(d.sim, d.target, spell)
		if landed {
			return true, nil
		}
	}
	return false, nil
}

// mobProcs is whether the sim casts the spell at the dummy within that many main-hand swings.
func (d *damageSim) mobProcs(id int32, swings int) (procs bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	proc := spellBySpell(&d.char.Unit, id)
	mh := d.char.AutoAttacks.MHAuto()
	if proc == nil || mh == nil {
		return false, fmt.Errorf("no sim spell %d or no main-hand swing", id)
	}
	hooks, hit := d.target.DynamicDamageTakenModifiers, mh.BonusHitRating
	defer func() { d.target.DynamicDamageTakenModifiers, mh.BonusHitRating = hooks, hit }()
	d.target.DynamicDamageTakenModifiers = append(slices.Clip(hooks), func(_ *core.Simulation, s *core.Spell, _ *core.SpellResult) {
		procs = procs || s == proc
	})
	d.prepare(d.rec.SpellPower.Damage)
	mh.BonusHitRating = hit + forcedHitRating
	for range swings {
		mh.ApplyEffects(d.sim, d.target, mh)
		if procs {
			break
		}
	}
	return procs, nil
}

// compareSelections holds each bare `threat` probe's target selection to whether the sim lands the spell there.
func (m *mobReplay) compareSelections(r *spellModResult, l mobLine) {
	for _, tr := range l.Threat {
		id := tr.Spell.ID
		name := fmt.Sprintf("%d selects the dummy", id)
		selects, ok := mobSelections[id]
		if !ok {
			r.skipped = append(r.skipped, name+": no sim selection for it")
			continue
		}
		sim, err := selects(m, l)
		if err != nil {
			r.add(fail(name, "sim: "+err.Error()), "")
			continue
		}
		detail := fmt.Sprintf("server %v, sim %v", tr.Bonus.Selected.Dummy, sim)
		if sim == tr.Bonus.Selected.Dummy {
			r.add(pass(name, detail), "")
		} else {
			r.add(fail(name, detail), "")
		}
	}
}

// mobDrop is a line, or one check of it, that disagrees with the sim, each with a row in the fixture README. An
// empty check drops the line.
type mobDrop struct {
	bot, state string
	dummy      uint32
	check      string
}

// mobExpected is whether the replay knows the line's bot, state and dummy.
func mobExpected(k mobKey) bool {
	if _, ok := mobDummies[k.dummy]; !ok {
		return false
	}
	return slices.ContainsFunc(mobBots, func(b mobBot) bool { return b.name == k.bot && slices.Contains(b.states, k.state) })
}

// replayMobs compares every line with the sim and holds the run to every bot, state and dummy. A dropped line or
// check only logs its failures, and one that fails nothing fails the replay.
func replayMobs(lines []mobLine, drops []mobDrop) spellModReport {
	report := spellModReport{counts: map[string]int{}}
	used := map[mobDrop]bool{}
	dropped := func(d mobDrop) bool {
		if slices.Contains(drops, d) {
			used[d] = true
			return true
		}
		return false
	}

	var kept []mobLine
	seen := map[mobKey]bool{}
	for _, l := range lines {
		k := l.key()
		switch {
		case !mobExpected(k):
			report.errorf("%s: no bot, state or dummy the replay expects", k)
			continue
		case l.CreatureType != mobDummies[l.Dummy] || l.Info.Target.Entry != l.Dummy ||
			l.Info.Target.CreatureType != l.CreatureType:
			report.errorf("%s: the line says type %d, its info's target is %d of type %d", k, l.CreatureType,
				l.Info.Target.Entry, l.Info.Target.CreatureType)
			continue
		case seen[k]:
			report.errorf("%s twice", k)
			continue
		}
		seen[k] = true
		kept = append(kept, l)
	}
	for _, b := range mobBots {
		for _, state := range b.states {
			for _, entry := range slices.Sorted(maps.Keys(mobDummies)) {
				k := mobKey{b.name, state, entry}
				if !seen[k] && !dropped(mobDrop{k.bot, k.state, k.dummy, ""}) {
					report.errorf("no line for %s", k)
				}
			}
		}
	}

	m := newMobReplay(kept)
	for _, l := range kept {
		k := l.key()
		result := m.compare(l)
		if l.CreatureType == mobReference && result.class != spellModDisagrees {
			result.class = mobsReferenceClass
		}
		report.counts[result.class]++
		text := fmt.Sprintf("MOBSIM %s: %s", k, result.class)
		if len(result.moved) > 0 {
			text += "; " + strings.Join(result.moved, ", ")
		}
		if len(result.skipped) > 0 {
			text += "; not compared: " + strings.Join(result.skipped, ", ")
		}
		report.logf("%s", text)

		lineDropped := dropped(mobDrop{k.bot, k.state, k.dummy, ""})
		for _, c := range result.checks {
			checkDropped := dropped(mobDrop{k.bot, k.state, k.dummy, c.Name})
			switch {
			case checkDropped && c.Passed:
				report.errorf("%s: %s passes, so it shouldn't be dropped: %s", k, c.Name, c.Detail)
			case !c.Passed && (checkDropped || lineDropped):
				report.logf("%s: %s (dropped): %s", k, c.Name, c.Detail)
			case !c.Passed:
				report.errorf("%s: %s: %s", k, c.Name, c.Detail)
			}
		}
		if lineDropped && result.class != spellModDisagrees {
			report.errorf("%s %s, so it shouldn't be dropped", k, result.class)
		}
	}
	for _, d := range drops {
		if !used[d] {
			report.errorf("%+v is dropped, but no line matches it", d)
		}
	}
	report.logf("%d lines agree, %d move nothing, %d disagree, %d are references", report.counts[spellModAgrees],
		report.counts[spellModMovesNothing], report.counts[spellModDisagrees], report.counts[mobsReferenceClass])
	return report
}
