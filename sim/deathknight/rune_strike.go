package deathknight

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

var RuneStrikeActionID = core.ActionID{SpellID: 56815}

// Spell::EffectWeaponDmg adds Rune Strike's 15% of attack power after the weapon percent, scaled by the
// hand's TOTAL_PCT like a fixed bonus.
var runeStrikeEffect = core.SpellEffect{Effect: 0, WeaponPct: 1.5, AP: 0.15}

func (dk *Deathknight) threatOfThassarianRuneStrikeProcMask(isMH bool) core.ProcMask {
	if isMH {
		return core.ProcMaskMeleeMHSpecial | core.ProcMaskMeleeMHAuto
	} else {
		return core.ProcMaskMeleeOHSpecial | core.ProcMaskMeleeOHAuto
	}
}

func (dk *Deathknight) newRuneStrikeSpell(isMH bool) *core.Spell {
	runeStrikeGlyphCritBonus := core.TernaryFloat64(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfRuneStrike), 10.0, 0.0)

	conf := core.SpellConfig{
		ActionID:    RuneStrikeActionID.WithTag(core.TernaryInt32(isMH, 1, 2)),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    dk.threatOfThassarianRuneStrikeProcMask(isMH),
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		RuneCost: core.RuneCostOptions{
			RunicPowerCost: 20,
		},
		Cast: core.CastConfig{
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return dk.RuneStrikeAura.IsActive()
		},

		BonusCritRating: (dk.annihilationCritBonus() + runeStrikeGlyphCritBonus) * core.CritRatingPerCritChance,
		DamageMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()) *
			dk.darkrunedPlateRuneStrikeDamageBonus(),
		CritMultiplier:   dk.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1.75,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			var baseDamage = 0.0
			var outcomeApplier core.OutcomeApplier

			if isMH {
				baseDamage = (spell.Direct.Roll(sim)+
					spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower())+
					spell.BonusWeaponDamage())*spell.Direct.WeaponPct +
					spell.Direct.AP*spell.MeleeAttackPower()

				outcomeApplier = spell.OutcomeMeleeSpecialNoBlockDodgeParry
			} else {
				baseDamage = (spell.Direct.Roll(sim)+
					spell.Unit.OHWeaponDamage(sim, spell.MeleeAttackPower())+
					spell.BonusWeaponDamage())*spell.Direct.WeaponPct +
					offHandFixedPct*spell.Direct.AP*spell.MeleeAttackPower()

				outcomeApplier = spell.OutcomeMeleeSpecialCritOnly
			}

			baseDamage *= dk.RoRTSBonus(target)
			result := spell.CalcAndDealDamage(sim, target, baseDamage, outcomeApplier)

			if result.Damage > 0 && dk.Talents.Necrosis > 0 {
				dk.necrosisDamage(result.Damage, sim, target)
			}

			if isMH {
				dk.threatOfThassarianProc(sim, result, dk.RuneStrikeOh)
				dk.RuneStrikeAura.Deactivate(sim)
				dk.RuneStrikeQueued = false
			}
		},
	}
	conf.Direct = runeStrikeEffect
	if !isMH { // only MH has cost & gcd
		conf.RuneCost = core.RuneCostOptions{}
		conf.Cast = core.CastConfig{}
		conf.ExtraCastCondition = nil
		// Threat of Thassarian's off-hand strike
		conf.Direct.FromSpellID = 66217
	}

	return dk.RegisterSpell(conf)
}

func (dk *Deathknight) registerRuneStrikeSpell() {
	dk.RuneStrikeQueue = dk.RegisterSpell(core.SpellConfig{
		ActionID: RuneStrikeActionID.WithTag(0),
		Flags:    core.SpellFlagAPL,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			dk.RuneStrikeQueued = true
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return dk.RuneStrikeAura.IsActive() && !dk.RuneStrikeQueued
		},
	})
	dk.RuneStrike = dk.newRuneStrikeSpell(true)
	dk.RuneStrikeOh = dk.newRuneStrikeSpell(false)

	dk.RuneStrikeAura = dk.RegisterAura(core.Aura{
		Label:    "Rune Strike",
		ActionID: RuneStrikeActionID,
		Duration: 6 * time.Second,
	})

	core.MakePermanent(dk.GetOrRegisterAura(core.Aura{
		Label:    "Rune Strike Trigger",
		Duration: core.NeverExpires,
		OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Outcome.Matches(core.OutcomeDodge | core.OutcomeParry) {
				dk.RuneStrikeAura.Activate(sim)
			}
		},
	}))
}

func (dk *Deathknight) registerDrwRuneStrikeSpell() {
	runeStrikeGlyphCritBonus := core.TernaryFloat64(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfRuneStrike), 10.0, 0.0)

	dk.RuneWeapon.RuneStrike = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    RuneStrikeActionID.WithTag(1),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		BonusCritRating:  (dk.annihilationCritBonus() + runeStrikeGlyphCritBonus) * core.CritRatingPerCritChance,
		DamageMultiplier: dk.darkrunedPlateRuneStrikeDamageBonus(),
		CritMultiplier:   dk.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1.75,

		Direct: runeStrikeEffect,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim)+dk.DrwWeaponDamage(sim, spell))*spell.Direct.WeaponPct +
				spell.Direct.AP*spell.MeleeAttackPower()
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialNoBlockDodgeParry)
		},
	})
}
