package paladin

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (paladin *Paladin) registerHammerOfTheRighteousSpell() {
	numHits := min(core.TernaryInt32(paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfHammerOfTheRighteous), 4, 3), paladin.Env.GetNumTargets())
	results := make([]*core.SpellResult, numHits)

	paladin.HammerOfTheRighteous = paladin.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 53595},
		SpellSchool:  core.SpellSchoolHoly,
		ProcMask:     core.ProcMaskMeleeMHSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MissileSpeed: 35,

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
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return paladin.MainHand().HandType != proto.HandType_HandTypeTwoHand
		},

		DamageMultiplier: spellModDamage(paladin.getItemSetRedemptionPlateBonus2(), paladin.getItemSetT9PlateBonus2(),
			paladin.getItemSetLightswornPlateBonus2()),
		CritMultiplier:   paladin.MeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 1, Max: 1},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Spell::EffectSchoolDMG adds effect 2's 4 times the main hand's dps, in whole points: the
			// average hit (weapon and attack power, no percent mods) per unhasted attack time in ms
			mh := spell.Unit.AutoAttacks.MH()
			avgWeaponDamage := mh.CalculateAverageWeaponDamage(spell.MeleeAttackPower()) + spell.BonusWeaponDamage()
			attackTimeMs := math.Round(mh.SwingSpeed * 1000)
			baseDamage := spell.Direct.Roll(sim) + math.Floor(4*math.Floor(avgWeaponDamage*1000)/attackTimeMs)

			curTarget := target
			for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
				results[hitIndex] = spell.CalcDamage(sim, curTarget, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
				curTarget = sim.Environment.NextTargetUnit(curTarget)
			}

			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				for _, result := range results {
					spell.DealDamage(sim, result)
				}
			})
		},
	})
}
