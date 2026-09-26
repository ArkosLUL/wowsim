package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// The nova is its own spell (61654): the cast (61657) is a binary dummy on the server, the nova isn't.
// spell_sha_fire_nova has the player cast it, only aimed at the fire totem, so it's the player's own
// launch damage and takes the ten-target cap (Spell::DoAllEffectOnLaunchTarget).
func (shaman *Shaman) registerFireNovaAttackSpell() *core.Spell {
	return shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 61654},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,
		// Focusable belongs on whichever half deals the damage: spell_sha_elemental_focus only refuses
		// the two weapon imbue attacks, so a Fire Nova crit still hands out Clearcasting.
		Flags: SpellFlagFocusable | core.SpellFlagNoOnCastComplete,

		BonusHitRating:   float64(shaman.Talents.ElementalPrecision) * core.SpellHitRatingPerHitChance,
		DamageMultiplier: 1 + float64(shaman.Talents.CallOfFlame)*0.05 + float64(shaman.Talents.ImprovedFireNova)*0.1,
		CritMultiplier:   shaman.ElementalCritMultiplier(0),
		ThreatMultiplier: shaman.spellThreatMultiplier(),

		Direct: core.SpellEffect{Effect: 0, Min: 893, Max: 997, SP: 0.214},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			dmgFromSP := spell.Direct.SP * spell.SpellPower()
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				baseDamage := (spell.Direct.Roll(sim) + dmgFromSP) * sim.Encounter.AOECapMultiplier()
				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			}
		},
	})
}

func (shaman *Shaman) registerFireNovaSpell() {
	fireNovaGlyphCDReduction := core.TernaryInt32(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfFireNova), 3, 0)
	impFireNovaCDReduction := shaman.Talents.ImprovedFireNova * 2
	fireNovaCooldown := 10 - fireNovaGlyphCDReduction - impFireNovaCDReduction

	fireNovaAttack := shaman.registerFireNovaAttackSpell()

	shaman.FireNova = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 61657},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       SpellFlagFocusable | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.22,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    shaman.NewTimer(),
				Duration: time.Second * time.Duration(fireNovaCooldown),
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, _ *core.Spell) {
			fireNovaAttack.Cast(sim, target)
		},
	})
}

func (shaman *Shaman) IsFireNovaCastable(sim *core.Simulation) bool {
	return shaman.FireNova.IsReady(sim) && shaman.Totems.Fire > 0
}
