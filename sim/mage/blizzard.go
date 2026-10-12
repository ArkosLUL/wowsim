package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (mage *Mage) registerBlizzardSpell() {
	var improvedBlizzardProcApplication *core.Spell
	if mage.Talents.ImprovedBlizzard > 0 {
		auras := mage.NewEnemyAuraArray(func(unit *core.Unit) *core.Aura {
			return unit.GetOrRegisterAura(core.Aura{
				ActionID: core.ActionID{SpellID: 12488},
				Label:    "Improved Blizzard",
				Duration: time.Millisecond * 1500,
			})
		})
		improvedBlizzardProcApplication = mage.RegisterSpell(core.SpellConfig{
			ActionID: core.ActionID{SpellID: 12488},
			ProcMask: core.ProcMaskProc,
			Flags:    SpellFlagMage | core.SpellFlagNoLogs,
			ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
				auras.Get(target).Activate(sim)
			},
		})
	}

	blizzardTickSpell := mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 42938},
		SpellSchool: core.SpellSchoolFrost,
		// the channel's aura casts each tick, so only procs with SPELL_ATTR3_CAN_PROC_FROM_PROCS take it
		// (Dying Curse and Greatness, not Illustration of the Dragon Soul)
		ProcMask:         core.ProcMaskProc,
		Flags:            SpellFlagMage,
		BonusCritRating:  float64(mage.Talents.WorldInFlames) * 2 * core.CritRatingPerCritChance,
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage+float64(mage.Talents.IceShards)/3),
		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 426, Max: 426, SP: 0.1429},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			damage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			spell.AoECapUnits = len(sim.Encounter.Targets)
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				result := spell.CalcAndDealDamage(sim, aoeTarget, damage, spell.OutcomeMagicHitAndCrit)

				// spell_mage_imp_blizzard, on each landed tick: Chilled, and its own Fingers of Frost roll
				if improvedBlizzardProcApplication != nil && result.Landed() {
					improvedBlizzardProcApplication.Cast(sim, aoeTarget)
					mage.chill(sim, aoeTarget, time.Millisecond*1500)
					if mage.procFingersOfFrost != nil {
						mage.procFingersOfFrost(sim)
					}
				}
			}
		},
	})

	mage.Blizzard = mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 42940},
		SpellSchool: core.SpellSchoolFrost,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | core.SpellFlagChanneled | core.SpellFlagAPL,
		ManaCost: core.ManaCostOptions{
			BaseCost: 0.74,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},
		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "Blizzard",
			},
			NumberOfTicks:       8,
			TickLength:          time.Second * 1,
			TicksCanCrit:        false,
			AffectedByCastSpeed: true,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				blizzardTickSpell.Cast(sim, target)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.AOEDot().Apply(sim)
		},
	})
}
