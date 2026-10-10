package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// `.simval threat` records what the server makes of one spell's threat: the threat for `amount` damage
// after spell_threat's percent, the threat spell mods and the school's multiplier, and the bonus
// Spell::HandleThreatSpells adds to each target the spell hits. The replay builds the record's character
// in the sim, in the same stance, form, presence or Righteous Fury, casts the spell at the dummy with a
// forced hit and reads what the sim adds per point of damage and on top of it.

const threatFixture = "threat.jsonl"

// The server works threat out in floats.
const (
	threatTolerance    = 1e-4 // relative
	threatAbsTolerance = 0.01
)

type threatModified struct {
	PctMod           float64 `json:"pctMod"`
	AfterPctMod      float64 `json:"afterPctMod"`
	AfterSpellMods   float64 `json:"afterSpellMods"`
	SchoolMultiplier float64 `json:"schoolMultiplier"`
}

// threatRecord is one `.simval threat` line.
type threatRecord struct {
	Command string `json:"command"`
	Spell   struct {
		ID         int32  `json:"id"`
		Name       string `json:"name"`
		SchoolMask uint32 `json:"schoolMask"`
		SpellLevel int32  `json:"spellLevel"`
		NoThreat   bool   `json:"noThreat"`
	} `json:"spell"`
	Attacker damageUnit `json:"attacker"`
	Target   damageUnit `json:"target"`

	Entry *struct {
		ID       int32   `json:"id"`
		FlatMod  float64 `json:"flatMod"`
		PctMod   float64 `json:"pctMod"`
		ApPctMod float64 `json:"apPctMod"`
	} `json:"entry"`
	Modifiers struct {
		// SPELL_AURA_MOD_THREAT's multiplier per school, physical first
		Schools []float64 `json:"schools"`
	} `json:"modifiers"`
	Damage struct {
		Amount float64 `json:"amount"`
		threatModified
		Threat float64 `json:"threat"`
	} `json:"damage"`
	Bonus struct {
		Selected struct {
			Count  int32 `json:"count"`
			Dummy  bool  `json:"dummy"`
			Caster bool  `json:"caster"`
		} `json:"selected"`
		Skipped      string          `json:"skipped"`
		Source       string          `json:"source"`
		AttackPower  float64         `json:"attackPower"`
		Base         float64         `json:"base"`
		Targets      int32           `json:"targets"`
		PerTarget    float64         `json:"perTarget"`
		Assist       bool            `json:"assist"`
		ReachesDummy bool            `json:"reachesDummy"`
		Modified     *threatModified `json:"modified"`
		Threat       float64         `json:"threat"`
	} `json:"bonus"`
	// SPELL_EFFECT_THREAT, nil in a capture from before the probe recorded it
	ThreatEffect *struct {
		Effects []struct {
			Index int32   `json:"index"`
			Value float64 `json:"value"`
			Dummy bool    `json:"dummy"`
		} `json:"effects"`
		Threat float64 `json:"threat"`
	} `json:"threatEffect"`
	// the threat of `amount` health the spell heals, nil for a spell that doesn't heal
	Heal *struct {
		Amount float64 `json:"amount"`
		Base   float64 `json:"base"`
		threatModified
		Threat float64 `json:"threat"`
	} `json:"heal"`
	// the threat of the power the spell gives its caster, nil in a capture from before the probe recorded it
	Energize *struct {
		Effects []threatEnergizeEffect `json:"effects"`
		// what the cast gives at once, and one tick of its aura that gains all of it
		Cast *threatEnergize `json:"cast"`
		Tick *threatEnergize `json:"tick"`
	} `json:"energize"`
	Live struct {
		Assist             []threatShare `json:"assist"`
		EnergizeAssist     []threatShare `json:"energizeAssist"`
		EnergizeTickAssist []threatShare `json:"energizeTickAssist"`
	} `json:"live"`
}

// threatEnergize is power in the server's units, rage and runic power in tenths, and the threat of it before
// it's shared out: half, unmodified.
type threatEnergize struct {
	Amount float64 `json:"amount"`
	Threat float64 `json:"threat"`
}

type threatEnergizeEffect struct {
	Type        string `json:"type"`
	Caster      bool   `json:"caster"`
	Power       int32  `json:"power"`
	AmplitudeMs int32  `json:"amplitudeMs"`
	Skipped     string `json:"skipped"`
}

// threatShare is one creature's share of assist threat as ThreatManager::ForwardThreatForAssistingMe hands it
// out: an even part to each creature fighting the caster, none to one under crowd control.
type threatShare struct {
	Entry      int32   `json:"entry"`
	Dummy      bool    `json:"dummy"`
	Controlled bool    `json:"controlled"`
	Threat     float64 `json:"threat"`
}

func readThreatRecords(path string) ([]threatRecord, error) {
	return readJSONLines[threatRecord](path)
}

func (r threatRecord) label() string {
	label := fmt.Sprintf("threat %d (%s): %s", r.Spell.ID, r.Spell.Name, r.Attacker.Name)
	if n := r.override(); n > 0 {
		label += fmt.Sprintf(" on %d targets", n)
	}
	if n := r.creatures(); n > 0 {
		label += fmt.Sprintf(" with %d creatures fighting", n)
	}
	return label
}

// fighting are the creatures that were fighting the caster as its assist threat was shared out: the bonus'
// list, else an energize's, which hold the same creatures.
func (r threatRecord) fighting() []threatShare {
	for _, list := range [][]threatShare{r.Live.Assist, r.Live.EnergizeAssist, r.Live.EnergizeTickAssist} {
		if len(list) > 0 {
			return list
		}
	}
	return nil
}

// creatures is how many creatures were fighting the caster when its assist threat was shared out, 0 for the
// dummy alone or a spell with no assist threat.
func (r threatRecord) creatures() int32 {
	if n := int32(len(r.fighting())); n > 1 {
		return n
	}
	return 0
}

// sharing are the creatures a share goes to, those not under crowd control.
func (r threatRecord) sharing() []threatShare {
	return threatSharing(r.fighting())
}

// threatSharing are the shares that count, the ones of creatures not under crowd control: the dummy's first,
// then the others in the record's order.
func threatSharing(list []threatShare) []threatShare {
	var out []threatShare
	for _, s := range list {
		switch {
		case s.Controlled:
		case s.Dummy:
			out = slices.Insert(out, 0, s)
		default:
			out = append(out, s)
		}
	}
	return out
}

// damageRecord is the part of the record the damage replay's character builders read.
func (r threatRecord) damageRecord() damageRecord {
	var d damageRecord
	d.Command = "damage"
	d.Spell.ID, d.Spell.Name = r.Spell.ID, r.Spell.Name
	d.Attacker, d.Target = r.Attacker, r.Target
	return d
}

// energizeTick is the periodic energize the record's tick is of: the first one its caster takes.
func (r threatRecord) energizeTick() (threatEnergizeEffect, bool) {
	if r.Energize == nil || r.Energize.Tick == nil {
		return threatEnergizeEffect{}, false
	}
	for _, e := range r.Energize.Effects {
		if e.Caster && e.Skipped == "" && (e.Type == "periodicEnergize" || e.Type == "obsModPower") {
			return e, true
		}
	}
	return threatEnergizeEffect{}, false
}

// threatForms are the stances, forms, presences and Righteous Fury TestSimvalThreat puts its characters
// in, each with the spec tried first for it. The sim character has to wear the same one.
var threatForms = map[int32]protoreflect.Name{
	71: "protection_warrior", 2457: "warrior", 2458: "warrior",
	25780: "protection_paladin",
	9634:  "feral_tank_druid", 768: "feral_druid",
	48263: "tank_deathknight", 48265: "deathknight", 48266: "deathknight",
	15473: "shadow_priest",
}

// form is the record's stance, form, presence or Righteous Fury, 0 for none.
func (r threatRecord) form() int32 {
	for _, a := range r.Attacker.Auras {
		if _, ok := threatForms[a.ID]; ok {
			return a.ID
		}
	}
	return 0
}

// override is the target count the probe was told to split the bonus over, 0 when it took the cast's own.
func (r threatRecord) override() int32 {
	if r.Bonus.Targets > 0 && r.Bonus.Targets != r.Bonus.Selected.Count {
		return r.Bonus.Targets
	}
	return 0
}

// enemies is how many targets the sim's encounter needs for the cast the server split its bonus over, or for
// the creatures that shared a positive spell's bonus. The caster is one of the server's targets when an effect
// lands on it, which the sim has no target for.
func (r threatRecord) enemies() int {
	n := r.Bonus.Targets
	if n == 0 {
		n = r.Bonus.Selected.Count
	}
	if r.override() == 0 && r.Bonus.Selected.Caster {
		n--
	}
	return max(1, int(n), len(r.sharing()))
}

// attackPower is what the server read for the bonus, else the snapshot's main hand attack power.
func (r threatRecord) attackPower() float64 {
	if r.Bonus.Source == "entry" {
		return r.Bonus.AttackPower
	}
	return r.Attacker.attack("mainhand").AttackPower
}

// threatSimSpells are probes the sim declares under another id: a lower rank, the proc's other id, or
// the cast whose aura the server's spell ticks for. Of that id's spells, the replay reads only those taking
// their threat or a hit from the probe's spell, if any do.
var threatSimSpells = map[int32]int32{
	7386:  47467, // Sunder Armor, the rank the server's warriors know
	58567: 47467, // Sunder Armor's debuff, which 7386 and Devastate cast
	47437: 25203, // Demoralizing Shout
	25742: 20154, // Seal of Righteousness, the damage
	52212: 49938, // Death and Decay, each second's damage
	50622: 46924, // Bladestorm, each tick's main hand Whirlwind
	44949: 46924, // Bladestorm, each tick's off hand Whirlwind
	58433: 58434, // Volley, each tick's arrows
	29131: 2687,  // Bloodrage, the rage over time it triggers
}

// threatSimActions are probes the sim declares as an action with no spell id.
var threatSimActions = map[int32]core.ActionID{
	75: {OtherID: proto.OtherAction_OtherActionShoot}, // Auto Shot
}

// threatTickSpells are server spells that deal one tick of the sim spell's dot each cast, so each tick
// gets their bonus.
var threatTickSpells = map[int32]bool{52212: true, 50622: true, 44949: true, 58433: true}

func threatSimSpell(id int32) int32 {
	if sim, ok := threatSimSpells[id]; ok {
		return sim
	}
	return id
}

// threatTaughtSpells are the spells a talent teaches, which the server probes without it.
var threatTaughtSpells = map[int32]string{
	47498: "devastate", 12809: "concussionBlow", 23881: "bloodthirst", 47486: "mortalStrike",
	48564: "mangle", 48566: "mangle",
	55262: "heartStrike", 55271: "scourgeStrike",
	48660: "hemorrhage",
	44781: "arcaneBarrage", 55360: "livingBomb",
	59172: "chaosBolt", 59092: "darkPact",
	60103: "lavaLash",
	12292: "deathWish", 12975: "lastStand", 48952: "holyShield", 61336: "survivalInstincts", 50334: "berserk",
	13877: "bladeFlurry", 12472: "icyVeins", 12042: "arcanePower", 11129: "combustion", 12043: "presenceOfMind",
	11958: "coldSnap", 16166: "elementalMastery", 30823: "shamanisticRage",
	50622: "bladestorm", 44949: "bladestorm", 48982: "runeTap", 10060: "powerInfusion",
	57669: "vampiricTouch", // the priest's Replenishment
}

// threatExtraSpecs are the specs past the damage replay's that register a probed spell or form.
var threatExtraSpecs = map[uint8][]protoreflect.Name{
	1:  {"protection_warrior"},
	2:  {"protection_paladin"},
	5:  {"smite_priest"},
	6:  {"tank_deathknight"},
	11: {"feral_tank_druid"},
}

// threatLoadouts give the sim character what a probe's spell reads and the damage replay's naked one
// lacks. Any weapon or pet does: the replay compares threat per point of damage and the bonus.
var threatLoadouts = map[int32]func(*proto.Player){
	25742: withThreatWeapons(proto.ItemSlot_ItemSlotMainHand),                                 // Seal of Righteousness: the main hand's speed
	60103: withThreatWeapons(proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand), // Lava Lash: the off hand
	// Bladestorm reads the main hand and the off hand, and its channel asks the rotation whether to go on
	50622: withThreat(withThreatWeapons(proto.ItemSlot_ItemSlotMainHand), withThreatRotation),
	44949: withThreat(withThreatWeapons(proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand), withThreatRotation),
	59092: withThreatPet, // Dark Pact drains the pet's mana
	871:   withThreatItems(map[proto.ItemSlot]int32{proto.ItemSlot_ItemSlotOffHand: threatShieldID}),
	75:    withThreatItems(map[proto.ItemSlot]int32{proto.ItemSlot_ItemSlotRanged: threatBowID}),
	58433: withThreatItems(map[proto.ItemSlot]int32{proto.ItemSlot_ItemSlotRanged: threatBowID}),
	// a rage or energy gain asks the rotation for the next action
	18499: withThreatRotation,
	50213: withThreatRotation,
	2687:  withThreatRotation,
	29131: withThreatRotation,
	5229:  withThreatRotation,
	// each tick of the channel asks it whether to go on
	12051: withThreatRotation,
	44578: withThreatEnchant(proto.ItemSlot_ItemSlotMainHand, threatLifewardID),
	55503: func(player *proto.Player) { player.Professions = []proto.Profession{proto.Profession_Herbalism} },
	// Invigoration is Heartpierce's proc
	71883: withThreatItems(map[proto.ItemSlot]int32{proto.ItemSlot_ItemSlotMainHand: threatHeartpierceID}),
	1719: func(player *proto.Player) {
		if w := player.GetWarrior(); w != nil {
			w.Options.UseRecklessness = true
		}
	},
	// the server's probe casts it on the priest
	10060: func(player *proto.Player) {
		if p := player.GetShadowPriest(); p != nil {
			p.Options.PowerInfusionTarget = &proto.UnitReference{Type: proto.UnitReference_Self}
		}
	},
}

// threatLoadoutSpecs are tried first for a loadout's spell: elemental has no off hand attack to read
// without Feral Spirit.
var threatLoadoutSpecs = map[int32]protoreflect.Name{60103: "enhancement_shaman"}

// plain items with no effects: a one-hand mace, a shield and a bow
const (
	threatWeaponID = 6360
	threatShieldID = 30314
	threatBowID    = 30105

	threatHeartpierceID = 49982
	threatLifewardID    = 3241 // the weapon enchant
)

func withThreatWeapons(slots ...proto.ItemSlot) func(*proto.Player) {
	items := map[proto.ItemSlot]int32{}
	for _, slot := range slots {
		items[slot] = threatWeaponID
	}
	return withThreatItems(items)
}

func withThreatItems(bySlot map[proto.ItemSlot]int32) func(*proto.Player) {
	return func(player *proto.Player) {
		items := make([]*proto.ItemSpec, len(proto.ItemSlot_name))
		for i := range items {
			items[i] = &proto.ItemSpec{}
		}
		for slot, id := range bySlot {
			items[slot].Id = id
		}
		player.Equipment = &proto.EquipmentSpec{Items: items}
	}
}

// withThreatEnchant is the plain weapon in that slot with the enchant on it.
func withThreatEnchant(slot proto.ItemSlot, enchant int32) func(*proto.Player) {
	return func(player *proto.Player) {
		withThreatWeapons(slot)(player)
		player.Equipment.Items[slot].Enchant = enchant
	}
}

func withThreatPet(player *proto.Player) {
	if w := player.GetWarlock(); w != nil && w.Options != nil {
		w.Options.Summon = proto.Warlock_Options_Imp
	}
}

func withThreatRotation(player *proto.Player) {
	player.Rotation = &proto.APLRotation{}
}

func withThreat(loads ...func(*proto.Player)) func(*proto.Player) {
	return func(player *proto.Player) {
		for _, load := range loads {
			load(player)
		}
	}
}

// threatSim is the record's character with the sim spells the probe casts, every tag of the id, the lowest
// first, and which of them it reads. With no spells it's a heal the sim gives through Unit.HealThreat, or an
// aura cast it books with a CastThreat.
type threatSim struct {
	*damageSim
	spells      []*core.Spell
	measured    []bool
	comboPoints *core.ResourceMetrics
	// takes the mana spent to make room for a tick
	manaSpent *core.ResourceMetrics
}

// threatSpellsByID are the unit's spells of that id, every tag, lowest first.
func threatSpellsByID(unit *core.Unit, id int32) []*core.Spell {
	var found []*core.Spell
	for _, s := range unit.Spellbook {
		if s.ActionID.SpellID == id && s.ActionID.ItemID == 0 && s.ActionID.OtherID == 0 {
			found = append(found, s)
		}
	}
	slices.SortStableFunc(found, func(a, b *core.Spell) int { return int(a.ActionID.Tag - b.ActionID.Tag) })
	return found
}

// threatSpellsFor are the unit's spells a probe casts.
func threatSpellsFor(unit *core.Unit, probe int32) []*core.Spell {
	action, ok := threatSimActions[probe]
	if !ok {
		return threatSpellsByID(unit, threatSimSpell(probe))
	}
	var found []*core.Spell
	for _, s := range unit.Spellbook {
		if s.ActionID.SpellID == action.SpellID && s.ActionID.OtherID == action.OtherID && s.ActionID.ItemID == 0 {
			found = append(found, s)
		}
	}
	return found
}

// threatMeasured marks the spells the replay reads: all of them, unless the sim declares the probe under
// another id and some take their threat or a hit from the probe's spell (Sunder Armor's 7386 apart from the
// 58567 it casts, Bladestorm's two Whirlwinds).
func threatMeasured(spells []*core.Spell, probe int32, target *core.Unit) []bool {
	measured := make([]bool, len(spells))
	reads := false
	if _, mapped := threatSimSpells[probe]; mapped {
		for i, s := range spells {
			measured[i] = threatReads(s, probe, target)
			reads = reads || measured[i]
		}
	}
	if !reads {
		for i := range measured {
			measured[i] = true
		}
	}
	return measured
}

// threatReads is whether the spell takes its threat or a hit from the server spell.
func threatReads(spell *core.Spell, id int32, target *core.Unit) bool {
	if spell.ThreatServerSpellID() == id || spell.Direct.FromSpellID == id {
		return true
	}
	for _, dot := range threatDots(spell, target) {
		if dot.Tick.FromSpellID == id {
			return true
		}
	}
	return false
}

// threatDots are the spell's dot on the target and its area dot, those it has.
func threatDots(spell *core.Spell, target *core.Unit) []*core.Dot {
	var dots []*core.Dot
	for _, dot := range []*core.Dot{dotOn(spell, target), spell.AOEDot()} {
		if dot != nil {
			dots = append(dots, dot)
		}
	}
	return dots
}

func threatDotCount(spell *core.Spell, target *core.Unit) int {
	return len(threatDots(spell, target))
}

// buildThreatSim tries the specs, its form's first, until one registers the spell and wears the form,
// then again with the talent that teaches the spell.
func buildThreatSim(rec threatRecord) (*threatSim, error) {
	dr := rec.damageRecord()
	setup, err := damageSetupFor(dr)
	if err != nil {
		return nil, err
	}
	setups := []damageSetup{setup}
	if field, ok := threatTaughtSpells[rec.Spell.ID]; ok {
		if taught, ok := setup.withTalent(rec.Attacker.Class, field); ok {
			setups = append(setups, taught)
		}
	}

	class := classes[rec.Attacker.Class]
	form := rec.form()
	var specs []protoreflect.Name
	for _, spec := range slices.Concat([]protoreflect.Name{threatForms[form], threatLoadoutSpecs[rec.Spell.ID], class.spec}, class.otherSpecs,
		threatExtraSpecs[rec.Attacker.Class]) {
		if spec != "" && !slices.Contains(specs, spec) {
			specs = append(specs, spec)
		}
	}

	simID := threatSimSpell(rec.Spell.ID)
	for _, setup := range setups {
		for _, spec := range specs {
			player, err := damagePlayer(dr, setup, spec)
			if err != nil {
				return nil, err
			}
			if load := threatLoadouts[rec.Spell.ID]; load != nil {
				load(player)
			}
			d, err := newDamageSim(dr, player, setup.others, rec.enemies())
			if err != nil {
				return nil, err
			}
			if form != 0 && slices.Contains(d.others, form) {
				continue
			}
			spells := threatSpellsFor(&d.char.Unit, rec.Spell.ID)
			if len(spells) == 0 {
				if _, _, heals := d.char.HealThreatOf(rec.Spell.ID, 0); heals {
					return &threatSim{damageSim: d}, nil
				}
				if _, casts := d.char.CastThreatOf(rec.Spell.ID); casts {
					return &threatSim{damageSim: d}, nil
				}
				continue
			}
			if threatTickSpells[rec.Spell.ID] {
				// cast the spell whose dot ticks, not one its ticks deal (Bladestorm's off hand)
				slices.SortStableFunc(spells, func(a, b *core.Spell) int {
					return threatDotCount(b, d.target) - threatDotCount(a, d.target)
				})
			}
			ts := &threatSim{damageSim: d, spells: spells, measured: threatMeasured(spells, rec.Spell.ID, d.target)}
			if d.char.HasEnergyBar() {
				ts.comboPoints = d.char.NewComboPointMetrics(core.ActionID{SpellID: simID})
			}
			if d.char.HasManaBar() {
				ts.manaSpent = d.char.NewManaMetrics(core.ActionID{SpellID: simID})
			}
			return ts, nil
		}
	}
	if form != 0 {
		return nil, fmt.Errorf("no %v spec registers spell %d and aura %d", specs, simID, form)
	}
	return nil, fmt.Errorf("no %v spec registers spell %d", specs, simID)
}

// threatCast is the sim's threat on the dummy from one cast that hits, beyond what its damage brings, and
// from one tick of the dot it leaves there, over every tag of the spell id.
type threatCast struct {
	// threat per point of damage, of the tag that dealt the most
	slope, spellMultiplier, schoolMultiplier float64
	bonus, damage                            float64
	// SPELL_EFFECT_THREAT the cast added on the dummy, which `.simval threat` doesn't record, so it's out of bonus
	effect                float64
	ticked                bool
	tickBonus, tickDamage float64
	// the cast's bonus on each target of the encounter, the dummy's first
	shares []float64
	// the unit's threat multiplier per server school before the cast
	schools []float64
	// the threat of the power the cast gave at once, summed over the enemies
	energize float64
	// one period on: the threat of its energize ticks, and the power they gave
	tickEnergize, tickGain float64
	note                   string
	failed                 error
}

type threatMetrics struct {
	threat, damage    float64
	outcomes, avoided int32
}

func (ts *threatSim) metrics(target *core.Unit) []threatMetrics {
	out := make([]threatMetrics, len(ts.spells))
	for i, s := range ts.spells {
		m := s.SpellMetrics[target.UnitIndex]
		out[i] = threatMetrics{
			threat:   m.TotalThreat,
			damage:   m.TotalDamage,
			outcomes: m.Hits + m.Crits + m.Crushes + m.Glances + m.Blocks + m.Misses + m.Dodges + m.Parries,
			avoided:  m.Misses + m.Dodges + m.Parries,
		}
	}
	return out
}

// cast casts the spell at the dummy with a hit forced and no missile travel time, and runs what that
// leaves queued for now: the damage, and the spells it casts in turn. Then one tick of its dot. A tag
// that lands nothing on the dummy (Rune Strike's queue) gives way to the next one.
func (ts *threatSim) cast(rec threatRecord) threatCast {
	type saved struct {
		flags core.SpellFlag
		speed float64
	}
	restore := make([]saved, len(ts.spells))
	for i, s := range ts.spells {
		restore[i] = saved{s.Flags, s.MissileSpeed}
		s.Flags |= core.SpellFlagIgnoreResists | core.SpellFlagAlwaysHit
		s.MissileSpeed = 0
	}
	defer func() {
		for i, s := range ts.spells {
			s.Flags, s.MissileSpeed = restore[i].flags, restore[i].speed
		}
	}()

	var first threatCast
	for i, spell := range ts.spells {
		c, landed := ts.castUntilHit(rec, spell)
		if i == 0 {
			first = c
		}
		if c.failed != nil || landed {
			return c
		}
	}
	return first
}

func (ts *threatSim) castUntilHit(rec threatRecord, spell *core.Spell) (c threatCast, landed bool) {
	for range maxHitAttempts {
		var avoided bool
		c, landed, avoided = ts.castOnce(rec, spell)
		if c.failed != nil || !avoided {
			return c, landed
		}
	}
	// Holy Wrath on a dummy that's neither undead nor a demon
	c.note = fmt.Sprintf("the sim's cast missed the dummy all %d tries", maxHitAttempts)
	return c, landed
}

func (ts *threatSim) castOnce(rec threatRecord, spell *core.Spell) (c threatCast, landed, avoided bool) {
	defer func() {
		if r := recover(); r != nil {
			c.failed = fmt.Errorf("%v", r)
		}
	}()
	ts.start()
	setStat(ts.sim, &ts.char.Unit, stats.AttackPower, rec.attackPower())
	// the finishers' damage reads combo points, never their threat
	if ts.comboPoints != nil {
		ts.char.AddComboPoints(ts.sim, 5, ts.comboPoints)
	}
	slopes, multipliers := make([]float64, len(ts.spells)), make([]float64, len(ts.spells))
	for i, s := range ts.spells {
		s.BonusHitRating += forcedHitRating
		s.BonusCritRating -= forcedCritRating
		slopes[i] = s.ThreatFromDamage(core.OutcomeHit, 1) - s.ThreatFromDamage(core.OutcomeHit, 0)
		multipliers[i] = s.ThreatMultiplier
	}
	pick := slices.Index(ts.spells, spell)
	if !ts.measured[pick] {
		pick = slices.Index(ts.measured, true)
	}
	c.slope, c.spellMultiplier = slopes[pick], multipliers[pick]
	c.schoolMultiplier = ts.char.ThreatSchoolMultiplier(ts.spells[pick].SpellSchool)
	c.schools = ts.schoolMultipliers()
	// an aura the cast puts on, booked with a CastThreat, goes out evenly
	auraCast, _ := ts.char.CastThreatOf(rec.Spell.ID)
	auraCast /= float64(len(ts.sim.Encounter.TargetUnits))

	// a cast, so it hands out its threat bonus
	spell.SkipCastAndApplyEffects(ts.sim, ts.target)
	ts.settle()
	c.energize, _, _ = ts.energizeThreat()
	after := ts.metrics(ts.target)
	dealt := make([]float64, len(after))
	for i, m := range after {
		if !ts.measured[i] {
			continue
		}
		landed = landed || m.outcomes > 0 || m.threat != 0 || m.damage != 0
		avoided = avoided || m.avoided > 0
		c.effect += ts.spells[i].ThreatEffect() * float64(m.outcomes-m.avoided)
		c.bonus += m.threat - slopes[i]*m.damage
		c.damage += m.damage
		dealt[i] = m.damage
	}
	c.bonus += auraCast - c.effect
	if avoided {
		return c, landed, true
	}
	for _, target := range ts.sim.Encounter.TargetUnits {
		share := auraCast
		for i, m := range ts.metrics(target) {
			if ts.measured[i] {
				share += m.threat - slopes[i]*m.damage
			}
		}
		c.shares = append(c.shares, share)
	}

	dot := dotOn(spell, ts.target)
	if dot == nil || !dot.IsActive() {
		dot = spell.AOEDot()
	}
	if dot != nil && dot.IsActive() {
		dot.OnTick(ts.sim, dot.Unit, dot)
		ts.settle()
		c.ticked = true
		for i, m := range ts.metrics(ts.target) {
			if !ts.measured[i] {
				continue
			}
			threat, damage := m.threat-after[i].threat, m.damage-after[i].damage
			c.tickBonus += threat - slopes[i]*damage
			c.tickDamage += damage
			dealt[i] += damage
		}
	}

	for i, d := range dealt {
		if d > dealt[pick] {
			pick = i
		}
	}
	c.slope, c.spellMultiplier = slopes[pick], multipliers[pick]
	c.schoolMultiplier = ts.char.ThreatSchoolMultiplier(ts.spells[pick].SpellSchool)
	// a strike reading a weapon the character doesn't carry
	if math.IsNaN(c.bonus + c.damage + c.tickBonus + c.tickDamage) {
		c.failed = fmt.Errorf("its damage is NaN, bonus %v damage %v", c.bonus, c.damage)
	}

	if tick, ok := rec.energizeTick(); ok && tick.AmplitudeMs > 0 {
		_, threat, gain := ts.energizeThreat()
		// room for a mana tick to land whole
		if tick.Power == serverPowerMana && ts.manaSpent != nil {
			ts.char.SpendMana(ts.sim, ts.char.CurrentMana(), ts.manaSpent)
		}
		// a tick waits for the next server update after its time, so half a period more
		ts.advance(time.Duration(tick.AmplitudeMs) * time.Millisecond * 3 / 2)
		_, threatAfter, gainAfter := ts.energizeThreat()
		c.tickEnergize, c.tickGain = threatAfter-threat, gainAfter-gain
	}
	return c, landed, false
}

// The server's Powers the replay tells apart.
const (
	serverPowerMana  = 0
	serverPowerRage  = 1
	serverPowerRunic = 6
)

// schoolMultipliers is the unit's threat multiplier for each server school, physical first.
func (ts *threatSim) schoolMultipliers() []float64 {
	out := make([]float64, 7)
	for i := range out {
		out[i] = ts.char.ThreatSchoolMultiplier(core.SpellSchoolFromServerMask(1 << i))
	}
	return out
}

// energizeThreat is what the energizes under the measured spells' actions made so far: the cast's own, then
// their ticks' and the power those gave.
func (ts *threatSim) energizeThreat() (bySpell, byTick, tickGain float64) {
	seen := map[core.ActionID]bool{}
	for i, s := range ts.spells {
		if !ts.measured[i] || seen[s.ActionID] {
			continue
		}
		seen[s.ActionID] = true
		spell, tick, gain := ts.char.EnergizeThreatOf(s.ActionID)
		bySpell, byTick, tickGain = bySpell+spell, byTick+tick, tickGain+gain
	}
	return bySpell, byTick, tickGain
}

// settle runs the actions queued for now and stops before the first one later, so a dot's own ticks
// stay out of it.
func (ts *threatSim) settle() {
	ts.advance(time.Nanosecond)
}

// advance runs the sim on by d, stopping before anything later.
func (ts *threatSim) advance(d time.Duration) {
	done := false
	ts.sim.AddPendingAction(&core.PendingAction{
		NextActionAt: ts.sim.CurrentTime + d,
		Priority:     core.ActionPriorityLow,
		OnAction:     func(*core.Simulation) { done = true },
	})
	for !done && !ts.sim.Step() {
	}
}

// Server spell effects and auras that deal damage.
var (
	threatDamageEffects = []int32{2, 9, 17, 31, 58, 121}
	threatDamageAuras   = []int32{3, 53, 89}
)

// threatServerDamage is whether one of the server spell's effects deals damage.
func threatServerDamage(id int32) bool {
	spell := serverdata.SpellByID(id)
	if spell == nil {
		return false
	}
	for _, e := range spell.Effects {
		if slices.Contains(threatDamageEffects, e.Effect) || e.Effect == 6 && slices.Contains(threatDamageAuras, e.Aura) {
			return true
		}
	}
	return false
}

func checkThreatNear(name string, sim, server float64, detail string) Check {
	text := fmt.Sprintf("sim %.2f, server %.2f", sim, server)
	if detail != "" {
		text += "; " + detail
	}
	if math.Abs(sim-server) <= max(threatAbsTolerance, threatTolerance*math.Abs(server)) {
		return pass(name, text)
	}
	return fail(name, text)
}

// checkThreat compares a record with the sim.
func checkThreat(rec threatRecord) []Check {
	return replayThreat(rec, buildThreatSim)
}

func replayThreat(rec threatRecord, build func(threatRecord) (*threatSim, error)) []Check {
	checks := replayThreatChecks(rec, build)
	for i, c := range checks {
		if reason, ok := knownThreatMismatches[threatMismatchOf(rec, c.Name)]; ok && !c.Passed {
			checks[i] = pass(c.Name+knownMismatchSuffix, reason+"; "+c.Detail)
		}
	}
	return checks
}

func replayThreatChecks(rec threatRecord, build func(threatRecord) (*threatSim, error)) []Check {
	if !rec.Attacker.IsPlayer || rec.Attacker.Level != core.CharacterLevel {
		return []Check{fail("attacker", fmt.Sprintf("level %d player=%v; the replay builds a level %d player",
			rec.Attacker.Level, rec.Attacker.IsPlayer, core.CharacterLevel))}
	}
	ts, err := build(rec)
	if err != nil {
		return []Check{fail("sim character", err.Error())}
	}
	if len(ts.spells) == 0 {
		return ts.gainChecks(rec)
	}
	c := ts.cast(rec)
	if c.failed != nil {
		return []Check{fail("sim cast", fmt.Sprintf("spell %v: %v", ts.spells[0].ActionID, c.failed))}
	}
	checks := append(ts.castChecks(rec, c), checkThreatSchools(rec, c.schools)...)
	return append(checks, ts.energizeChecks(rec, c)...)
}

// castChecks compares what the cast made on the dummy: its damage's threat, its bonus, its effect's threat, a
// tick's bonus and its heal's threat.
func (ts *threatSim) castChecks(rec threatRecord, c threatCast) []Check {
	var checks []Check
	d := rec.Damage
	if c.damage > 0 || c.tickDamage > 0 || threatServerDamage(rec.Spell.ID) {
		checks = append(checks, checkThreatNear("damage threat", c.slope*d.Amount, d.Threat, fmt.Sprintf(
			"sim x%.4f (spell %.4f, school %.4f), server x%.4f (spell_threat %.4f, after spell mods %.2f, school %.4f)",
			c.slope, c.spellMultiplier, c.schoolMultiplier, d.Threat/d.Amount, d.PctMod,
			d.AfterSpellMods, d.SchoolMultiplier)))
	} else {
		checks = append(checks, notCompared("damage threat", "neither the sim's cast nor the server spell deals damage"))
	}

	if threatTickSpells[rec.Spell.ID] {
		if !c.ticked {
			return append(checks, fail("tick bonus", "the sim spell leaves no dot on the dummy"))
		}
		return append(checks, checkThreatNear("tick bonus", c.tickBonus, rec.Bonus.Threat, rec.serverBonus()))
	}
	note := c.note
	if c.effect != 0 && rec.ThreatEffect == nil {
		note = threatJoinDetail(fmt.Sprintf("the sim's %.2f SPELL_EFFECT_THREAT left out, the record has none", c.effect), note)
	}
	checks = append(checks, checkThreatNear("bonus", c.bonus, rec.dummyBonus(), threatJoinDetail(rec.serverBonus(), note)))
	if len(rec.Live.Assist) > 1 {
		checks = append(checks, checkThreatShares("assist shares", rec.Live.Assist, c.shares))
	}
	if e := rec.ThreatEffect; e != nil && (len(e.Effects) > 0 || c.effect != 0) {
		checks = append(checks, checkThreatNear("effect threat", c.effect, e.Threat, rec.serverEffect()))
	}
	if c.ticked {
		// an aura's periodic damage never runs HandleThreatSpells
		checks = append(checks, checkThreatNear("tick bonus", c.tickBonus, 0, "server ticks add no bonus"))
	}
	if rec.Heal != nil {
		checks = append(checks, ts.checkHealThreat(rec))
	}
	return checks
}

// dummyBonus is the server's bonus on the dummy: its share when more creatures shared a positive spell's.
func (r threatRecord) dummyBonus() float64 {
	return threatDummyShare(r.Live.Assist, r.Bonus.Threat)
}

// threatDummyShare is the dummy's share of total when more creatures shared it out, else all of it.
func threatDummyShare(list []threatShare, total float64) float64 {
	if shares := threatSharing(list); len(list) > 1 && len(shares) > 0 && shares[0].Dummy {
		return shares[0].Threat
	}
	return total
}

// gainChecks compares a probe the sim has no spell for and books with a gain instead: a heal through
// Unit.HealThreat, whose bonus comes with each gain when the gain is a cast, or an aura cast's CastThreat.
func (ts *threatSim) gainChecks(rec threatRecord) []Check {
	ts.start()
	setStat(ts.sim, &ts.char.Unit, stats.AttackPower, rec.attackPower())
	schools := ts.schoolMultipliers()
	_, bonus, _ := ts.char.HealThreatOf(rec.Spell.ID, 0)
	if cast, ok := ts.char.CastThreatOf(rec.Spell.ID); ok {
		bonus += cast
	}

	var checks []Check
	if threatServerDamage(rec.Spell.ID) {
		checks = append(checks, fail("damage threat", "the server spell deals damage, the sim has no spell for it"))
	} else {
		checks = append(checks, notCompared("damage threat", "neither the sim nor the server spell deals damage with it"))
	}
	// the sim shares a gain's threat evenly over the enemies
	enemies := len(ts.sim.Encounter.TargetUnits)
	shares := threatEvenShares(bonus, enemies)
	checks = append(checks, checkThreatNear("bonus", shares[0], rec.dummyBonus(), rec.serverBonus()))
	if len(rec.Live.Assist) > 1 {
		checks = append(checks, checkThreatShares("assist shares", rec.Live.Assist, shares))
	}
	if rec.Heal != nil {
		checks = append(checks, ts.checkHealThreat(rec))
	}
	checks = append(checks, checkThreatSchools(rec, schools)...)
	const noCast = "the sim has no spell to cast for it"
	if e := rec.Energize; e != nil && e.Cast != nil {
		checks = append(checks, notCompared("energize threat", noCast))
	}
	if _, ok := rec.energizeTick(); ok {
		checks = append(checks, notCompared("energize tick threat", noCast))
	}
	return checks
}

func threatEvenShares(total float64, enemies int) []float64 {
	shares := make([]float64, enemies)
	for i := range shares {
		shares[i] = total / float64(enemies)
	}
	return shares
}

// checkThreatSchools holds the unit's threat multiplier of every school to the server's product of
// SPELL_AURA_MOD_THREAT auras, the schools no probed spell has too. A record without them gets no check.
func checkThreatSchools(rec threatRecord, sim []float64) []Check {
	server := rec.Modifiers.Schools
	if len(server) != len(sim) {
		return nil
	}
	list := func(values []float64) string {
		parts := make([]string, len(values))
		for i, v := range values {
			parts[i] = fmt.Sprintf("%.4f", v)
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	detail := fmt.Sprintf("physical first: sim %s, server %s", list(sim), list(server))
	for i := range sim {
		if math.Abs(sim[i]-server[i]) > threatTolerance*max(1, math.Abs(server[i])) {
			return []Check{fail("school multipliers", detail)}
		}
	}
	return []Check{pass("school multipliers", detail)}
}

// energizeChecks compares the threat of the power the cast gave its caster at once on the dummy, and of one
// tick of the aura it left.
func (ts *threatSim) energizeChecks(rec threatRecord, c threatCast) []Check {
	e := rec.Energize
	if e == nil {
		return nil
	}
	var checks []Check
	if e.Cast != nil {
		shares := threatEvenShares(c.energize, len(ts.sim.Encounter.TargetUnits))
		detail := fmt.Sprintf("server %.0f power, unmodified", e.Cast.Amount)
		if n := len(rec.Live.EnergizeAssist); n > 1 {
			detail += fmt.Sprintf(", shared by %d of %d creatures", len(threatSharing(rec.Live.EnergizeAssist)), n)
		}
		checks = append(checks, checkThreatNear("energize threat", shares[0],
			threatDummyShare(rec.Live.EnergizeAssist, e.Cast.Threat), detail))
		if len(rec.Live.EnergizeAssist) > 1 {
			checks = append(checks, checkThreatShares("energize shares", rec.Live.EnergizeAssist, shares))
		}
	}
	if tick, ok := rec.energizeTick(); ok {
		checks = append(checks, checkThreatEnergizeTick(*e.Tick, tick, c))
	}
	return checks
}

// checkThreatEnergizeTick holds the threat the sim makes of a tick's power to the server's, summed over the
// enemies and for the server's amount: the size of the sim's tick isn't threat's to check.
func checkThreatEnergizeTick(server threatEnergize, effect threatEnergizeEffect, c threatCast) Check {
	const name = "energize tick threat"
	// the sim counts rage and runic power in points, the server in tenths
	gain := c.tickGain
	if effect.Power == serverPowerRage || effect.Power == serverPowerRunic {
		gain *= 10
	}
	detail := fmt.Sprintf("server %.0f power every %d ms, unmodified", server.Amount, effect.AmplitudeMs)
	if gain <= 0 {
		return fail(name, fmt.Sprintf("the sim's spell gave no power in %d ms; %s", effect.AmplitudeMs, detail))
	}
	return checkThreatNear(name, c.tickEnergize/gain*server.Amount, server.Threat,
		fmt.Sprintf("the sim's %.2f for %.1f power; %s", c.tickEnergize, gain, detail))
}

// checkHealThreat holds the sim's threat for the record's amount of healing to the server's: a heal through
// Unit.HealThreat of the probe's spell, else a heal of the measured spell that heals.
func (ts *threatSim) checkHealThreat(rec threatRecord) Check {
	h := rec.Heal
	server := fmt.Sprintf("server %.0f healed x%.2f (spell_threat %.4f, after spell mods %.2f, school %.4f)",
		h.Amount, h.Base/h.Amount, h.PctMod, h.AfterSpellMods, h.SchoolMultiplier)
	// a heal-only probe starts here; after a cast the run is still up (a channel can't be ended without its rotation)
	if !ts.ran {
		ts.start()
		setStat(ts.sim, &ts.char.Unit, stats.AttackPower, rec.attackPower())
	}
	if heal, _, ok := ts.char.HealThreatOf(rec.Spell.ID, h.Amount); ok {
		return checkThreatNear("heal threat", heal, h.Threat, server)
	}
	for i, s := range ts.spells {
		if ts.measured[i] && s.ProcMask.Matches(core.ProcMaskSpellHealing) {
			return checkThreatNear("heal threat", s.ThreatFromHealing(h.Amount), h.Threat, server)
		}
	}
	return notCompared("heal threat", "the sim doesn't heal with it; "+server)
}

// serverEffect says what the server's SPELL_EFFECT_THREAT is made of.
func (r threatRecord) serverEffect() string {
	var parts []string
	for _, e := range r.ThreatEffect.Effects {
		part := fmt.Sprintf("effect %d %.0f", e.Index, e.Value)
		if !e.Dummy {
			part += " off the dummy"
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "server has no SPELL_EFFECT_THREAT"
	}
	return "server " + strings.Join(parts, ", ")
}

// checkThreatShares holds the cast's assist threat on the sim's other targets to the shares of the server's
// other creatures in list. The sim has a target for each creature that shared, and all of them fight the caster.
func checkThreatShares(name string, list []threatShare, sim []float64) Check {
	var server []float64
	for _, s := range threatSharing(list) {
		if !s.Dummy {
			server = append(server, s.Threat)
		}
	}
	if len(sim) > 0 {
		sim = sim[1:]
	}
	detail := fmt.Sprintf("sim %s, server %s", threatList(sim), threatList(server))
	if len(sim) != len(server) {
		return fail(name, detail)
	}
	for i := range sim {
		if math.Abs(sim[i]-server[i]) > max(threatAbsTolerance, threatTolerance*math.Abs(server[i])) {
			return fail(name, detail)
		}
	}
	return pass(name, detail)
}

func threatList(values []float64) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%.2f", v)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func threatJoinDetail(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

// serverBonus says where the server's bonus came from.
func (r threatRecord) serverBonus() string {
	b := r.Bonus
	switch {
	case b.Skipped != "":
		return "server skipped it: " + b.Skipped
	case !b.ReachesDummy:
		return fmt.Sprintf("server %s %.2f misses the dummy", b.Source, b.Base)
	}
	text := fmt.Sprintf("server %s %.2f over %d targets", b.Source, b.Base, b.Targets)
	if b.Modified != nil {
		text += fmt.Sprintf(", modified x%.4f", b.Modified.SchoolMultiplier)
	} else {
		text += ", unmodified"
	}
	if n := len(r.Live.Assist); n > 1 {
		text += fmt.Sprintf(", shared by %d of %d creatures", len(threatSharing(r.Live.Assist)), n)
	}
	return text
}

// threatMismatch names a check of one spell for one server class in one form, split over a target count
// the probe was given (0 for the cast's own), with that many creatures fighting (0 for the dummy alone).
type threatMismatch struct {
	class     uint8
	form      int32
	spell     int32
	override  int32
	creatures int32
	check     string
}

func threatMismatchOf(rec threatRecord, check string) threatMismatch {
	return threatMismatch{rec.Attacker.Class, rec.form(), rec.Spell.ID, rec.override(), rec.creatures(), check}
}

// knownThreatMismatches are checks where the sim and the server really differ, each with its reason and a
// row in the fixture README. The check still runs; only its failure is let through. An entry no record
// needs fails TestThreatRecords.
var knownThreatMismatches = map[threatMismatch]string{}
