package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (druid *Druid) registerLacerateSpell() {
	idolBonus := core.TernaryInt32(druid.Ranged().ID == 27744, 8, 0) // Idol of Ursoc

	initialDamageMul := 1 *
		core.TernaryFloat64(druid.HasSetBonus(ItemSetLasherweaveBattlegear, 2), 1.2, 1) *
		core.TernaryFloat64(druid.HasSetBonus(ItemSetDreamwalkerBattlegear, 2), 1.05, 1)

	tickDamageMul := 1 *
		core.TernaryFloat64(druid.HasSetBonus(ItemSetLasherweaveBattlegear, 2), 1.2, 1) *
		core.TernaryFloat64(druid.HasSetBonus(ItemSetMalfurionsBattlegear, 2), 1.05, 1)

	druid.Lacerate = druid.RegisterSpell(Bear, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48568},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost:    15,
			FlatMod: -float64(druid.Talents.ShreddingAttacks),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: initialDamageMul,
		CritMultiplier:   druid.MeleeCritMultiplier(Bear),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: druid.applyRendAndTear(core.Aura{
				Label:     "Lacerate",
				MaxStacks: 5,
				Duration:  time.Second * 15,
			}),
			NumberOfTicks: 5,
			TickLength:    time.Second * 3,
			TicksCanCrit:  druid.Talents.PrimalGore,

			Tick: core.SpellEffect{Effect: 0, Min: 64, Max: 64, AP: 0.01},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.AP, dot.Spell.MeleeAttackPower())
				dot.SnapshotStacks = dot.Aura.GetStacks()

				if !isRollover {
					attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
					dot.Spell.DamageMultiplier = tickDamageMul
					dot.SnapshotCritChance = dot.Spell.PhysicalCritChance(attackTable)
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable)
				}
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				if druid.Talents.PrimalGore {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.Spell.OutcomeAlwaysHit)
				}
			},
		},

		Direct: core.SpellEffect{Effect: 1, Min: 88, Max: 88},
		Mods: []core.SpellMod{
			{Op: core.SpellModAllEffects, Flat: idolBonus},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim)

			spell.DamageMultiplier = initialDamageMul
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				dot := spell.Dot(target)
				if dot.IsActive() {
					dot.Refresh(sim)
					dot.AddStack(sim)
					dot.TakeSnapshot(sim, true)
				} else {
					dot.Apply(sim)
					dot.SetStacks(sim, 1)
					dot.TakeSnapshot(sim, true)
				}
			} else {
				spell.IssueRefund(sim)
			}

			spell.DealDamage(sim, result)
		},
	})
}
