package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

const CryingWind int32 = 45270

func (druid *Druid) registerInsectSwarmSpell() {
	missAuras := druid.NewEnemyAuraArray(core.InsectSwarmAura)
	hasGlyph := druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfInsectSwarm)
	numTicks := 6 + core.TernaryInt32(druid.Talents.NaturesSplendor, 1, 0)
	// spell_dru_insect_swarm (spell_druid.cpp:2008-2013): the relic's 374 is added to the tick's own
	// base points, split evenly over the ticks, not run through the dot's SP coefficient.
	idolFlatDamage := core.TernaryFloat64(druid.Ranged().ID == CryingWind, 374/float64(numTicks), 0)
	dotCanCrit := druid.balanceDotTicksCanCrit()
	addsTicks := druid.balanceDotAddsTicks()

	impISMultiplier := 1 + 0.01*float64(druid.Talents.ImprovedInsectSwarm)

	if druid.HasSetBonus(ItemSetNightsongGarb, 4) {
		druid.MoonkinT84PCAura = druid.RegisterAura(core.Aura{
			Label:    "Elune's Wrath",
			ActionID: core.ActionID{SpellID: 64823},
			Duration: time.Second * 10,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				druid.Starfire.CastTimeMultiplier -= 1
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				druid.Starfire.CastTimeMultiplier += 1
			},
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if druid.Starfire.IsEqual(spell) && (druid.Starfire.CurCast.CastTime < (10*time.Second - aura.RemainingDuration(sim))) {
					aura.Deactivate(sim)
				}
			},
		})
	}

	druid.InsectSwarm = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48468},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagOmenTrigger | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.08,
			Multiplier: 1,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplier: spellModDamage(
			0.01*float64(druid.Talents.Genesis),
			core.TernaryFloat64(druid.HasSetBonus(ItemSetDreamwalkerGarb, 2), 0.1, 0),
			core.TernaryFloat64(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfInsectSwarm), 0.3, 0),
		),
		CritMultiplier:   druid.BalanceCritMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Insect Swarm",
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					druid.Wrath.DamageMultiplier *= impISMultiplier
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					druid.Wrath.DamageMultiplier /= impISMultiplier
				},
			},
			NumberOfTicks:       numTicks,
			TickLength:          time.Second * 2,
			AffectedByCastSpeed: addsTicks,
			TickHaste:           core.SpellHasteAddsTicks,
			TicksCanCrit:        dotCanCrit,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = 215 + idolFlatDamage + 0.2*dot.Spell.SpellPower()
				dot.SnapshotCritChance = dot.Spell.SpellCritChance(target)
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				if dotCanCrit {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
				}

				if druid.MoonkinT84PCAura != nil && sim.RandomFloat("Elune's Wrath proc") < 0.08 {
					druid.MoonkinT84PCAura.Activate(sim)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
				if !hasGlyph {
					missAuras.Get(target).Activate(sim)
				}
			}
			spell.DealOutcome(sim, result)
		},
	})

	if !hasGlyph {
		druid.InsectSwarm.RelatedAuras = append(druid.InsectSwarm.RelatedAuras, missAuras)
	}
}
