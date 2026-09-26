package paladin

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (paladin *Paladin) registerShieldOfRighteousnessSpell() {
	var aegisPlateProcAura *core.Aura
	if paladin.HasSetBonus(ItemSetAegisPlate, 4) {
		aegisPlateProcAura = paladin.NewTemporaryStatsAura("Aegis", core.ActionID{SpellID: 64883}, stats.Stats{stats.BlockValue: 225}, time.Second*6)
	}

	paladin.ShieldOfRighteousness = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 61411},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.06,
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    paladin.NewTimer(),
				Duration: time.Second * 6,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   paladin.MeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 520, Max: 520},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Spell::EffectSchoolDMG adds effect 1's 100% of the block value, halved past 29.5 a level and
			// capped at 34.5 a level, plus a flat 225 with the T8 4pc, in whole points
			soft, hard := uint32(core.CharacterLevel*29.5), uint32(core.CharacterLevel*34.5)
			block := uint32(max(paladin.BlockValue(), 0))
			if block >= hard {
				block = (soft + hard) / 2
			} else if block > soft {
				block = soft + (block-soft)/2
			}
			if aegisPlateProcAura != nil {
				block += 225
			}

			baseDamage := spell.Direct.Roll(sim) + float64(block)
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			// the 4pc's block value proc (64883) comes from the hit, after its damage
			if aegisPlateProcAura != nil && result.Landed() {
				aegisPlateProcAura.Activate(sim)
			}
		},
	})
}
