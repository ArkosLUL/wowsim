package core

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// A cast corresponds to any action which causes the in-game castbar to be
// shown, and activates the GCD. Note that a cast can also be instant, i.e.
// the effects are applied immediately even though the GCD is still activated.

// Callback for when a cast is finished, i.e. when the in-game castbar reaches full.
type OnCastComplete func(aura *Aura, sim *Simulation, spell *Spell)

type Hardcast struct {
	Expires    time.Duration
	ActionID   ActionID
	OnComplete func(*Simulation, *Unit)
	Target     *Unit
}

// Input for constructing the CastSpell function for a spell.
type CastConfig struct {
	// Default cast values with all static effects applied.
	DefaultCast Cast

	// Dynamic modifications for each cast.
	ModifyCast func(*Simulation, *Spell, *Cast)

	// Ignores haste when calculating the GCD and cast time for this cast.
	// Automatically set if GCD and cast times are all 0, e.g. for empty casts.
	IgnoreHaste bool

	CD       Cooldown
	SharedCD Cooldown

	CastTime func(spell *Spell) time.Duration
}

type Cast struct {
	// Amount of resource that will be consumed by this cast.
	Cost float64

	// The length of time the GCD will be on CD as a result of this cast.
	GCD time.Duration

	// The amount of time between the call to spell.Cast() and when the spell
	// effects are invoked.
	CastTime time.Duration
}

// EffectiveTime reads the GCD as is: makeCastFunc has already applied its floor, if it has one.
func (cast *Cast) EffectiveTime() time.Duration {
	return max(cast.GCD, cast.CastTime)
}

// castTiming is how a spell's cast time and GCD are worked out, read once at registration from its
// server data. Spell.CastTime and Spell.EffectiveCastTime go through it too, so what the APL predicts
// is what the cast costs.
type castTiming struct {
	serverData bool

	// the spell's CastConfig.IgnoreHaste: nothing scales its cast time or GCD
	ignoreHaste bool

	// Spell::TriggerGlobalCooldown only hastes and clamps a GCD of 1000-1500 ms
	clampGCD bool
	hasteGCD bool

	castHaste castHaste

	resetsAutoAttack bool
	// SpellInfo::CalcCastTime without a caster: when there is one, a cast made instant doesn't reset autos
	hasCastTime bool
	// SPELL_AURA_IGNORE_MELEE_RESET auras whose class mask covers the spell
	ignoreResetAuras []ActionID
}

func newCastTiming(spell *Spell, ignoreHaste bool) castTiming {
	ct := castTiming{ignoreHaste: ignoreHaste}
	sd := spell.ServerSpell()
	if sd == nil {
		return ct
	}
	ct.serverData = true
	ct.clampGCD = time.Duration(sd.GCDMs)*time.Millisecond >= GCDMin && time.Duration(sd.GCDMs)*time.Millisecond <= GCDDefault
	ct.hasteGCD = sd.Flags&serverdata.FlagHasteGCD != 0
	ct.castHaste = castHasteFor(sd)
	ct.resetsAutoAttack = sd.Flags&serverdata.FlagResetsAutoAttack != 0
	ct.hasCastTime = sd.CastMs > 0
	ct.ignoreResetAuras = ignoreMeleeResetAuras(sd)
	return ct
}

// gcd is Spell::TriggerGlobalCooldown for spells with server data. The rest keep the sim's own rule:
// hasted unless IgnoreHaste, never below GCDMin.
func (ct *castTiming) gcd(unit *Unit, gcd time.Duration) time.Duration {
	switch {
	case gcd == 0:
		return 0
	case !ct.serverData:
		if !ct.ignoreHaste {
			gcd = unit.ApplyCastSpeed(gcd)
		}
		return max(GCDMin, gcd)
	case ct.clampGCD:
		// A sim GCD over 1500 ms can't come from the server's data: it stands in for something else, like
		// hunter pets' AI delay or Shadowcrawl's 6 s cooldown, so the upper clamp leaves it alone.
		capped := gcd <= GCDDefault
		if ct.hasteGCD {
			gcd = unit.ApplyCastSpeed(gcd)
		}
		gcd = max(gcd, GCDMin)
		if capped {
			gcd = min(gcd, GCDDefault)
		}
		return gcd
	}
	return gcd
}

type castHaste uint8

const (
	castHasteSpell castHaste = iota
	castHasteRanged
	castHasteNone
)

// castHasteFor is Unit::ModSpellCastTime's switch on the damage class.
func castHasteFor(sd *serverdata.Spell) castHaste {
	switch sd.DmgClass {
	case serverdata.DmgClassMagic:
		return castHasteSpell
	case serverdata.DmgClassRanged:
		return castHasteRanged
	case serverdata.DmgClassNone:
		if sd.Flags&serverdata.FlagHasteAffectsPeriodic != 0 {
			return castHasteSpell
		}
	}
	return castHasteNone
}

func (ct *castTiming) castTime(unit *Unit, castTime time.Duration, spell *Spell) time.Duration {
	if ct.ignoreHaste {
		return castTime
	}
	switch ct.castHaste {
	case castHasteRanged:
		return unit.ApplyRangedCastSpeed(castTime, spell)
	case castHasteNone:
		return time.Duration(float64(castTime) * spell.CastTimeMultiplier)
	}
	return unit.ApplyCastSpeedForSpell(castTime, spell)
}

// resetsSwing is the rest of Spell::IsAutoActionResetSpell and its caller, for an untriggered cast.
func (ct *castTiming) resetsSwing(spell *Spell) bool {
	if !ct.resetsAutoAttack || (ct.hasCastTime && spell.CurCast.CastTime == 0) {
		return false
	}
	for _, id := range ct.ignoreResetAuras {
		if spell.Unit.GetAuraByID(id).IsActive() {
			return false
		}
	}
	return true
}

type ignoreMeleeResetEffect struct {
	auraID    int32
	family    int32
	classMask [3]uint32
}

var ignoreMeleeResetEffects = sync.OnceValue(func() []ignoreMeleeResetEffect {
	var effects []ignoreMeleeResetEffect
	for _, s := range serverdata.Spells() {
		for _, e := range s.Effects {
			if e.Aura == auraTypeIgnoreMeleeReset {
				effects = append(effects, ignoreMeleeResetEffect{auraID: s.ID, family: s.Family, classMask: e.ClassMask})
			}
		}
	}
	return effects
})

// affects is SpellInfo::IsAffected: family 0 covers every spell, an empty class mask the whole family.
func (e ignoreMeleeResetEffect) affects(sd *serverdata.Spell) bool {
	switch {
	case e.family == 0:
		return true
	case e.family != sd.Family:
		return false
	case e.classMask == [3]uint32{}:
		return true
	}
	return e.classMask[0]&sd.FamilyFlags[0] != 0 || e.classMask[1]&sd.FamilyFlags[1] != 0 || e.classMask[2]&sd.FamilyFlags[2] != 0
}

// ignoreMeleeResetAuras is AuraEffect::IsAffectedOnSpell over every SPELL_AURA_IGNORE_MELEE_RESET effect.
func ignoreMeleeResetAuras(sd *serverdata.Spell) []ActionID {
	var ids []ActionID
	for _, e := range ignoreMeleeResetEffects() {
		if id := (ActionID{SpellID: e.auraID}); e.affects(sd) && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

type CastFunc func(*Simulation, *Unit)
type CastSuccessFunc func(*Simulation, *Unit) bool

func (spell *Spell) castFailureHelper(sim *Simulation, message string, vals ...any) bool {
	if sim.CurrentTime < 0 && spell.Unit.Rotation != nil {
		spell.Unit.Rotation.ValidationWarning(fmt.Sprintf(spell.ActionID.String()+" failed to cast: "+message, vals...))
	} else {
		if sim.Log != nil && !spell.Flags.Matches(SpellFlagNoLogs) {
			spell.Unit.Log(sim, fmt.Sprintf(spell.ActionID.String()+" failed to cast: "+message, vals...))
		}
	}
	return false
}

func (spell *Spell) makeCastFunc(config CastConfig) CastSuccessFunc {
	return func(sim *Simulation, target *Unit) bool {
		spell.CurCast = spell.DefaultCast

		if config.ModifyCast != nil {
			config.ModifyCast(sim, spell, &spell.CurCast)
			if spell.CurCast.Cost != spell.DefaultCast.Cost {
				// Costs need to be modified using the unit and spell multipliers, so that
				// their affects are also visible in the spell.CanCast() function, which
				// does not invoke ModifyCast.
				panic("May not modify cost in ModifyCast!")
			}
		}

		if spell.ExtraCastCondition != nil {
			if !spell.ExtraCastCondition(sim, target) {
				return spell.castFailureHelper(sim, "extra spell condition")
			}
		}

		if spell.Cost != nil {
			if !spell.Cost.MeetsRequirement(sim, spell) {
				return spell.castFailureHelper(sim, spell.Cost.CostFailureReason(sim, spell))
			}
		}

		spell.CurCast.GCD = spell.timing.gcd(spell.Unit, spell.CurCast.GCD)
		spell.CurCast.CastTime = spell.timing.castTime(spell.Unit, spell.CurCast.CastTime, spell)
		if spell.CurCast.CastTime > 0 {
			// the cast lands on a server tick, and its CD starts then too
			spell.CurCast.CastTime = sim.NextServerTick(sim.CurrentTime+spell.CurCast.CastTime) - sim.CurrentTime
		}

		if config.CD.Timer != nil {
			// By panicking if spell is on CD, we force each sim to properly check for their own CDs.
			if !spell.CD.IsReady(sim) {
				return spell.castFailureHelper(sim, "still on cooldown for %s, curTime = %s", spell.CD.TimeToReady(sim), sim.CurrentTime)
			}
			spell.CD.Set(sim.CurrentTime + spell.CurCast.CastTime + spell.CD.Duration)
		}

		if config.SharedCD.Timer != nil {
			// By panicking if spell is on CD, we force each sim to properly check for their own CDs.
			if !spell.SharedCD.IsReady(sim) {
				return spell.castFailureHelper(sim, "still on shared cooldown for %s, curTime = %s", spell.SharedCD.TimeToReady(sim), sim.CurrentTime)
			}
			spell.SharedCD.Set(sim.CurrentTime + spell.CurCast.CastTime + spell.SharedCD.Duration)
		}

		// By panicking if spell is on CD, we force each sim to properly check for their own CDs.
		if spell.CurCast.GCD != 0 && !spell.Unit.GCD.IsReady(sim) {
			return spell.castFailureHelper(sim, "GCD on cooldown for %s, curTime = %s", spell.Unit.GCD.TimeToReady(sim), sim.CurrentTime)
		}

		if hc := spell.Unit.Hardcast; hc.Expires > sim.CurrentTime {
			return spell.castFailureHelper(sim, "casting/channeling %v for %s, curTime = %s", hc.ActionID, hc.Expires-sim.CurrentTime, sim.CurrentTime)
		}

		resetsSwing := spell.timing.resetsSwing(spell)

		if effectiveTime := spell.CurCast.EffectiveTime(); effectiveTime != 0 {
			spell.SpellMetrics[target.UnitIndex].TotalCastTime += effectiveTime
			spell.Unit.SetGCDTimer(sim, sim.CurrentTime+effectiveTime)
		}

		// Hardcasts
		if spell.CurCast.CastTime > 0 {
			if resetsSwing {
				// no swings while casting
				spell.Unit.AutoAttacks.resetSwingTimers(sim, sim.CurrentTime+spell.CurCast.CastTime)
			}

			if sim.Log != nil && !spell.Flags.Matches(SpellFlagNoLogs) {
				spell.Unit.Log(sim, "Casting %s (Cost = %0.03f, Cast Time = %s, Effective Time = %s)",
					spell.ActionID, max(0, spell.CurCast.Cost), spell.CurCast.CastTime, spell.CurCast.EffectiveTime())
			}

			spell.Unit.Hardcast = Hardcast{
				Expires:  sim.CurrentTime + spell.CurCast.CastTime,
				ActionID: spell.ActionID,
				OnComplete: func(sim *Simulation, target *Unit) {
					if sim.Log != nil && !spell.Flags.Matches(SpellFlagNoLogs) {
						spell.Unit.Log(sim, "Completed cast %s", spell.ActionID)
					}

					if spell.Cost != nil {
						spell.Cost.SpendCost(sim, spell)
					}

					spell.applyEffects(sim, target)
					if resetsSwing {
						spell.Unit.AutoAttacks.resetSwingTimers(sim, sim.CurrentTime)
					}

					if !spell.Flags.Matches(SpellFlagNoOnCastComplete) {
						spell.Unit.OnCastComplete(sim, spell)
					}
				},
				Target: target,
			}

			if spell.Unit.Hardcast.Expires != spell.Unit.NextGCDAt() {
				spell.Unit.newHardcastAction(sim)
			}

			return true
		}

		if sim.Log != nil && !spell.Flags.Matches(SpellFlagNoLogs) {
			spell.Unit.Log(sim, "Casting %s (Cost = %0.03f, Cast Time = %s, Effective Time = %s)",
				spell.ActionID, max(0, spell.CurCast.Cost), spell.CurCast.CastTime, spell.CurCast.EffectiveTime())
			spell.Unit.Log(sim, "Completed cast %s", spell.ActionID)
		}

		if spell.Cost != nil {
			spell.Cost.SpendCost(sim, spell)
		}

		spell.applyEffects(sim, target)
		if resetsSwing {
			spell.Unit.AutoAttacks.resetSwingTimers(sim, sim.CurrentTime)
		}

		if !spell.Flags.Matches(SpellFlagNoOnCastComplete) {
			spell.Unit.OnCastComplete(sim, spell)
		}

		return true
	}
}

// makeCastFuncSimple and makeCastFuncAutosOrProcs never reset the swing timer: they cast procs and
// autos, which the server casts triggered. Off-GCD cooldowns with ResetsAutoAttack (Barkskin, Blood
// Tap) come through here too, so they need class code for the reset.
func (spell *Spell) makeCastFuncSimple() CastSuccessFunc {
	return func(sim *Simulation, target *Unit) bool {
		if spell.ExtraCastCondition != nil {
			if !spell.ExtraCastCondition(sim, target) {
				return spell.castFailureHelper(sim, "extra spell condition")
			}
		}

		if spell.CD.Timer != nil {
			// By panicking if spell is on CD, we force each sim to properly check for their own CDs.
			if !spell.CD.IsReady(sim) {
				return spell.castFailureHelper(sim, "still on cooldown for %s, curTime = %s", spell.CD.TimeToReady(sim), sim.CurrentTime)
			}

			spell.CD.Set(sim.CurrentTime + spell.CD.Duration)
		}

		if spell.SharedCD.Timer != nil {
			// By panicking if spell is on CD, we force each sim to properly check for their own CDs.
			if !spell.SharedCD.IsReady(sim) {
				return spell.castFailureHelper(sim, "still on shared cooldown for %s, curTime = %s", spell.SharedCD.TimeToReady(sim), sim.CurrentTime)
			}

			spell.SharedCD.Set(sim.CurrentTime + spell.SharedCD.Duration)
		}

		if sim.Log != nil && !spell.Flags.Matches(SpellFlagNoLogs) {
			spell.Unit.Log(sim, "Casting %s (Cost = %0.03f, Cast Time = %s, Effective Time = %s)",
				spell.ActionID, 0.0, "0s", "0s")
			spell.Unit.Log(sim, "Completed cast %s", spell.ActionID)
		}

		spell.applyEffects(sim, target)

		if !spell.Flags.Matches(SpellFlagNoOnCastComplete) {
			spell.Unit.OnCastComplete(sim, spell)
		}

		return true
	}
}

func (spell *Spell) makeCastFuncAutosOrProcs() CastSuccessFunc {
	return func(sim *Simulation, target *Unit) bool {
		if sim.Log != nil && !spell.Flags.Matches(SpellFlagNoLogs) {
			spell.Unit.Log(sim, "Casting %s (Cost = %0.03f, Cast Time = %s, Effective Time = %s)",
				spell.ActionID, 0.0, "0s", "0s")
			spell.Unit.Log(sim, "Completed cast %s", spell.ActionID)
		}

		spell.applyEffects(sim, target)

		if !spell.Flags.Matches(SpellFlagNoOnCastComplete) {
			spell.Unit.OnCastComplete(sim, spell)
		}

		return true
	}
}

func (spell *Spell) ApplyCostModifiers(cost float64) float64 {
	cost -= spell.Unit.PseudoStats.CostReduction
	cost = max(0, cost*spell.Unit.PseudoStats.CostMultiplier)
	return max(0, cost*spell.CostMultiplier)
}
