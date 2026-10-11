// Functions for creating common types of auras.
package core

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type AuraCallback uint16

func (c AuraCallback) Matches(other AuraCallback) bool {
	return (c & other) != 0
}

const (
	CallbackEmpty AuraCallback = 0

	CallbackOnSpellHitDealt AuraCallback = 1 << iota
	CallbackOnSpellHitTaken
	CallbackOnPeriodicDamageDealt
	CallbackOnHealDealt
	CallbackOnPeriodicHealDealt
	CallbackOnCastComplete
)

type ProcHandler func(sim *Simulation, spell *Spell, result *SpellResult)

type ProcTrigger struct {
	Name            string
	ActionID        ActionID
	Duration        time.Duration
	Callback        AuraCallback
	ProcMask        ProcMask
	ProcMaskExclude ProcMask
	SpellFlags      SpellFlag
	Outcome         HitOutcome
	Harmful         bool
	ProcChance      float64
	PPM             float64
	ICD             time.Duration
	Handler         ProcHandler

	// ProcSpellID is the item, enchant or talent aura that procs. When set, its generated spell_proc
	// entry gives ProcChance, PPM and ICD instead of the three fields above.
	ProcSpellID int32

	// AnyPeriodic lets every periodic tick past ProcMask, which then only filters direct hits and heals: the
	// server's DONE_PERIODIC proc flag takes any dot or hot tick, whatever its spell's damage class. Ticks the
	// server deals as casts of another spell (ticksAsCasts) stay out unless CanProcFromProcs.
	AnyPeriodic bool

	// CanProcFromProcs is the proc aura's SPELL_ATTR3_CAN_PROC_FROM_PROCS: triggered spells proc it too. With
	// AnyPeriodic, a ticksAsCasts tick then counts as a direct hit of its spell's damage class.
	CanProcFromProcs bool
}

const (
	auraTypePeriodicTriggerSpell          = 23
	auraTypePeriodicDummy                 = 226
	auraTypePeriodicTriggerSpellWithValue = 227
)

// ticksAsCasts is whether the server deals this sim dot's ticks as triggered casts of another spell, from a
// periodic trigger or dummy aura (Death and Decay, Blizzard, Volley, Explosive Shot). Those raise no
// DONE_PERIODIC, only the triggered spell's own hit, which an aura without SPELL_ATTR3_CAN_PROC_FROM_PROCS
// ignores (none of these tick spells has SPELL_ATTR3_NOT_A_PROC).
func (spell *Spell) ticksAsCasts() bool {
	return spell.derived().ticksAsCasts
}

func ticksAsCastsOn(s *serverdata.Spell) bool {
	if s == nil {
		return false
	}
	triggers := false
	for _, e := range s.Effects {
		switch e.Aura {
		case auraTypePeriodicDamage, auraTypePeriodicDamagePercent, auraTypePeriodicHeal, auraTypePeriodicLeech:
			return false
		case auraTypePeriodicTriggerSpell, auraTypePeriodicDummy, auraTypePeriodicTriggerSpellWithValue:
			triggers = true
		}
	}
	return triggers
}

// tickCastMask is the proc mask of the hit a ticksAsCasts tick is on the server, by its damage class.
func (spell *Spell) tickCastMask() ProcMask {
	switch spell.ServerSpell().DmgClass {
	case serverdata.DmgClassMagic:
		return ProcMaskSpellDamage
	case serverdata.DmgClassMelee:
		return ProcMaskMeleeSpecial
	case serverdata.DmgClassRanged:
		return ProcMaskRangedSpecial
	}
	return ProcMaskEmpty
}

func ApplyProcTriggerCallback(unit *Unit, aura *Aura, config ProcTrigger) {
	if config.ProcSpellID != 0 {
		sp := ServerProcFor(config.ProcSpellID)
		config.ProcChance, config.PPM, config.ICD = sp.Chance, sp.PPM, sp.ICD
		if sp.PPM > 0 {
			// Aura::CalcProcChance takes PPM over the entry's own chance on a damage or heal event.
			config.ProcChance = 0
		}
	}

	var icd Cooldown
	if config.ICD != 0 {
		icd = Cooldown{
			Timer:    unit.NewTimer(),
			Duration: config.ICD,
		}
		aura.Icd = &icd
	}

	var ppmm PPMManager
	if config.PPM > 0 {
		ppmm = unit.AutoAttacks.NewPPMManager(config.PPM, config.ProcMask)
	}

	handler := config.Handler
	callbackFor := func(anyPeriodic bool) func(aura *Aura, sim *Simulation, spell *Spell, result *SpellResult) {
		return func(aura *Aura, sim *Simulation, spell *Spell, result *SpellResult) {
			if config.SpellFlags != SpellFlagNone && !spell.Flags.Matches(config.SpellFlags) {
				return
			}
			if config.ProcMaskExclude != ProcMaskUnknown && spell.ProcMask.Matches(config.ProcMaskExclude) {
				return
			}
			if anyPeriodic {
				if spell.ticksAsCasts() && !(config.CanProcFromProcs &&
					(config.ProcMask == ProcMaskUnknown || spell.tickCastMask().Matches(config.ProcMask))) {
					return
				}
			} else if config.ProcMask != ProcMaskUnknown && !spell.ProcMask.Matches(config.ProcMask) {
				return
			}
			if config.Outcome != OutcomeEmpty && !result.Outcome.Matches(config.Outcome) {
				return
			}
			if config.Harmful && result.Damage == 0 {
				return
			}
			if icd.Duration != 0 && !icd.IsReady(sim) {
				return
			}
			if config.ProcChance != 1 && sim.UnitRandomFloat(unit, config.Name) > config.ProcChance {
				return
			} else if config.PPM != 0 && !ppmm.Proc(sim, spell.ProcMask, config.Name) {
				return
			}

			if icd.Duration != 0 {
				icd.Use(sim)
			}
			handler(sim, spell, result)
		}
	}
	callback := callbackFor(false)
	periodicCallback := callback
	if config.AnyPeriodic {
		periodicCallback = callbackFor(true)
	}

	if config.ProcChance == 0 {
		config.ProcChance = 1
	}

	if config.Callback.Matches(CallbackOnSpellHitDealt) {
		aura.OnSpellHitDealt = callback
	}
	if config.Callback.Matches(CallbackOnSpellHitTaken) {
		aura.OnSpellHitTaken = callback
	}
	if config.Callback.Matches(CallbackOnPeriodicDamageDealt) {
		aura.OnPeriodicDamageDealt = periodicCallback
	}
	if config.Callback.Matches(CallbackOnHealDealt) {
		aura.OnHealDealt = callback
	}
	if config.Callback.Matches(CallbackOnPeriodicHealDealt) {
		aura.OnPeriodicHealDealt = periodicCallback
	}
	if config.Callback.Matches(CallbackOnCastComplete) {
		aura.OnCastComplete = func(aura *Aura, sim *Simulation, spell *Spell) {
			if config.SpellFlags != SpellFlagNone && !spell.Flags.Matches(config.SpellFlags) {
				return
			}
			if config.ProcMask != ProcMaskUnknown && !spell.ProcMask.Matches(config.ProcMask) {
				return
			}
			if config.ProcMaskExclude != ProcMaskUnknown && spell.ProcMask.Matches(config.ProcMaskExclude) {
				return
			}
			if icd.Duration != 0 && !icd.IsReady(sim) {
				return
			}
			if config.ProcChance != 1 && sim.UnitRandomFloat(unit, config.Name) > config.ProcChance {
				return
			}

			if icd.Duration != 0 {
				icd.Use(sim)
			}
			handler(sim, spell, nil)
		}
	}
}

func MakeProcTriggerAura(unit *Unit, config ProcTrigger) *Aura {
	aura := Aura{
		Label:           config.Name,
		ActionIDForProc: config.ActionID,
		Duration:        config.Duration,
	}
	if config.Duration == 0 {
		aura.Duration = NeverExpires
		aura.OnReset = func(aura *Aura, sim *Simulation) {
			aura.Activate(sim)
		}
	}

	ApplyProcTriggerCallback(unit, &aura, config)

	return unit.GetOrRegisterAura(aura)
}

type StackingStatAura struct {
	Aura          Aura
	BonusPerStack stats.Stats
}

func MakeStackingAura(character *Character, config StackingStatAura) *Aura {
	bonusPerStack := config.BonusPerStack
	config.Aura.OnStacksChange = func(aura *Aura, sim *Simulation, oldStacks int32, newStacks int32) {
		character.AddStatsDynamic(sim, bonusPerStack.Multiply(float64(newStacks-oldStacks)))
	}
	return character.RegisterAura(config.Aura)
}

// Returns the same Aura for chaining.
func MakePermanent(aura *Aura) *Aura {
	aura.Duration = NeverExpires
	if aura.OnReset == nil {
		aura.OnReset = func(aura *Aura, sim *Simulation) {
			aura.Activate(sim)
		}
	} else {
		oldOnReset := aura.OnReset
		aura.OnReset = func(aura *Aura, sim *Simulation) {
			oldOnReset(aura, sim)
			aura.Activate(sim)
		}
	}
	return aura
}

// Helper for the common case of making an aura that adds stats.
func (character *Character) NewTemporaryStatsAura(auraLabel string, actionID ActionID, tempStats stats.Stats, duration time.Duration) *Aura {
	return character.NewTemporaryStatsAuraWrapped(auraLabel, actionID, tempStats, duration, nil)
}

// Alternative that allows modifying the Aura config.
func (character *Character) NewTemporaryStatsAuraWrapped(auraLabel string, actionID ActionID, buffs stats.Stats, duration time.Duration, modConfig func(*Aura)) *Aura {
	// If one of the stat bonuses is a health bonus, then set up healing metrics for the associated
	// heal, since all temporary max health bonuses also instantaneously heal the player.
	var healthMetrics *ResourceMetrics
	var amountHealed float64
	includesHealthBuff := false

	for statIdx, increment := range buffs {
		if stats.Stat(statIdx) == stats.Health && increment > 0 {
			includesHealthBuff = true
			amountHealed = increment
			healthMetrics = character.NewHealthMetrics(actionID)
		}
	}

	config := Aura{
		Label:    auraLabel,
		ActionID: actionID,
		Duration: duration,
		OnGain: func(aura *Aura, sim *Simulation) {
			if sim.Log != nil {
				character.Log(sim, "Gained %s from %s.", buffs.FlatString(), actionID)
			}
			character.AddStatsDynamic(sim, buffs)

			if includesHealthBuff {
				character.GainHealth(sim, amountHealed, healthMetrics)
			}
		},
		OnExpire: func(aura *Aura, sim *Simulation) {
			if sim.Log != nil {
				character.Log(sim, "Lost %s from fading %s.", buffs.FlatString(), actionID)
			}
			character.AddStatsDynamic(sim, buffs.Invert())
		},
	}

	if modConfig != nil {
		modConfig(&config)
	}

	return character.GetOrRegisterAura(config)
}

func ApplyFixedUptimeAura(aura *Aura, uptime float64, tickLength time.Duration, startTime time.Duration) {
	auraDuration := aura.Duration
	ticksPerAura := float64(auraDuration) / float64(tickLength)
	chancePerTick := TernaryFloat64(uptime == 1, 1, 1.0-math.Pow(1-uptime, 1/ticksPerAura))

	// counted from now, like an aura the unit's own update puts on: a tick one duration later still
	// finds it on, so a full uptime has no gaps
	activate := func(sim *Simulation) {
		outer := sim.updating
		sim.updating = aura.countdownUnit()
		aura.Activate(sim)
		sim.updating = outer
	}

	aura.Unit.RegisterResetEffect(func(sim *Simulation) {
		StartPeriodicAction(sim, PeriodicActionOptions{
			Period: tickLength,
			OnAction: func(sim *Simulation) {
				if sim.UnitRandomFloat(aura.Unit, "FixedAura") < chancePerTick {
					activate(sim)
				}
			},
		})

		// Also try once at the start.
		StartPeriodicAction(sim, PeriodicActionOptions{
			Period:   startTime,
			NumTicks: 1,
			OnAction: func(sim *Simulation) {
				if sim.UnitRandomFloat(aura.Unit, "FixedAura") < uptime {
					// Use random duration to compensate for increased chance collapsed into single tick.
					randomDur := tickLength + time.Duration(float64(auraDuration-tickLength)*sim.UnitRandomFloat(aura.Unit, "FixedAuraDur"))

					aura.Duration = randomDur
					activate(sim)
					aura.Duration = auraDuration
				}
			},
		})
	})
}
