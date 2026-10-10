package mage

import (
	"github.com/wowsims/wotlk/sim/core"
	"time"
)

// spell_mage_dragon_breath keeps Living Bomb's explosion from taking the aura off
const livingBombExplosionID = 55362

func (mage *Mage) registerDragonsBreathSpell() {
	if !mage.Talents.DragonsBreath {
		return
	}

	// 42950's aura has one proc charge (spell_proc -31661): the first direct hit that damages the target takes
	// the whole aura off, snare included. Dot ticks and Living Bomb's explosion leave it.
	if mage.Talents.TormentTheWeak > 0 {
		mage.dragonsBreathSnares = mage.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
			if target.IsWorldBoss {
				return nil
			}
			return target.GetOrRegisterAura(core.Aura{
				Label:    "Dragon's Breath by " + mage.Label,
				Duration: time.Second * 5,
				OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
					if result.Damage > 0 && spell.SpellID != livingBombExplosionID {
						aura.Deactivate(sim)
					}
				},
			})
		})
	}

	mage.DragonsBreath = mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 42950},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | core.SpellFlagAPL,
		ManaCost: core.ManaCostOptions{
			BaseCost: 0.07,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: time.Second * 20,
			},
		},
		BonusCritRating:          float64(mage.Talents.CriticalMass+mage.Talents.WorldInFlames) * 2 * core.CritRatingPerCritChance,
		DamageMultiplierAdditive: 1 + .02*float64(mage.Talents.FirePower),
		CritMultiplier:           mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier:         1,

		Direct: core.SpellEffect{Effect: 0, Min: 1101, Max: 1279, SP: 0.193},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				baseDamage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP*spell.SpellPower())
				baseDamage *= sim.Encounter.AOECapMultiplier()
				if !spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit).Landed() || mage.dragonsBreathSnares == nil {
					continue
				}
				// the aura goes on after the hit, so its own damage doesn't take it off
				if snare := mage.dragonsBreathSnares.Get(aoeTarget); snare != nil {
					snare.Activate(sim)
				}
			}
		},
	})
}
