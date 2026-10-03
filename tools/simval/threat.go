package main

import (
	"fmt"
	"math"
	"slices"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim/core"
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
}

func readThreatRecords(path string) ([]threatRecord, error) {
	return readJSONLines[threatRecord](path)
}

func (r threatRecord) label() string {
	label := fmt.Sprintf("threat %d (%s): %s", r.Spell.ID, r.Spell.Name, r.Attacker.Name)
	if n := r.override(); n > 0 {
		label += fmt.Sprintf(" on %d targets", n)
	}
	return label
}

// damageRecord is the part of the record the damage replay's character builders read.
func (r threatRecord) damageRecord() damageRecord {
	var d damageRecord
	d.Command = "damage"
	d.Spell.ID, d.Spell.Name = r.Spell.ID, r.Spell.Name
	d.Attacker, d.Target = r.Attacker, r.Target
	return d
}

// threatForms are the stances, forms, presences and Righteous Fury TestSimvalThreat puts its characters
// in, each with the spec tried first for it. The sim character has to wear the same one.
var threatForms = map[int32]protoreflect.Name{
	71: "protection_warrior", 2457: "warrior", 2458: "warrior",
	25780: "protection_paladin",
	9634:  "feral_tank_druid", 768: "feral_druid",
	48263: "tank_deathknight", 48265: "deathknight", 48266: "deathknight",
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

// enemies is how many targets the sim's encounter needs for the cast the server split its bonus over.
// The caster is one of the server's when an effect lands on it, which the sim has no target for.
func (r threatRecord) enemies() int {
	n := r.Bonus.Targets
	if n == 0 {
		n = r.Bonus.Selected.Count
	}
	if r.override() == 0 && r.Bonus.Selected.Caster {
		n--
	}
	return max(1, int(n))
}

// attackPower is what the server read for the bonus, else the snapshot's main hand attack power.
func (r threatRecord) attackPower() float64 {
	if r.Bonus.Source == "entry" {
		return r.Bonus.AttackPower
	}
	return r.Attacker.attack("mainhand").AttackPower
}

// threatSimSpells are probes the sim declares under another id: a lower rank, the proc's other id, or
// the cast whose aura the server's spell ticks for.
var threatSimSpells = map[int32]int32{
	47437: 25203, // Demoralizing Shout
	25742: 20154, // Seal of Righteousness, the damage
	52212: 49938, // Death and Decay, each second's damage
}

// threatTickSpells are server spells that deal one tick of the sim spell's dot each cast, so each tick
// gets their bonus.
var threatTickSpells = map[int32]bool{52212: true}

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
}

// threatExtraSpecs are the specs past the damage replay's that register a probed spell or form.
var threatExtraSpecs = map[uint8][]protoreflect.Name{
	1:  {"protection_warrior"},
	2:  {"protection_paladin"},
	5:  {"smite_priest"},
	6:  {"tank_deathknight"},
	11: {"feral_tank_druid"},
}

// threatSim is the record's character with the sim spells the probe compares: every tag of the id, the
// lowest first.
type threatSim struct {
	*damageSim
	spells      []*core.Spell
	comboPoints *core.ResourceMetrics
}

// threatSpellsByID are the unit's spells of that id, every tag, lowest first.
func threatSpellsByID(unit *core.Unit, id int32) []*core.Spell {
	var found []*core.Spell
	for _, s := range unit.Spellbook {
		if s.ActionID.SpellID == id && s.ActionID.ItemID == 0 && s.ActionID.OtherID == 0 {
			found = append(found, s)
		}
	}
	slices.SortFunc(found, func(a, b *core.Spell) int { return int(a.ActionID.Tag - b.ActionID.Tag) })
	return found
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
	for _, spec := range slices.Concat([]protoreflect.Name{threatForms[form], class.spec}, class.otherSpecs,
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
			d, err := newDamageSim(dr, player, setup.others, rec.enemies())
			if err != nil {
				return nil, err
			}
			spells := threatSpellsByID(&d.char.Unit, simID)
			if len(spells) == 0 || form != 0 && slices.Contains(d.others, form) {
				continue
			}
			ts := &threatSim{damageSim: d, spells: spells}
			if d.char.HasEnergyBar() {
				ts.comboPoints = d.char.NewComboPointMetrics(core.ActionID{SpellID: simID})
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
	slope, spellMultiplier float64
	bonus, damage          float64
	ticked                 bool
	tickBonus, tickDamage  float64
	note                   string
	failed                 error
}

type threatMetrics struct {
	threat, damage    float64
	outcomes, avoided int32
}

func (ts *threatSim) metrics() []threatMetrics {
	out := make([]threatMetrics, len(ts.spells))
	for i, s := range ts.spells {
		m := s.SpellMetrics[ts.target.UnitIndex]
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
	c.slope, c.spellMultiplier = slopes[pick], multipliers[pick]

	spell.ApplyEffects(ts.sim, ts.target, spell)
	ts.settle()
	after := ts.metrics()
	var dealt []float64
	for i, m := range after {
		landed = landed || m.outcomes > 0 || m.threat != 0 || m.damage != 0
		avoided = avoided || m.avoided > 0
		c.bonus += m.threat - slopes[i]*m.damage
		c.damage += m.damage
		dealt = append(dealt, m.damage)
	}
	if avoided {
		return c, landed, true
	}

	dot := dotOn(spell, ts.target)
	if dot == nil || !dot.IsActive() {
		dot = spell.AOEDot()
	}
	if dot != nil && dot.IsActive() {
		dot.OnTick(ts.sim, dot.Unit, dot)
		ts.settle()
		c.ticked = true
		for i, m := range ts.metrics() {
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
	// Lava Lash without an off hand weapon
	if math.IsNaN(c.bonus + c.damage + c.tickBonus + c.tickDamage) {
		c.failed = fmt.Errorf("its damage is NaN, bonus %v damage %v", c.bonus, c.damage)
	}
	return c, landed, false
}

// settle runs the actions queued for now and stops before the first one later, so a dot's own ticks
// stay out of it.
func (ts *threatSim) settle() {
	done := false
	ts.sim.AddPendingAction(&core.PendingAction{
		NextActionAt: ts.sim.CurrentTime + time.Nanosecond,
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
	c := ts.cast(rec)
	if c.failed != nil {
		return []Check{fail("sim cast", fmt.Sprintf("spell %v: %v", ts.spells[0].ActionID, c.failed))}
	}

	var checks []Check
	d := rec.Damage
	if c.damage > 0 || c.tickDamage > 0 || threatServerDamage(rec.Spell.ID) {
		checks = append(checks, checkThreatNear("damage threat", c.slope*d.Amount, d.Threat, fmt.Sprintf(
			"sim x%.4f (spell %.4f, unit %.4f), server x%.4f (spell_threat %.4f, after spell mods %.2f, school %.4f)",
			c.slope, c.spellMultiplier, ts.char.PseudoStats.ThreatMultiplier, d.Threat/d.Amount, d.PctMod,
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
	checks = append(checks, checkThreatNear("bonus", c.bonus, rec.Bonus.Threat, threatJoinDetail(rec.serverBonus(), c.note)))
	if c.ticked {
		// an aura's periodic damage never runs HandleThreatSpells
		checks = append(checks, checkThreatNear("tick bonus", c.tickBonus, 0, "server ticks add no bonus"))
	}
	return checks
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
	return text
}

// threatMismatch names a check of one spell for one server class in one form, split over a target count
// the probe was given (0 for the cast's own).
type threatMismatch struct {
	class    uint8
	form     int32
	spell    int32
	override int32
	check    string
}

func threatMismatchOf(rec threatRecord, check string) threatMismatch {
	return threatMismatch{rec.Attacker.Class, rec.form(), rec.Spell.ID, rec.override(), check}
}

// knownThreatMismatches are checks where the sim and the server really differ, each with its reason and a
// row in the fixture README. The check still runs; only its failure is let through. An entry no record
// needs fails TestThreatRecords.
var knownThreatMismatches = func() map[threatMismatch]string {
	const (
		level     = "the server adds the spell's level per cast when spell_threat has no row for it, the sim nothing"
		levelHalf = "the server adds the spell's level split between the dummy and the caster an effect also hits, the sim nothing"
		scaled    = "the sim multiplies the spell_threat bonus by the unit's threat multiplier, the server adds it unmodified"
		simFlat   = "the sim adds a flat bonus the server has no spell_threat row for, so the server adds the spell's level"
		simPct    = "the sim multiplies this spell's threat, the server has no spell_threat percent for it"
		bear      = "Dire Bear Form is x2.1021 in the sim, x2.0735 on the server"
		unholy    = "the server's Unholy Presence takes 20% off threat besides Subversion's 25% (x0.55), the sim only Subversion's (x0.75)"
		shadow    = "Shadowform takes 30% off threat on the server and nothing in the sim, and Shadow Affinity takes 25% there, 24% in the sim"
	)
	m := map[threatMismatch]string{}
	add := func(reason string, class uint8, form int32, check string, spells ...int32) {
		for _, spell := range spells {
			m[threatMismatch{class, form, spell, 0, check}] = reason
		}
	}

	add(level, 1, 71, "bonus", 12809, 47471)
	add(level, 1, 2458, "bonus", 23881, 1680, 47471, 47486, 7384, 47465)
	add(level, 2, 25780, "bonus", 48827, 53595, 61411, 20187, 48801, 48819, 35395)
	add(level, 2, 0, "bonus", 35395, 48801, 20187, 48806, 48819)
	add(level, 11, 9634, "bonus", 48564)
	add(level, 11, 768, "bonus", 48572, 48566, 48574, 49800, 48577, 62078)
	add(level, 6, 48263, "bonus", 49909, 55262, 49924, 49941, 47632)
	add(level, 6, 48265, "bonus", 49930, 51425, 47632, 49909, 55262)
	add(level, 4, 0, "bonus", 48638, 48657, 51723, 57965, 48660)
	add(level, 8, 0, "bonus", 44781, 42833, 42926, 42842, 47610, 42914, 55360, 42859)
	add(level, 5, 0, "bonus", 48127, 48125, 48160, 48300, 48158, 48123)
	add(level, 9, 0, "bonus", 47809, 47838, 47811, 47813, 59172, 47864, 47865, 47867, 50511)
	add(level, 7, 0, "bonus", 49238, 49271, 60043, 49231, 49233, 61654)
	add(levelHalf, 2, 0, "bonus", 53385)
	add(levelHalf, 4, 0, "bonus", 48668, 48672, 57993)
	add(levelHalf, 8, 0, "bonus", 42897)
	add(levelHalf, 5, 0, "bonus", 48156)

	add(scaled, 1, 71, "bonus", 47450, 57823, 47488)
	add(scaled, 1, 2458, "bonus", 47450)
	const cleave = "the sim's Cleave bonus is 225 times the unit's threat multiplier, spell_threat's is 112 unmodified"
	add(cleave, 1, 71, "bonus", 47520)
	add(cleave, 1, 2458, "bonus", 47520)
	m[threatMismatch{1, 71, 47520, 2, "bonus"}] = cleave + " and split over the targets hit"
	add("the sim's Maul bonus is 424 times Dire Bear Form's multiplier, spell_threat's is 422 unmodified", 11, 9634, "bonus", 48480)
	add("the sim's Lacerate bonus is 515.5 times Dire Bear Form's multiplier, spell_threat's is 515 unmodified", 11, 9634, "bonus", 48568)
	add("the sim's Devastate adds 315 and 5% of attack power, times the unit's threat multiplier; "+
		"spell_threat has the 5% alone, unmodified", 1, 71, "bonus", 47498)
	const slam = "the sim puts Slam's 140 on its damage spell, which the server gives its level of 1; the server's row " +
		"is on the cast, whose bonus SPELL_ATTR3_SUPPRESS_TARGET_PROCS drops"
	add(slam, 1, 71, "bonus", 50783)
	add(slam, 1, 2458, "bonus", 50783)
	add("the server's Sunder Armor row (7386) isn't in this rank's chain, so it adds the spell's level; the sim adds "+
		"360 and 5% of attack power, times the unit's threat multiplier", 1, 71, "bonus", 47467)
	add(simFlat, 1, 71, "bonus", 47437)
	add(simFlat, 11, 9634, "bonus", 48560)
	const fffCast = "the sim puts Faerie Fire (Feral)'s flat on the cast, the server's 638 is on the damage spell 60089 and the cast adds its level"
	add(fffCast, 11, 9634, "bonus", 16857)
	add(fffCast, 11, 768, "bonus", 16857)
	add("the server's 638 for Faerie Fire (Feral) is on this damage spell, the sim puts its flat on the cast 16857",
		11, 9634, "bonus", 60089)
	const scourge = "spell_threat gives Scourge Strike 120, the sim nothing"
	add(scourge, 6, 48263, "bonus", 55271)
	add(scourge, 6, 48265, "bonus", 55271)
	add("the server's bonus for this positive spell reaches the dummy as assist threat, its level x0.7; the sim adds nothing",
		7, 0, "bonus", 61657)
	add("the sim adds Curse of Doom's 160 to each tick, the server adds no bonus to ticks", 9, 0, "tick bonus", 47867)

	add(simPct, 1, 71, "damage threat", 47488, 12809, 47471)
	add(simPct, 1, 2458, "damage threat", 1680, 47471, 7384)
	add("the sim never applies Fanaticism's 30% threat reduction (applyFanaticism is never called)", 2, 0, "damage threat",
		35395, 53385, 48801, 20187, 48806, 48819, 20424)
	add(bear, 11, 9634, "damage threat", 48480, 48568, 48564, 60089)
	const swipe = "spell_threat gives Swipe (Bear) x1.5, the sim nothing; " + bear
	add(swipe, 11, 9634, "damage threat", 48562)
	m[threatMismatch{11, 9634, 48562, 3, "damage threat"}] = swipe
	add(unholy, 6, 48265, "damage threat", 55271, 49930, 51425, 47632, 49909, 55262, 52212)
	add("the sim's Flamestrike takes 5% per Burning Soul point, the server 10%", 8, 0, "damage threat", 42926)
	add("the server multiplies Burning Soul's and Frost Channeling's school multipliers (x0.72), the sim adds them (x0.68)",
		8, 0, "damage threat", 47610)
	add(shadow, 5, 0, "damage threat", 48127, 58381, 48125, 48160, 48158)
	add("Shadowform takes 30% off threat on the server and nothing in the sim, and the sim's Devouring Plague takes "+
		"only 15% for Shadow Affinity", 5, 0, "damage threat", 48300)
	add("Shadowform takes 30% off threat in every school on the server, nothing in the sim", 5, 0, "damage threat", 48123)
	add("spell_threat gives Searing Pain x2, the sim nothing", 9, 0, "damage threat", 47815)
	return m
}()
