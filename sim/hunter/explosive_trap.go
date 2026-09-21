package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (hunter *Hunter) registerExplosiveTrapSpell(timer *core.Timer) {
	hasGlyph := hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfExplosiveTrap)
	bonusPeriodicDamageMultiplier := .10 * float64(hunter.Talents.TrapMastery)

	hunter.ExplosiveTrap = hunter.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 49067},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.19,
			Multiplier: 1 - 0.2*float64(hunter.Talents.Resourcefulness),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second*30 - time.Second*2*time.Duration(hunter.Talents.Resourcefulness),
			},
		},

		DamageMultiplierAdditive: 1 +
			.02*float64(hunter.Talents.TNT),
		// the damage is 49065's: magic class with SPELL_ATTR3_ALWAYS_HIT, so it never misses and
		// crits off the hunter's spell crit for +50%
		CritMultiplier:   hunter.DefaultSpellCritMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "Explosive Trap",
			},
			NumberOfTicks: 10,
			TickLength:    time.Second * 2,
			// Glyph of Explosive Trap is an SPELL_AURA_ABILITY_PERIODIC_CRIT on the trap's periodic
			TicksCanCrit: hasGlyph,

			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				baseDamage := 90 + 0.1*dot.Spell.RangedAttackPower(target)
				dot.Spell.DamageMultiplierAdditive += bonusPeriodicDamageMultiplier
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					if hasGlyph {
						dot.Spell.CalcAndDealPeriodicDamage(sim, aoeTarget, baseDamage, dot.Spell.OutcomeMagicCrit)
					} else {
						dot.Spell.CalcAndDealPeriodicDamage(sim, aoeTarget, baseDamage, dot.OutcomeTickCounted)
					}
				}
				dot.Spell.DamageMultiplierAdditive -= bonusPeriodicDamageMultiplier
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Traps only last 30s.
			if sim.CurrentTime < -time.Second*30 {
				return
			}

			// The trap's gameobject (189322) arms its startDelay, 1 s, after it's laid, and goes off on
			// the next update after that (GameObject::Update). One laid before the pull waits for it.
			core.StartDelayedAction(sim, core.DelayedActionOptions{
				DoAt: max(0, sim.NextServerTick(sim.CurrentTime+time.Second)),
				OnAction: func(sim *core.Simulation) {
					// no 10 target cap: the trap's trigger creature casts it, not a player
					// (GameObject::CastSpell)
					for _, aoeTarget := range sim.Encounter.TargetUnits {
						baseDamage := sim.Roll(523, 671) + 0.1*spell.RangedAttackPower(aoeTarget)
						spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicCrit)
					}
					hunter.ExplosiveTrap.AOEDot().Apply(sim)
				},
			})
		},
	})

	timeToTrapWeave := time.Millisecond * time.Duration(hunter.Options.TimeToTrapWeaveMs)
	// mod-spell-tweaks' Trap Launcher (425777-425782) lays the trap from range, so there's no walk
	if hunter.Server().SpellTweaks.Enabled {
		timeToTrapWeave = 0
	}
	halfWeaveTime := timeToTrapWeave / 2
	hunter.TrapWeaveSpell = hunter.RegisterSpell(core.SpellConfig{
		ActionID: hunter.ExplosiveTrap.ActionID.WithTag(1),
		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagNoMetrics | core.SpellFlagNoLogs | core.SpellFlagAPL,

		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return hunter.ExplosiveTrap.CanCast(sim, target)
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if sim.CurrentTime < 0 {
				hunter.mayMoveAt = sim.CurrentTime
			}

			// Assume we started running after the most recent ranged auto, so that time
			// can be subtracted from the run in.
			reachLocationAt := hunter.mayMoveAt + halfWeaveTime
			layTrapAt := max(reachLocationAt, sim.CurrentTime)
			doneAt := layTrapAt + halfWeaveTime

			// Auto Shot fails with SPELL_FAILED_MOVING until we stop, then goes on the next update: it's
			// the one auto-repeat spell _UpdateAutoRepeatSpell spares the 500 ms restart delay.
			hunter.AutoAttacks.DelayRangedUntil(sim, doneAt)

			if layTrapAt == sim.CurrentTime {
				hunter.ExplosiveTrap.Cast(sim, target)
				if doneAt > hunter.GCD.ReadyAt() {
					hunter.GCD.Set(doneAt)
				}
			} else {
				// Make sure the GCD doesn't get used while we're waiting.
				hunter.WaitUntil(sim, doneAt)

				core.StartDelayedAction(sim, core.DelayedActionOptions{
					DoAt: layTrapAt,
					OnAction: func(sim *core.Simulation) {
						hunter.GCD.Reset()
						hunter.ExplosiveTrap.Cast(sim, target)
						if doneAt > hunter.GCD.ReadyAt() {
							hunter.GCD.Set(doneAt)
						}
					},
				})
			}
		},
	})
}
