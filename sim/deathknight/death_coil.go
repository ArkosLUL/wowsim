package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

var DeathCoilActionID = core.ActionID{SpellID: 49895}

// DeathCoilDamageActionID is the spell spell_dk_death_coil casts at an enemy the cast hits, which
// deals the damage.
var DeathCoilDamageActionID = core.ActionID{SpellID: 47632}

func (dk *Deathknight) registerDeathCoilSpell() {
	// The dummy's 443 goes to the damage spell as its base points. Sigil of the Wild Buck's flat
	// modifier covers both spells, so ApplyEffectModifiers adds it to each; the Vengeful Heart's bonus
	// is spell_dk_death_coil's, once.
	bonusFlatDamage := 443 + 2*dk.sigilOfTheWildBuckBonus() + dk.sigilOfTheVengefulHeartDeathCoil()

	damage := dk.RegisterSpell(core.SpellConfig{
		ActionID:    DeathCoilDamageActionID,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,
		// only the cast counts as the player casting Death Coil
		Flags:        core.SpellFlagNoOnCastComplete,
		MissileSpeed: 24,

		BonusCritRating: dk.darkrunedBattlegearCritBonus() * core.CritRatingPerCritChance,
		DamageMultiplier: (1 + float64(dk.Talents.Morbidity)*0.05) +
			core.TernaryFloat64(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfDarkDeath), 0.15, 0.0),
		CritMultiplier:   dk.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1.0,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// a missile, worked out when it lands
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				baseDamage := (bonusFlatDamage + 0.15*dk.getImpurityBonus(spell)) * dk.RoRTSBonus(target)
				// SPELL_ATTR3_ALWAYS_HIT, and not binary, so it resists partially
				result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicCrit)
				if dk.Talents.UnholyBlight {
					dk.procUnholyBlight(sim, target, result.Damage)
				}
				spell.DealDamage(sim, result)
			})
		},
	})

	dk.DeathCoil = dk.RegisterSpell(core.SpellConfig{
		ActionID:    DeathCoilActionID,
		Flags:       core.SpellFlagAPL,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,

		RuneCost: core.RuneCostOptions{
			RunicPowerCost: 40,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		ThreatMultiplier: 1.0,

		// The cast is a binary dummy: a resist is a miss, and only a hit sends the damage spell.
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				damage.Cast(sim, target)
			}
		},
	})
}

// The rune weapon mirrors the damage spell itself (spell_dk_dancing_rune_weapon's Death Coil
// exception), so it gets 47632's own 600 base points and no relic bonus, and can't miss.
func (dk *Deathknight) registerDrwDeathCoilSpell() {
	dk.RuneWeapon.DeathCoil = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:     DeathCoilDamageActionID,
		SpellSchool:  core.SpellSchoolShadow,
		ProcMask:     core.ProcMaskSpellDamage,
		MissileSpeed: 24,

		BonusCritRating: dk.darkrunedBattlegearCritBonus() * core.CritRatingPerCritChance,
		DamageMultiplier: (1.0 + float64(dk.Talents.Morbidity)*0.05) *
			core.TernaryFloat64(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfDarkDeath), 1.15, 1.0),
		CritMultiplier:   dk.RuneWeapon.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1.0,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				baseDamage := 600 + 0.15*dk.RuneWeapon.getImpurityBonus(spell)
				spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicCrit)
			})
		},
	})
}
