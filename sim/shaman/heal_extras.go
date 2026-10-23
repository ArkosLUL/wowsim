package shaman

import (
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// registerGlyphOfHealingWaveHeal registers 55533, which spell_sha_glyph_of_healing_wave casts on the shaman for
// 20% of each Healing Wave that lands on someone else.
func (shaman *Shaman) registerGlyphOfHealingWaveHeal() {
	shaman.glyphOfHealingWaveHeal = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55533},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		// SPELL_ATTR6_IGNORE_HEALTH_MODIFIERS: Unit::SpellPctHealingModsDone gives it no healing done percent
		Flags: core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete | core.SpellFlagIgnoreAttackerModifiers,
		// no spell_bonus_data row
		HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(0),

		DamageMultiplier: 1,
		CritMultiplier:   shaman.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.CalcAndDealHealing(sim, target, shaman.glyphHealAmount, spell.OutcomeHealingCrit)
		},
	})
}

// healFromGlyphOfHealingWave is the script's CalculatePct(int32(heal), 20) as base points. 55533's class mask is 0
// and its effect has no spell power coefficient, so no talent, spell mod or spell power adds to them.
func (shaman *Shaman) healFromGlyphOfHealingWave(sim *core.Simulation, heal float64) {
	amount := int32(float32(heal) * 20 / 100)
	if amount == 0 {
		return
	}
	shaman.glyphHealAmount = float64(amount)
	shaman.glyphOfHealingWaveHeal.Cast(sim, &shaman.Unit)
}

// registerTidalForce is the talent's cast, 55198, and the aura it puts on the shaman, 55166: three charges of
// 20% crit chance for Healing Wave, Lesser Healing Wave and Chain Heal. The charges go one at a time on the crits
// of those three heals (spell_proc for 55198; spell_sha_tidal_force_dummy removes the stack), for 20 s.
func (shaman *Shaman) registerTidalForce() {
	if !shaman.Talents.TidalForce {
		return
	}
	heals := []*core.Spell{shaman.HealingWave, shaman.LesserHealingWave, shaman.ChainHeal}

	aura := shaman.RegisterAura(core.Aura{
		Label:     "Tidal Force",
		ActionID:  core.ActionID{SpellID: 55166},
		Duration:  time.Second * 20,
		MaxStacks: 3,
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
			bonus := core.CritRatingPerCritChance * 20 * float64(newStacks-oldStacks)
			for _, heal := range heals {
				heal.BonusCritRating += bonus
			}
		},
		OnHealDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Outcome.Matches(core.OutcomeCrit) && slices.Contains(heals, spell) {
				aura.RemoveStack(sim)
			}
		},
	})

	shaman.TidalForce = shaman.RegisterSpell(core.SpellConfig{
		ActionID:         core.ActionID{SpellID: 55198},
		SpellSchool:      core.SpellSchoolPhysical,
		ProcMask:         core.ProcMaskEmpty,
		Flags:            core.SpellFlagNoOnCastComplete,
		ThreatMultiplier: 1,
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    shaman.NewTimer(),
				Duration: time.Minute * 3,
			},
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			spell.LandOn(sim, &shaman.Unit)
			aura.Activate(sim)
			aura.SetStacks(sim, aura.MaxStacks)
		},
	})
}

// registerChainedHeal registers 70809, the hot T10's 4 piece puts on the target of a Chain Heal crit.
func (shaman *Shaman) registerChainedHeal() {
	shaman.chainedHealDelay = core.NewDelayedPeriodicApplier(&shaman.Unit)
	shaman.chainedHeal = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 70809},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,
		// no spell_bonus_data row: 9 s of 15, over its 3 ticks
		HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(9.0 / 15 / 3),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		CritMultiplier:   1,

		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Chained Heal",
			},
			NumberOfTicks: 3,
			TickLength:    time.Second * 3,
			TicksCanCrit:  false,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				// each tick takes the healing done percents and the target's healing taken
				dot.SnapshotBaseDamage = shaman.chainedHealAmount
				dot.SnapshotHotMultiplier(1)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
			},
		},
	})
}

// chainHealHeals is spell_sha_t10_restoration_4p_bonus: a Chain Heal crit heals its target for 25% of the crit over
// the hot's 3 ticks, int32(int32(heal × 25%) / 3) a tick. Unit::CastDelayedSpellWithPeriodicAmount adds
// int32(old × ticks left / 3) of the shaman's hot already on the target, read at the crit, and lands the new one
// on the shaman's next 400 ms step, at once on the shaman.
func (shaman *Shaman) chainHealHeals(sim *core.Simulation, target *core.Unit, heal float64) {
	hot := shaman.chainedHeal.Hot(target)
	amount := int32(float32(heal)*25/100) / 3
	if hot.IsActive() {
		total := hot.TotalTicks()
		amount += int32(hot.SnapshotBaseDamage) * max(total-hot.TickCount, 0) / total
	}
	shaman.chainedHealDelay.Apply(sim, target, func(sim *core.Simulation) {
		shaman.chainedHealAmount = float64(amount)
		hot.Apply(sim)
	})
}
