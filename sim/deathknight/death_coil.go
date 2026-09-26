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
	vengefulHeart := dk.sigilOfTheVengefulHeartDeathCoil()

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

		// the dummy's 443 goes to the damage spell as its base points, under the damage spell's 0.15 AP
		Direct: core.SpellEffect{Effect: 0, FromSpellID: DeathCoilActionID.SpellID, Min: 443, Max: 443, AP: 0.15},
		Mods: []core.SpellMod{
			// Sigil of the Wild Buck covers both spells, so ApplyEffectModifiers adds it twice
			{Op: core.SpellModEffect1, Flat: dk.sigilOfTheWildBuckBonus()},
			{Op: core.SpellModEffect1, Flat: dk.sigilOfTheWildBuckBonus()},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Vengeful Heart's bonus comes from spell_dk_death_coil, once. Damage and crit are worked out at
			// launch and land with the missile.
			baseDamage := (spell.Direct.Roll(sim) + vengefulHeart + spell.Direct.AP*dk.getImpurityBonus(spell)) *
				dk.RoRTSBonus(target)
			// SPELL_ATTR3_ALWAYS_HIT, and not binary, so it resists partially
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
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
// exception), so it gets 47632's own 600 base points and can't miss. Its spell mods are its owner's, so
// Sigil of the Wild Buck counts once. The Vengeful Heart's bonus comes from the cast's script, so never.
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

		Direct: core.SpellEffect{Effect: 0, Min: 600, Max: 600, AP: 0.15},
		Mods:   []core.SpellMod{{Op: core.SpellModEffect1, Flat: dk.sigilOfTheWildBuckBonus()}},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*dk.RuneWeapon.getImpurityBonus(spell)
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicCrit)
			spell.DealDamageAfterTravel(sim, result)
		},
	})
}
