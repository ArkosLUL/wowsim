package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (shaman *Shaman) registerThunderstormSpell() {
	if !shaman.Talents.Thunderstorm {
		return
	}

	actionID := core.ActionID{SpellID: 59159}
	manaMetrics := shaman.NewManaMetrics(actionID)

	manaRestore := 0.08
	if shaman.HasMinorGlyph(proto.ShamanMinorGlyph_GlyphOfThunderstorm) {
		manaRestore = 0.1
	}

	shaman.Thunderstorm = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		Flags:       core.SpellFlagAPL,
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellDamage,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    shaman.NewTimer(),
				Duration: time.Second * 45,
			},
		},

		BonusHitRating:   float64(shaman.Talents.ElementalPrecision) * core.SpellHitRatingPerHitChance,
		BonusCritRating:  core.TernaryFloat64(shaman.Talents.CallOfThunder, 5*core.CritRatingPerCritChance, 0),
		DamageMultiplier: 1 + 0.01*float64(shaman.Talents.Concussion),
		CritMultiplier:   shaman.ElementalCritMultiplier(0),
		ThreatMultiplier: shaman.spellThreatMultiplier(),

		Direct: core.SpellEffect{Effect: 0, Min: 1450, Max: 1656, SP: 0.172},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			shaman.AddMana(sim, shaman.MaxMana()*manaRestore, manaMetrics)

			if shaman.thunderstormInRange {
				dmgFromSP := spell.Direct.SP * spell.SpellPower()
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					baseDamage := spell.Direct.Roll(sim) + dmgFromSP
					baseDamage *= sim.Encounter.AOECapMultiplier()
					spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
				}
			}
		},
	})
}
