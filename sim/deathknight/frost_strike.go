package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

var frostStrikeActionID = core.ActionID{SpellID: 55268}
var FrostStrikeMHActionID = frostStrikeActionID.WithTag(1)
var FrostStrikeOHActionID = frostStrikeActionID.WithTag(2)

func (dk *Deathknight) newFrostStrikeHitSpell(isMH bool) *core.Spell {
	actionID := FrostStrikeMHActionID
	if !isMH {
		actionID = FrostStrikeOHActionID
	}

	conf := core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolFrost,
		ProcMask:    dk.threatOfThassarianProcMask(isMH),
		Flags:       core.SpellFlagMeleeMetrics,

		RuneCost: core.RuneCostOptions{
			RunicPowerCost: core.TernaryFloat64(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfFrostStrike), 32, 40),
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		BonusCritRating:  (dk.annihilationCritBonus() + dk.darkrunedBattlegearCritBonus()) * core.CritRatingPerCritChance,
		DamageMultiplier: dk.bloodOfTheNorthCoeff(),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.GuileOfGorefiend),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			var baseDamage float64
			if isMH {
				baseDamage = normalizedStrikeBase(sim, spell, true, spell.Direct.Roll(sim)) * spell.Direct.WeaponPct
			} else {
				// Spell::EffectWeaponDmg applies the hand's TOTAL_PCT to physical strikes only, so this Frost
				// one skips the off hand's half and Nerves of Cold Steel, on the weapon and the fixed bonus alike.
				baseDamage = (spell.Direct.Roll(sim) +
					spell.Unit.AutoAttacks.OH().CalculateNormalizedWeaponDamage(sim, spell.MeleeAttackPower()) +
					spell.BonusWeaponDamage()) * spell.Direct.WeaponPct
			}
			baseDamage *= dk.glacielRotBonus(target) *
				dk.RoRTSBonus(target) *
				dk.mercilessCombatBonus(sim)

			result := spell.CalcDamage(sim, target, baseDamage, dk.threatOfThassarianOutcomeApplier(spell))

			if isMH {
				spell.SpendRefundableCost(sim, result)
				dk.threatOfThassarianProc(sim, result, dk.FrostStrikeOhHit)
			}

			spell.DealDamage(sim, result)
		},
	}

	conf.Mods = []core.SpellMod{{Op: core.SpellModEffect1, Flat: dk.sigilOfTheVengefulHeartFrostStrike()}}
	if !isMH {
		conf.RuneCost = core.RuneCostOptions{}
		conf.Cast = core.CastConfig{}
		// Threat of Thassarian's off-hand strike
		conf.Direct = core.SpellEffect{Effect: 0, FromSpellID: 66962, Min: 125, Max: 125, WeaponPct: 0.55}
	} else {
		conf.Flags |= core.SpellFlagAPL
		conf.Direct = core.SpellEffect{Effect: 0, Min: 250, Max: 250, WeaponPct: 0.55}
	}

	return dk.RegisterSpell(conf)
}

func (dk *Deathknight) registerFrostStrikeSpell() {
	if !dk.Talents.FrostStrike {
		return
	}

	dk.FrostStrikeMhHit = dk.newFrostStrikeHitSpell(true)
	dk.FrostStrikeOhHit = dk.newFrostStrikeHitSpell(false)
	dk.FrostStrike = dk.FrostStrikeMhHit
}
