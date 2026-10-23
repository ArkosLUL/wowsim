package paladin

import (
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// sacredShieldState is the unit wearing the paladin's Sacred Shield and the hits standing in for its damage taken.
type sacredShieldState struct {
	target  *core.Unit
	standIn *core.HitTakenStandIn
}

// sacredShieldEffectMultiplier is Divine Guardian's SPELLMOD_EFFECT1, 10% a point on effect 0 of 53601 and 58597.
func (paladin *Paladin) sacredShieldEffectMultiplier() float32 {
	return 1 + float32(10*paladin.Talents.DivineGuardian)/100
}

// sacredShieldDuration is a duration with Divine Guardian's SPELLMOD_DURATION, 50% a point on 53601 and 58597.
func (paladin *Paladin) sacredShieldDuration(base time.Duration) time.Duration {
	return base * time.Duration(100+50*paladin.Talents.DivineGuardian) / 100
}

// SacredShieldCooldown is spell_pal_sacred_shield_dummy's cooldown: 53601's effect 0 in seconds, which Divine
// Guardian's effect mod reaches too, whole.
func (paladin *Paladin) SacredShieldCooldown() time.Duration {
	return time.Duration(int32(float32(6)*paladin.sacredShieldEffectMultiplier())) * time.Second
}

// sacredShieldAbsorb is 58597's amount: its 500 with the effect mods, plus spell_pal_sacred_shield's 75% of the
// healing power with the effect mods too, and Strength of Wrynn's 30% on the sum. No healing done percent.
func (paladin *Paladin) sacredShieldAbsorb(spell *core.Spell, target *core.Unit) float64 {
	effectMul := paladin.sacredShieldEffectMultiplier()
	amount := int32(float32(500) * effectMul)
	// CalculatePct on the int32 healing bonus: whole before the effect mods
	bonus := float32(int32(float32(spell.HealingPower(target)) * 75 / 100))
	amount += int32(bonus * effectMul)
	if paladin.raidBuffs != nil && paladin.raidBuffs.StrengthOfWrynn {
		amount += int32(float32(amount) * 30 / 100)
	}
	return float64(amount)
}

// sacredShieldFlashCrit is 58597's +50% crit chance for Flash of Light, which Unit::SpellTakenCritChance looks up
// cast by the healed unit itself: only the paladin's heal on himself under his own shield gets it.
func (paladin *Paladin) sacredShieldFlashCrit(target *core.Unit) float64 {
	if paladin.SacredShieldAbsorb == nil || target != &paladin.Unit || !paladin.SacredShieldAbsorb.Shield(target).IsActive() {
		return 0
	}
	return 50 * core.CritRatingPerCritChance
}

// registerSacredShieldSpell is 53601, one target at a time (SPELL_ATTR5_LIMIT_N). Nothing damages the sim's raid,
// so its target counts as hit all the time: 58597's absorb goes on with the aura, then once a cooldown, a fresh
// aura starting the cooldown over and a refresh keeping it.
func (paladin *Paladin) registerSacredShieldSpell() {
	actionID := core.ActionID{SpellID: 53601}
	cooldown := paladin.SacredShieldCooldown()
	state := &paladin.sacredShield

	paladin.SacredShieldAbsorb = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 58597},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Shield: core.ShieldConfig{
			Aura: core.Aura{
				Label:    "Sacred Shield Absorb",
				Duration: paladin.sacredShieldDuration(time.Second * 6),
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Shield(target).Apply(sim, paladin.sacredShieldAbsorb(spell, target))
			spell.LandOn(sim, target)
		},
	})

	hit := func(sim *core.Simulation) {
		if state.target != nil {
			paladin.SacredShieldAbsorb.Cast(sim, state.target)
		}
	}
	state.standIn = core.NewHitTakenStandIn(float64(time.Minute)/float64(cooldown), hit)

	paladin.SacredShieldAuras = paladin.NewAllyAuraArray(func(unit *core.Unit) *core.Aura {
		return unit.RegisterAura(core.Aura{
			Label:    "Sacred Shield-" + strconv.Itoa(int(paladin.UnitIndex)),
			Tag:      sacredShieldTag,
			ActionID: actionID,
			Duration: paladin.sacredShieldDuration(time.Second * 30),
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				state.target = aura.Unit
				hit(sim)
				state.standIn.Start(sim)
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				if state.target == aura.Unit {
					state.standIn.Stop(sim)
					state.target = nil
				}
			},
		})
	})

	paladin.SacredShield = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.12,
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if old := state.target; old != nil && old != target {
				paladin.SacredShieldAuras.Get(old).Deactivate(sim)
			}
			paladin.SacredShieldAuras.Get(target).Activate(sim)
			spell.LandOn(sim, target)
		},
	})
}
