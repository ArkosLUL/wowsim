package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (hunter *Hunter) registerVolleySpell() {
	// each tick is a cast of 58433, which adds its own bonus
	var tickThreat *core.ThreatTick
	hunter.Volley = hunter.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 58434},
		SpellSchool:  core.SpellSchoolArcane,
		ProcMask:     core.ProcMaskRangedSpecial,
		Flags:        core.SpellFlagChanneled | core.SpellFlagAPL,
		MissileSpeed: 30,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.17,
			Multiplier: 1 -
				core.TernaryFloat64(hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfVolley), 0.2, 0) -
				0.03*float64(hunter.Talents.Efficiency),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplier: 1 *
			(1 + 0.04*float64(hunter.Talents.Barrage)),
		CritMultiplier:   hunter.critMultiplier(true, false),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "Volley",
				// a channel with SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS: Auto Shot keeps going, its timer doesn't
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					hunter.AutoAttacks.SuspendRangedTimer(sim)
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					hunter.AutoAttacks.ResumeRangedTimer(sim)
				},
			},
			NumberOfTicks:       6,
			TickLength:          time.Second * 1,
			AffectedByCastSpeed: true,
			// each tick is 58433's own hit, which crits like any other
			TicksCanCrit: true,
			Tick:         core.SpellEffect{Effect: 0, FromSpellID: 58433, Min: 353, Max: 353, AP: 0.0837},

			OnSnapshot: func(sim *core.Simulation, _ *core.Unit, dot *core.Dot, _ bool) {
				target := hunter.CurrentTarget
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.AP, dot.Spell.RangedAttackPowerSpellBonus(target))
				dot.Spell.AoECapUnits = len(sim.Encounter.Targets)

				attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
				dot.SnapshotCritChance = dot.Spell.PhysicalCritChance(attackTable)
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					tickThreat.Hit(dot.CalcAndDealPeriodicSnapshotDamage(sim, aoeTarget, dot.OutcomeRangedHitAndCritSnapshot))
				}
				tickThreat.Done(sim)
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			spell.AOEDot().Apply(sim)
		},
	})
	tickThreat = hunter.Volley.NewThreatTick(58433)
}
