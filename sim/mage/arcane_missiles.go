package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (mage *Mage) registerArcaneMissilesSpell() {
	mage.ArcaneMissilesTickSpell = mage.GetOrRegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 42845},
		SpellSchool: core.SpellSchoolArcane,
		// unlike Mind Flay, this CAN proc JoW. It can also proc trinkets without the "can proc from proc" flag
		// such as illustration of the dragon soul
		// however, it cannot proc Nibelung so we add the ProcMaskNotInSpellbook flag
		ProcMask:        core.ProcMaskSpellDamage | core.ProcMaskNotInSpellbook,
		Flags:           SpellFlagMage | core.SpellFlagNoLogs,
		MissileSpeed:    20,
		BonusHitRating:  float64(mage.Talents.ArcaneFocus) * core.SpellHitRatingPerHitChance,
		BonusCritRating: core.TernaryFloat64(mage.HasSetBonus(ItemSetKhadgarsRegalia, 4), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplierAdditive: 1 +
			core.TernaryFloat64(mage.HasSetBonus(ItemSetTempestRegalia, 4), .05, 0),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage+core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfArcaneMissiles), .25, 0)),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 361, Max: 361, SP: 0.2857},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 3 * mage.Talents.ArcaneEmpowerment},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			damage := (spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()) * mage.tormentTheWeak(target)
			result := spell.CalcDamage(sim, target, damage, spell.OutcomeMagicHitAndCrit)

			spell.DealDamageAfterTravel(sim, result)
		},
	})

	mage.ArcaneMissiles = mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 42846},
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSpellDamage,
		// Arcane Focus covers the missiles (42845), not the channel cast itself
		Flags: SpellFlagMage | core.SpellFlagChanneled | core.SpellFlagAPL,
		ManaCost: core.ManaCostOptions{
			BaseCost: 0.31,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},
		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "ArcaneMissiles",
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					mage.missiles.channeling = true
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					mage.missiles.channeling = false
					// a used up Missile Barrage stays on until here, so the channel keeps its halved ticks
					if mage.missiles.barrageUsed {
						mage.missiles.barrageUsed = false
						mage.MissileBarrageAura.Deactivate(sim)
					}

					// TODO: This check is necessary to ensure the final tick occurs before
					// Arcane Blast stacks are dropped. To fix this, ticks need to reliably
					// occur before aura expirations.
					dot := mage.ArcaneMissiles.Dot(aura.Unit)
					if dot.TickCount < dot.NumberOfTicks {
						dot.TickCount++
						dot.TickOnce(sim)
					}
					mage.ArcaneBlastAura.Deactivate(sim)

					// spell_mage_arcane_missiles: T10's 2 pieces again when the channel ends, but not in the
					// iteration's cleanup, which runs at the duration with the mage's auras already off
					if mage.missiles.t10AtEnd && sim.CurrentTime < sim.Duration {
						mage.missiles.t10AtEnd = false
						mage.pushingTheLimit(sim)
					}
				},
			},
			NumberOfTicks:       5,
			TickLength:          time.Second,
			TicksCanCrit:        false,
			AffectedByCastSpeed: true,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				mage.ArcaneMissilesTickSpell.Cast(sim, target)
			},
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
				// spell_proc 44401 uses Missile Barrage up at the cast's finish phase, once the channel's aura is on
				if mage.MissileBarrageAura.IsActive() && !mage.t84pcKeepsProc(sim) {
					mage.missiles.barrageUsed = true
					mage.pushingTheLimit(sim)
					mage.missiles.t10AtEnd = mage.t10Aura != nil
				}
			}
			spell.DealOutcome(sim, result)
		},
	})
}
