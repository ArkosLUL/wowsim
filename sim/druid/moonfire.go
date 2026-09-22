package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (druid *Druid) registerMoonfireSpell() {
	numTicks := druid.moonfireTicks()

	starfireBonusCrit := float64(druid.Talents.ImprovedInsectSwarm) * core.CritRatingPerCritChance
	// Malfurion's Regalia 2pc grants its own periodic-crit aura, on top of the one Earth and Moon
	// gets from mod-spell-tweaks (SpellTweaks_classes.cpp:453-459).
	dotCanCrit := druid.balanceDotTicksCanCrit() || druid.HasSetBonus(ItemSetMalfurionsRegalia, 2)
	addsTicks := druid.balanceDotAddsTicks()

	impMoonfire := 0.05 * float64(druid.Talents.ImprovedMoonfire)
	moonfury := []float64{0.0, 0.03, 0.06, 0.1}[druid.Talents.Moonfury]
	// Glyph of Moonfire (54829) carries both halves as percent spell mods, -90% on the direct hit
	// (SPELLMOD_DAMAGE) and +75% on the tick (SPELLMOD_DOT), so each multiplies with the talents.
	// Genesis only lists Moonfire on its SPELLMOD_DOT mask.
	glyphInitial := core.TernaryFloat64(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfMoonfire), -0.9, 0)
	glyphPeriodic := core.TernaryFloat64(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfMoonfire), 0.75, 0)

	initialDamageMultiplier := spellModDamage(impMoonfire, moonfury, glyphInitial)
	periodicDamageMultiplier := spellModDamage(impMoonfire, moonfury, 0.01*float64(druid.Talents.Genesis), glyphPeriodic)

	druid.Moonfire = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48463},
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagNaturesGrace | SpellFlagOmenTrigger | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.21,
			Multiplier: 1 - 0.03*float64(druid.Talents.Moonglow),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		BonusCritRating:  float64(druid.Talents.ImprovedMoonfire) * 5 * core.CritRatingPerCritChance,
		DamageMultiplier: initialDamageMultiplier,

		CritMultiplier:   druid.BalanceCritMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Moonfire",
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					druid.Starfire.BonusCritRating += starfireBonusCrit
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					druid.Starfire.BonusCritRating -= starfireBonusCrit
				},
			},
			NumberOfTicks:       druid.moonfireTicks(),
			TickLength:          time.Second * 3,
			AffectedByCastSpeed: addsTicks,
			TickHaste:           core.SpellHasteAddsTicks,
			TicksCanCrit:        dotCanCrit,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.Spell.DamageMultiplier = periodicDamageMultiplier
				dot.SnapshotBaseDamage = 200 + 0.13*dot.Spell.SpellPower()
				attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
				dot.SnapshotCritChance = dot.Spell.SpellCritChance(target)
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable)
				dot.Spell.DamageMultiplier = initialDamageMultiplier
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				if dotCanCrit {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
				} else {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTickCounted)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(406, 476) + 0.13*spell.SpellPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			if result.Landed() {
				druid.ExtendingMoonfireStacks = 3
				dot := spell.Dot(target)
				dot.NumberOfTicks = numTicks
				dot.Apply(sim)
			}
			spell.DealDamage(sim, result)
		},
	})
}

func (druid *Druid) moonfireTicks() int32 {
	return 4 +
		core.TernaryInt32(druid.Talents.NaturesSplendor, 1, 0) +
		core.TernaryInt32(druid.HasSetBonus(ItemSetThunderheartRegalia, 2), 1, 0)
}

// spellModDamage stacks percent damage bonuses from talents, glyphs and set pieces the server's way:
// Player::ApplySpellMod multiplies SPELLMOD_DAMAGE and SPELLMOD_DOT percentages, it doesn't add them.
func spellModDamage(bonuses ...float64) float64 {
	multiplier := 1.0
	for _, bonus := range bonuses {
		multiplier *= 1 + bonus
	}
	return multiplier
}

// balanceDotAddsTicks is mod-spell-tweaks' spell_tweaks_balance_dot_haste (SpellTweaks_classes.cpp):
// with Eclipse talented, spell haste shortens Moonfire and Insect Swarm's tick interval and their
// duration stays put, so it fits more ticks.
func (druid *Druid) balanceDotAddsTicks() bool {
	return druid.Talents.Eclipse > 0 && druid.Server().SpellTweaks.BalanceDotScaling
}

// balanceDotTicksCanCrit is mod-spell-tweaks' spell_tweaks_balance_dot_crit_apply
// (SpellTweaks_classes.cpp): with Earth and Moon talented, Moonfire and Insect Swarm's ticks can
// crit.
func (druid *Druid) balanceDotTicksCanCrit() bool {
	return druid.Talents.EarthAndMoon > 0 && druid.Server().SpellTweaks.BalanceDotScaling
}
