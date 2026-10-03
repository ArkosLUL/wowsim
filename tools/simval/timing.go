package main

import (
	"fmt"
	"math"
	"slices"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// `.simval timing` records one spell's cast time, GCD, cooldowns, cost, channel and aura timing with the
// caster's mods and haste, and the bot's `info` records carry its regen. The replay builds the sim's
// character from the record and reads the same numbers off the sim's own spell, dot and aura.

const timingFixture = "timing.jsonl"

const bloodlustSpell = 2825

// regenTolerance is how far the sim's regen may sit from the server's, relative.
const regenTolerance = 0.001

type timingRegen struct {
	Flat        []float64 `json:"flat"`
	Interrupted []float64 `json:"interrupted"`
	Rune        []float64 `json:"rune"`
	ManaRate    float64   `json:"manaRate"`
	EnergyRate  float64   `json:"energyRate"`
}

type timingUnit struct {
	damageUnit
	Regen *timingRegen `json:"regen"`
}

type timingEffect struct {
	Index       int32 `json:"index"`
	Periodic    bool  `json:"periodic"`
	AmplitudeMs int32 `json:"amplitudeMs"`
	Ticks       int32 `json:"ticks"`
}

// timingRecord is a `.simval timing` line, or a bot's `info` line for the regen.
type timingRecord struct {
	Command string `json:"command"`
	Spell   struct {
		ID        int32  `json:"id"`
		Name      string `json:"name"`
		Channeled bool   `json:"channeled"`
	} `json:"spell"`
	Attacker timingUnit `json:"attacker"`
	Target   timingUnit `json:"target"`

	Cast struct {
		AfterSpellModsMs int32   `json:"afterSpellModsMs"`
		Ms               int32   `json:"ms"`
		Haste            string  `json:"haste"`
		CastSpeed        float64 `json:"castSpeed"`
		RangedSpeed      float64 `json:"rangedSpeed"`
	} `json:"cast"`
	GCD struct {
		Category int32 `json:"category"`
		Ms       int32 `json:"ms"`
	} `json:"gcd"`
	Cooldown *struct {
		StartsOnEvent bool  `json:"startsOnEvent"`
		Ms            int32 `json:"ms"`
		CategoryMs    int32 `json:"categoryMs"`
	} `json:"cooldown"`
	Cost struct {
		// a health cost is POWER_HEALTH, -2, which the module writes unsigned
		PowerType uint32 `json:"powerType"`
		Amount    int32  `json:"amount"`
		Runes     *struct {
			Cost           []int32 `json:"cost"`
			RunicPowerGain int32   `json:"runicPowerGain"`
			BaseCooldownMs []int32 `json:"baseCooldownMs"`
		} `json:"runes"`
	} `json:"cost"`
	ChannelMs *int32 `json:"channelMs"`
	Aura      struct {
		Applies  bool           `json:"applies"`
		OnCaster bool           `json:"onCaster"`
		BaseMs   int32          `json:"baseMs"`
		Skipped  string         `json:"skipped"`
		Immune   bool           `json:"immune"`
		Ms       int32          `json:"ms"`
		Effects  []timingEffect `json:"effects"`
	} `json:"aura"`
}

func (r timingRecord) label() string {
	if r.Command == "info" {
		return "regen: " + r.Attacker.Name
	}
	return fmt.Sprintf("timing %d (%s): %s", r.Spell.ID, r.Spell.Name, r.Attacker.Name)
}

// damageRecord is the part of the record the damage replay's character builders read.
func (r timingRecord) damageRecord() damageRecord {
	var d damageRecord
	d.Command = "damage"
	d.Spell.ID, d.Spell.Name = r.Spell.ID, r.Spell.Name
	d.Attacker, d.Target = r.Attacker.damageUnit, r.Target.damageUnit
	return d
}

func (r timingRecord) hasAura(id int32) bool {
	return slices.ContainsFunc(r.Attacker.Auras, func(a aura) bool { return a.ID == id })
}

// pass is which of TestSimvalTiming's three passes took the record: bare, with the talents, or
// under Bloodlust on top of them.
func (r timingRecord) pass() (string, error) {
	if r.hasAura(bloodlustSpell) {
		return "bloodlust", nil
	}
	setup, err := damageSetupFor(r.damageRecord())
	if err != nil {
		return "", err
	}
	if setup.modded {
		return "talents", nil
	}
	return "bare", nil
}

func readTimingRecords(path string) ([]timingRecord, error) {
	return readJSONLines[timingRecord](path)
}

// timingTaughtSpells are the spells a talent teaches, which the server probes without it.
var timingTaughtSpells = map[int32]string{
	47486: "mortalStrike", 49050: "aimedShot", 48156: "mindFlay", 48982: "runeTap", 51411: "howlingBlast",
	59172: "chaosBolt",
}

// timingExtraSpecs are specs beyond the damage replay's that register a spell the probes use, by server
// class id: Healing Wave is the shaman's.
var timingExtraSpecs = map[uint8][]protoreflect.Name{7: {"restoration_shaman"}}

// timingSim is the record's character with the spell the record is about.
type timingSim struct {
	*damageSim
	spell  *core.Spell
	quiver bool
}

// start is damageSim.start with the quiver's haste on, which a reset takes off again.
func (ts *timingSim) start() {
	ts.damageSim.start()
	if ts.quiver {
		ts.char.MultiplyRangedSpeed(ts.sim, 1.1)
	}
}

// newTimingSim is newDamageSim with Bloodlust registered for a record taken under it.
func newTimingSim(rec damageRecord, player *proto.Player, mirror []int32) (*damageSim, error) {
	return newSimWith(rec, player, mirror, 1, &proto.RaidBuffs{Bloodlust: slices.Contains(mirror, bloodlustSpell)})
}

// smallQuiverHaste is the +10% ranged speed aura of the quivers the server's bare bot carries (Light Quiver
// and its kin). The replay character has no bag, and the sim's quiver option is the 15% one only.
const smallQuiverHaste = 29418

// auraBySpell is the aura of a spell id, the lowest tag of several.
func auraBySpell(auras []*core.Aura, id int32) *core.Aura {
	var found *core.Aura
	for _, a := range auras {
		if a.ActionID.SpellID == id && a.ActionID.ItemID == 0 && a.ActionID.OtherID == 0 &&
			(found == nil || a.ActionID.Tag < found.ActionID.Tag) {
			found = a
		}
	}
	return found
}

func spellBySpell(unit *core.Unit, id int32) *core.Spell {
	var found *core.Spell
	for _, s := range unit.Spellbook {
		if s.ActionID.SpellID == id && s.ActionID.ItemID == 0 && s.ActionID.OtherID == 0 &&
			(found == nil || s.ActionID.Tag < found.ActionID.Tag) {
			found = s
		}
	}
	return found
}

// buildTimingSim tries the class's specs until one registers the record's spell, then again with the
// talent that teaches it. A record with no spell (an `info`) takes the class's first spec.
func buildTimingSim(rec timingRecord) (*timingSim, error) {
	dr := rec.damageRecord()
	setup, err := damageSetupFor(dr)
	if err != nil {
		return nil, err
	}
	setups := []damageSetup{setup}
	field, ok := timingTaughtSpells[rec.Spell.ID]
	if !ok {
		field, ok = talentTaughtSpells[rec.Spell.ID]
	}
	if ok {
		if taught, ok := setup.withTalent(rec.Attacker.Class, field); ok {
			setups = append(setups, taught)
		}
	}
	class := classes[rec.Attacker.Class]
	specs := append([]protoreflect.Name{class.spec}, class.otherSpecs...)
	specs = append(specs, timingExtraSpecs[rec.Attacker.Class]...)

	for _, setup := range setups {
		for _, spec := range specs {
			player, err := damagePlayer(dr, setup, spec)
			if err != nil {
				return nil, err
			}
			d, err := newTimingSim(dr, player, setup.others)
			if err != nil {
				return nil, err
			}
			quiver := rec.hasAura(smallQuiverHaste)
			if rec.Spell.ID == 0 {
				return &timingSim{damageSim: d, quiver: quiver}, nil
			}
			if spell := spellBySpell(&d.char.Unit, rec.Spell.ID); spell != nil {
				return &timingSim{damageSim: d, spell: spell, quiver: quiver}, nil
			}
		}
	}
	return nil, fmt.Errorf("no %v spec registers spell %d", specs, rec.Spell.ID)
}

// simMs is a duration in whole ms, leaving float noise out of the truncation.
func simMs(d time.Duration) int64 {
	return int64(math.Floor(float64(d)/float64(time.Millisecond) + 1e-6))
}

func checkRelative(name string, sim, server float64) Check {
	detail := fmt.Sprintf("sim %.4f, server %.4f", sim, server)
	if math.Abs(sim-server) <= regenTolerance*math.Abs(server)+1e-9 {
		return pass(name, detail)
	}
	return fail(name, detail)
}

// notCompared is a pass that says what it left out and why.
func notCompared(name, why string) Check { return pass(name+" (not compared)", why) }

// checkTiming compares a record with the sim.
func checkTiming(rec timingRecord) []Check {
	return replayTiming(rec, buildTimingSim)
}

func replayTiming(rec timingRecord, build func(timingRecord) (*timingSim, error)) []Check {
	checks := replayTimingChecks(rec, build)
	for i, c := range checks {
		if reason, ok := knownTimingMismatches[timingMismatchOf(rec, c.Name)]; ok && !c.Passed {
			checks[i] = pass(c.Name+knownMismatchSuffix, reason+"; "+c.Detail)
		}
	}
	return checks
}

const knownMismatchSuffix = " (known mismatch)"

// timingMismatchOf is the knownTimingMismatches key of a check on the record.
func timingMismatchOf(rec timingRecord, check string) timingMismatch {
	passName, _ := rec.pass()
	return timingMismatch{rec.Attacker.Class, rec.Spell.ID, passName, check}
}

func replayTimingChecks(rec timingRecord, build func(timingRecord) (*timingSim, error)) (checks []Check) {
	if !rec.Attacker.IsPlayer || rec.Attacker.Level != core.CharacterLevel {
		return []Check{fail("attacker", fmt.Sprintf("level %d player=%v; the replay builds a level %d player",
			rec.Attacker.Level, rec.Attacker.IsPlayer, core.CharacterLevel))}
	}
	if rec.Command == "info" && rec.Attacker.Regen == nil {
		return nil
	}
	ts, err := build(rec)
	if err != nil {
		return []Check{fail("sim character", err.Error())}
	}
	defer func() {
		if r := recover(); r != nil {
			checks = append(checks, fail("sim", fmt.Sprintf("panic: %v", r)))
		}
	}()
	ts.start()

	if rec.Command == "info" {
		return ts.checkRegen(rec)
	}
	checks = append(checks, ts.checkCast(rec)...)
	checks = append(checks, ts.checkGCD(rec)...)
	checks = append(checks, ts.checkCooldown(rec)...)
	checks = append(checks, ts.checkCost(rec)...)
	checks = append(checks, ts.checkDuration(rec)...)
	return checks
}

func (ts *timingSim) checkCast(rec timingRecord) []Check {
	unit := &ts.char.Unit
	checks := []Check{checkNear("cast speed", unit.CastSpeed, rec.Cast.CastSpeed, 1e-4)}
	if rec.Cast.Haste == "ranged" {
		checks = append(checks, checkNear("ranged speed", 1/unit.RangedSwingSpeed(), rec.Cast.RangedSpeed, 1e-4))
	}
	checks = append(checks, checkInt("cast time before haste", simMs(ts.spell.DefaultCast.CastTime), int64(max(0, rec.Cast.AfterSpellModsMs))))
	if rec.Cast.Haste == "instant" {
		return append(checks, notCompared("cast time", "the server made the cast instant, which the sim has no rule for"))
	}
	return append(checks, checkInt("cast time", simMs(ts.spell.CastTime()), int64(rec.Cast.Ms)))
}

// gcd is the spell's GCD after haste: its effective time with the cast time taken out.
func (ts *timingSim) gcd() time.Duration {
	spell := ts.spell
	cast := spell.DefaultCast.CastTime
	spell.DefaultCast.CastTime = 0
	defer func() { spell.DefaultCast.CastTime = cast }()
	return spell.EffectiveCastTime()
}

func (ts *timingSim) checkGCD(rec timingRecord) []Check {
	// the sim's one GCD is category 133's; any other category only holds back its own spells
	if rec.GCD.Category != 133 && rec.GCD.Ms > 0 {
		return []Check{notCompared("gcd", fmt.Sprintf("the server's %d ms GCD is in category %d, which the sim has no GCD for",
			rec.GCD.Ms, rec.GCD.Category))}
	}
	return []Check{checkInt("gcd", simMs(ts.gcd()), int64(rec.GCD.Ms))}
}

func (ts *timingSim) checkCooldown(rec timingRecord) []Check {
	cd := rec.Cooldown
	if cd == nil {
		return nil
	}
	spell := ts.spell
	var checks []Check
	if cd.StartsOnEvent {
		checks = append(checks, notCompared("cooldown", "the server starts it on an event"))
	} else {
		own := int64(0)
		if spell.CD.Timer != nil {
			own = simMs(spell.CD.Duration)
		}
		checks = append(checks, checkInt("cooldown", own, int64(cd.Ms)))
	}
	if cd.CategoryMs > 0 {
		if spell.SharedCD.Timer == nil {
			checks = append(checks, notCompared("category cooldown", "the sim has no shared cooldown for this spell"))
		} else {
			checks = append(checks, checkInt("category cooldown", simMs(spell.SharedCD.Duration), int64(cd.CategoryMs)))
		}
	}
	return checks
}

// The server counts rage and runic power in tenths.
func powerScale(powerType uint32) float64 {
	if powerType == 1 || powerType == 6 {
		return 10
	}
	return 1
}

func (ts *timingSim) checkCost(rec timingRecord) []Check {
	spell := ts.spell
	runes := rec.Cost.Runes
	impl, simRunes := spell.Cost.(*core.RuneCostImpl)
	if !simRunes {
		var checks []Check
		cost := spell.ApplyCostModifiers(spell.DefaultCast.Cost) * powerScale(rec.Cost.PowerType)
		checks = append(checks, checkInt("cost", int64(math.Floor(cost+1e-6)), int64(rec.Cost.Amount)))
		if runes != nil && slices.ContainsFunc(runes.Cost, func(c int32) bool { return c != 0 }) {
			checks = append(checks, fail("rune cost", fmt.Sprintf("server %v, the sim spell costs no runes", runes.Cost)))
		}
		return checks
	}

	if runes == nil || len(runes.Cost) != 3 {
		return []Check{fail("rune cost", "the sim spell costs runes, the record has no rune cost")}
	}
	checks := []Check{
		checkInt("blood rune cost", int64(impl.BloodRuneCost), int64(runes.Cost[0])),
		checkInt("unholy rune cost", int64(impl.UnholyRuneCost), int64(runes.Cost[1])),
		checkInt("frost rune cost", int64(impl.FrostRuneCost), int64(runes.Cost[2])),
		checkInt("runic power gain", int64(math.Round(impl.RunicPowerGain*10)), int64(runes.RunicPowerGain)),
	}
	// a runic power cost is the spell's mana cost entry; a rune spell's own amount is 0
	checks = append(checks, checkInt("runic power cost",
		int64(math.Round(impl.RunicPowerCost*10)), int64(rec.Cost.Amount)))
	for i, server := range runes.BaseCooldownMs {
		checks = append(checks, checkRelative(fmt.Sprintf("rune %d cooldown", i), float64(ts.runeCooldown()), float64(server)))
	}
	return checks
}

// runeCooldown is a rune's cooldown in ms, with no rune grace.
func (ts *timingSim) runeCooldown() int64 {
	return simMs(ts.char.SpendRuneReadyAt(0, 0))
}

// safeDot is the dot the lookup returns, nil where the spell has none of that kind for the unit.
func safeDot(get func() *core.Dot) (dot *core.Dot) {
	defer func() {
		if recover() != nil {
			dot = nil
		}
	}()
	return get()
}

// dot finds the spell's dot or hot: on the dummy for a harmful spell, on the caster for a beneficial one.
func (ts *timingSim) dot(onCaster bool) *core.Dot {
	spell, caster := ts.spell, &ts.char.Unit
	victim := ts.target
	if onCaster {
		victim = caster
	}
	for _, get := range []func() *core.Dot{
		func() *core.Dot { return spell.Dot(victim) },
		func() *core.Dot { return spell.Hot(victim) },
		spell.AOEDot,
		spell.AOEHot,
		spell.SelfHot,
	} {
		if dot := safeDot(get); dot != nil {
			return dot
		}
	}
	return nil
}

func (ts *timingSim) checkDuration(rec timingRecord) []Check {
	onCaster := rec.Aura.OnCaster
	dot := ts.dot(onCaster)
	if dot != nil {
		// the haste on the character now, after Bloodlust is on
		dot.RecomputeAuraDuration()
	}

	var checks []Check
	if rec.ChannelMs != nil {
		if dot == nil {
			checks = append(checks, notCompared("channel", "the sim has no dot for the channel"))
		} else {
			checks = append(checks, checkInt("channel", simMs(dot.Aura.Duration), int64(*rec.ChannelMs)))
		}
	}

	a := rec.Aura
	switch {
	case !a.Applies:
		return checks
	case a.Immune || a.Skipped != "":
		return append(checks, notCompared("aura", fmt.Sprintf("immune %v, skipped %q", a.Immune, a.Skipped)))
	}

	if dot == nil {
		unit := ts.target
		if onCaster {
			unit = &ts.char.Unit
		}
		found := auraBySpell(unit.GetAuras(), rec.Spell.ID)
		switch {
		case found == nil:
			return append(checks, notCompared("aura duration", "the sim has no aura of this spell on that unit"))
		case a.BaseMs <= 0:
			return append(checks, notCompared("aura duration", "no duration"))
		}
		return append(checks, checkInt("aura duration", simMs(found.Duration), int64(a.Ms)))
	}

	checks = append(checks, checkInt("aura duration", simMs(dot.Aura.Duration), int64(a.Ms)))
	for _, e := range a.Effects {
		if !e.Periodic {
			continue
		}
		name := fmt.Sprintf("e%d", e.Index)
		checks = append(checks,
			checkInt(name+" amplitude", simMs(dot.TickPeriod()), int64(e.AmplitudeMs)),
			checkInt(name+" ticks", int64(dot.TotalTicks()), int64(e.Ticks)))
	}
	return checks
}

// checkRegen compares mana and energy regen per second and the runes' cooldowns. The server keeps mana
// regen per second for each state of the five second rule, energy as 10 per second plus a flat bonus.
func (ts *timingSim) checkRegen(rec timingRecord) []Check {
	regen, unit := rec.Attacker.Regen, &ts.char.Unit
	if len(regen.Flat) != 7 || len(regen.Interrupted) != 7 || len(regen.Rune) != 4 {
		return []Check{fail("regen", fmt.Sprintf("flat %d, interrupted %d and rune %d values", len(regen.Flat),
			len(regen.Interrupted), len(regen.Rune)))}
	}
	var checks []Check
	if rec.Attacker.PowerType == 0 && unit.HasManaBar() {
		// the sim's mage, priest and druid carry their own class's raid buff (Arcane Brilliance, Divine
		// Spirit, Gift of the Wild) and its stats aren't truncated, so regen is read at the server's
		if st := rec.Attacker.Stats; st.Intellect > 0 && st.Spirit > 0 {
			setStat(ts.sim, unit, stats.Intellect, st.Intellect)
			setStat(ts.sim, unit, stats.Spirit, st.Spirit)
		}
		checks = append(checks,
			checkRelative("mana regen", unit.ManaRegenPerSecondWhileNotCasting(), regen.Flat[0]*regen.ManaRate),
			checkRelative("mana regen while casting", unit.ManaRegenPerSecondWhileCasting(), regen.Interrupted[0]*regen.ManaRate))
	}
	if rec.Attacker.PowerType == 3 && unit.HasEnergyBar() {
		perSecond := core.EnergyPerTick * unit.EnergyTickMultiplier * float64(time.Second/core.EnergyTickDuration)
		checks = append(checks, checkRelative("energy regen", perSecond, (regen.Flat[3]+10)*regen.EnergyRate))
	}
	if unit.HasRunicPowerBar() {
		for i, rate := range regen.Rune[:3] {
			checks = append(checks, checkRelative(fmt.Sprintf("rune %d cooldown", i), float64(ts.runeCooldown()), 1000/rate))
		}
	}
	return checks
}

// The three passes of TestSimvalTiming, as timingRecord.pass names them.
var allPasses = []string{"bare", "talents", "bloodlust"}

// timingMismatch names a check of one spell for one server class id in one pass.
type timingMismatch struct {
	class uint8
	spell int32
	pass  string
	check string
}

// knownTimingMismatches are checks where the sim and the server really differ, each with its reason and a
// row in the fixture README. The check still runs; only its failure is let through, in the passes named.
// An entry no record needs fails TestTimingRecords.
var knownTimingMismatches = func() map[timingMismatch]string {
	const (
		comboless = "the probe has no combo points, so the sim's finisher has no duration"
	)
	m := map[timingMismatch]string{}
	add := func(reason string, class uint8, spell int32, passes []string, checks ...string) {
		for _, p := range passes {
			for _, c := range checks {
				m[timingMismatch{class, spell, p, c}] = reason
			}
		}
	}
	add(comboless, 4, 48672, allPasses, "aura duration", "e0 ticks")
	add(comboless, 4, 6774, allPasses, "aura duration")
	return m
}()
