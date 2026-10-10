package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (warlock *Warlock) registerInfernoSpell() {
	summonInfernalAura := warlock.RegisterAura(core.Aura{
		Label:    "Summon Infernal",
		ActionID: core.ActionID{SpellID: 1122},
		Duration: time.Second * 60,
	})

	warlock.Inferno = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 1122},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				CastTime: time.Millisecond * 1500,
				GCD:      core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    warlock.NewTimer(),
				Duration: time.Second * time.Duration(600),
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		CritMultiplier:   warlock.SpellCritMultiplier(1, 0),

		Direct: core.SpellEffect{Effect: 0, FromSpellID: 22703, Min: 200, Max: 200, SP: 1},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// TODO: add fire spell damage
			baseDmg := (spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP*spell.SpellPower())) * sim.Encounter.AOECapMultiplier()

			for _, aoeTarget := range sim.Encounter.TargetUnits {
				spell.CalcAndDealDamage(sim, aoeTarget, baseDmg, spell.OutcomeMagicHitAndCrit)
			}

			if warlock.Pet != nil {
				warlock.Pet.Disable(sim)
			}
			warlock.Infernal.EnableWithTimeout(sim, warlock.Infernal, time.Second*60)

			// fake aura to show duration
			summonInfernalAura.Activate(sim)
		},
	})
}

type InfernalPet struct {
	*core.Pet
	owner          *Warlock
	immolationAura *core.Spell
}

func (warlock *Warlock) NewInfernal() *InfernalPet {
	// spell_warl_infernal_scaling. 36186's immunities keep the later scaling auras' intellect and
	// resistances off too: unlike the other guardians' scaling, its auras aren't passive.
	statInheritance := func(ownerStats stats.Stats) stats.Stats {
		// TODO: account for fire spell damage
		return stats.Stats{
			stats.Stamina:          core.PetScalingPct(ownerStats[stats.Stamina], 75),
			stats.Armor:            core.PetScalingPct(ownerStats[stats.Armor], 35),
			stats.AttackPower:      core.PetScalingPct(ownerStats[stats.SpellPower], 57),
			stats.SpellPower:       core.PetScalingPct(ownerStats[stats.SpellPower], 15),
			stats.SpellPenetration: ownerStats[stats.SpellPenetration],
		}
	}

	infernal := &InfernalPet{
		Pet: core.NewPet("Infernal", &warlock.Character, core.PetCreateStats(314, 90, 328, 150, 209, 16148).Add(stats.Stats{
			stats.MeleeCrit: 5 * core.CritRatingPerCritChance,
		}), statInheritance, false, true),
		owner: warlock,
	}
	infernal.BuffImmunity = core.PetImmuneToScaling

	infernal.AddStatDependency(stats.Strength, stats.AttackPower, 2)
	infernal.AddStat(stats.AttackPower, -20)

	// command doesn't apply to infernal
	if warlock.RacialTraits == proto.Race_RaceOrc {
		infernal.PseudoStats.DamageDealtMultiplier /= 1.05
	}

	// InitStatsForLevel's NPC_INFERNAL case: pet_levelstats' weapon range
	infernal.EnableAutoAttacks(infernal, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:  1263,
			BaseDamageMax:  1768,
			SwingSpeed:     2,
			CritMultiplier: 2,
		},
		AutoSwingMelee: true,
	})

	warlock.AddPet(infernal)

	return infernal
}

func (infernal *InfernalPet) GetPet() *core.Pet {
	return infernal.Pet
}

func (infernal *InfernalPet) Initialize() {
	infernal.immolationAura = infernal.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 20153},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskEmpty,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label:    "Immolation",
				ActionID: core.ActionID{SpellID: 19483},
			},
			NumberOfTicks:       31,
			TickLength:          time.Second * 2,
			AffectedByCastSpeed: false,
			TicksCanCrit:        false,

			// scales with the Infernal's own SP, which spell_warl_infernal_scaling sets to 15% of the
			// warlock's, spirit-based SP included
			Tick: core.SpellEffect{Effect: 0, Min: 40, Max: 40, SP: 1.35},

			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				baseDmg := dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.SP*dot.Spell.SpellPower())

				for _, aoeTarget := range sim.Encounter.TargetUnits {
					dot.Spell.CalcAndDealDamage(sim, aoeTarget, baseDmg, dot.Spell.OutcomeMagicHit)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.AOEDot().Apply(sim)
		},
	})
}

func (infernal *InfernalPet) Reset(_ *core.Simulation) {
}

func (infernal *InfernalPet) ExecuteCustomRotation(sim *core.Simulation) {
	infernal.immolationAura.Cast(sim, nil)
}
