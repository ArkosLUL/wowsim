package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warlock *Warlock) registerHauntSpell() {
	if !warlock.Talents.Haunt {
		return
	}

	actionID := core.ActionID{SpellID: 59164}
	debuffMult := core.TernaryFloat64(warlock.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfHaunt), 1.23, 1.2)

	warlock.HauntDebuffAuras = warlock.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return target.GetOrRegisterAura(core.Aura{
			Label:    "Haunt-" + warlock.Label,
			ActionID: actionID,
			Duration: time.Second * 12,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				warlock.AttackTables[aura.Unit.UnitIndex].HauntSEDamageTakenMultiplier *= debuffMult
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				warlock.AttackTables[aura.Unit.UnitIndex].HauntSEDamageTakenMultiplier /= debuffMult
			},
		})
	})

	warlock.Haunt = warlock.RegisterSpell(core.SpellConfig{
		ActionID:     actionID,
		SpellSchool:  core.SpellSchoolShadow,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        core.SpellFlagAPL,
		MissileSpeed: 20,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.12,
			Multiplier: 1 - 0.02*float64(warlock.Talents.Suppression),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 1500,
			},
			CD: core.Cooldown{
				Timer:    warlock.NewTimer(),
				Duration: time.Second * 8,
			},
		},

		DamageMultiplier: spellModDamage(
			warlock.GrandFirestoneBonus(),
			0.03*float64(warlock.Talents.ShadowMastery),
		),
		CritMultiplier:   warlock.SpellCritMultiplier(1, core.TernaryFloat64(warlock.Talents.Pandemic, 1, 0)),
		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),

		Direct: core.SpellEffect{Effect: 0, Min: 645, Max: 753, SP: 0.4286},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
				if result.Landed() {
					warlock.HauntDebuffAuras.Get(result.Target).Activate(sim)
					warlock.everlastingAfflictionRefresh(sim, target)
				}
			})
		},
	})
}
