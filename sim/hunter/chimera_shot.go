package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (hunter *Hunter) registerChimeraShotSpell() {
	if !hunter.Talents.ChimeraShot {
		return
	}

	ssProcSpell := hunter.chimeraShotSerpentStingSpell()

	hunter.ChimeraShot = hunter.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 53209},
		SpellSchool:  core.SpellSchoolNature,
		ProcMask:     core.ProcMaskRangedSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MissileSpeed: 40,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.12,
			Multiplier: 1 -
				0.03*float64(hunter.Talents.Efficiency) -
				0.05*float64(hunter.Talents.MasterMarksman),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
			CD: core.Cooldown{
				Timer:    hunter.NewTimer(),
				Duration: time.Second*10 - core.TernaryDuration(hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfChimeraShot), time.Second*1, 0),
			},
		},

		DamageMultiplier: 1 * hunter.markedForDeathMultiplier(),
		CritMultiplier:   hunter.critMultiplier(true, true),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 1, Min: 0, Max: 0, WeaponPct: 1.25},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// the weapon's AP part, normalized: RAP / 14 × 2.8 s
			baseDamage := (spell.Direct.Roll(sim) + 0.2*spell.RangedAttackPower(target) +
				hunter.AutoAttacks.Ranged().BaseDamage(sim) +
				hunter.NormalizedAmmoDamageBonus +
				spell.BonusWeaponDamage()) * spell.Direct.WeaponPct

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				if result.Landed() {
					if hunter.SerpentSting.Dot(target).IsActive() {
						hunter.SerpentSting.Dot(target).Rollover(sim)
						ssProcSpell.Cast(sim, target)
					} else if hunter.ScorpidStingAuras.Get(target).IsActive() {
						hunter.ScorpidStingAuras.Get(target).Refresh(sim)
					}
				}
				spell.DealDamage(sim, result)
			})
		},
	})
}

func (hunter *Hunter) chimeraShotSerpentStingSpell() *core.Spell {
	return hunter.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 53353},
		SpellSchool:  core.SpellSchoolNature,
		ProcMask:     core.ProcMaskRangedSpecial,
		Flags:        core.SpellFlagMeleeMetrics,
		MissileSpeed: 40,

		DamageMultiplierAdditive: 1 +
			0.1*float64(hunter.Talents.ImprovedStings) +
			core.TernaryFloat64(hunter.HasSetBonus(ItemSetScourgestalkerBattlegear, 2), .1, 0),
		DamageMultiplier: 1 *
			(2.0 + core.TernaryFloat64(hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfSerpentSting), 0.8, 0)) *
			hunter.markedForDeathMultiplier(),
		CritMultiplier:   hunter.critMultiplier(true, false),
		ThreatMultiplier: 1,

		// spell_hun_chimera_shot deals the sting's tick, times 40% of its tick count in DamageMultiplier
		Direct: core.SpellEffect{Effect: 0, FromSpellID: 49001, Min: 242, Max: 242, AP: 0.04},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*spell.RangedAttackPower(target)
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialCritOnly)
			spell.DealDamageAfterTravel(sim, result)
		},
	})
}
