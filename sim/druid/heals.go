package druid

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Tree of Life has SHAPESHIFT_FLAG_STANCE, so it acts as no form for SpellInfo::CheckShapeshift and every heal
// casts in it: none has Tree in its StancesNot.
const healForms = Humanoid | Tree

// hotCostMultiplier is Tree of Life Passive (5420): -20% cost on Rejuvenation, Regrowth, Lifebloom and Wild
// Growth. It has no Stances, so it holds in any form once the talent has taught it.
func (druid *Druid) hotCostMultiplier() float64 {
	return 1 - core.TernaryFloat64(druid.Talents.TreeOfLife, 0.2, 0)
}

// directHealMultiplier is the SPELLMOD_DAMAGE percents on Healing Touch, Regrowth's heal, Nourish and the bloom:
// Gift of Nature's 2% a point. Its mask leaves Swiftmend out.
func (druid *Druid) directHealMultiplier() float64 {
	return spellModDamage(0.02 * float64(druid.Talents.GiftOfNature))
}

// periodicHealMultiplier is the SPELLMOD_DOT percents on a hot, which multiply: Genesis' 1% and Gift of Nature's 2%
// a point on all four, plus the hot's own.
func (druid *Druid) periodicHealMultiplier(own ...float64) float64 {
	return spellModDamage(0.01*float64(druid.Talents.Genesis), 0.02*float64(druid.Talents.GiftOfNature)) *
		spellModDamage(own...)
}

// Moonglow takes 3% a point off Healing Touch, Rejuvenation, Regrowth and Nourish.
func (druid *Druid) moonglowCost() float64 {
	return 0.03 * float64(druid.Talents.Moonglow)
}

// Tranquil Spirit takes 2% a point off Healing Touch and Nourish.
func (druid *Druid) tranquilSpiritCost() float64 {
	return 0.02 * float64(druid.Talents.TranquilSpirit)
}

// healThreatMultiplier is Subtlety's SPELLMOD_THREAT, 10% a point off every heal.
func (druid *Druid) healThreatMultiplier() float64 {
	return 1 - 0.1*float64(druid.Talents.Subtlety)
}

// empoweredRejuvenation is its SPELLMOD_BONUS_MULTIPLIER: 4% a point on the coefficients of Rejuvenation,
// Regrowth, Lifebloom, the bloom and Wild Growth.
func (druid *Druid) empoweredRejuvenation() []core.SpellMod {
	return []core.SpellMod{{Op: core.SpellModBonusMultiplier, Pct: 4 * druid.Talents.EmpoweredRejuvenation}}
}

// hotSnapshot is a hot's amount as AuraEffect::CalculateAmount takes it: the roll and the healing power with the
// SPELLMOD_DOT percents, but not the healing done percents, which each tick takes. The spell's DamageMultiplier is
// its direct heal's.
func hotSnapshot(periodic float64) core.OnSnapshot {
	return hotSnapshotWith(periodic, 0, nil)
}

// hotSnapshotWith adds a flat SPELLMOD_DOT after every percent, as Player::ApplySpellMod does, and the multiplier
// extra returns (Glyph of Regrowth's, which reads the target as the hot goes on).
func hotSnapshotWith(periodic float64, flat float64, extra func(target *core.Unit) float64) core.OnSnapshot {
	return func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
		multiplier := periodic
		if extra != nil {
			multiplier *= extra(target)
		}
		dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.SP, dot.Spell.HealingPower(target)) + flat/multiplier
		dot.SnapshotHotMultiplier(multiplier)
	}
}

func tickHot(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
	dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
}

// wholeHot is a hot's whole amount for one stack, what SpellHealingBonusDone returns for it, a flat SPELLMOD_DOT
// added after the percents.
func wholeHot(sim *core.Simulation, target *core.Unit, dot *core.Dot, periodic float64, flat float64) float64 {
	return max(0, math.Trunc((dot.Tick.Roll(sim)+core.PowerBonus(dot.Tick.SP, dot.Spell.HealingPower(target)))*periodic+flat))
}

func (druid *Druid) registerHealingTouchSpell() {
	glyphCost, glyphDamage, glyphCastTime := druid.healingTouchGlyph()
	druid.HealingTouch = druid.RegisterSpell(healForms, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48378},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL | SpellFlagNaturesGrace | SpellFlagOmenTrigger,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.33,
			Multiplier: 1 - druid.moonglowCost() - druid.tranquilSpiritCost() + glyphCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second*3 - time.Millisecond*100*time.Duration(druid.Talents.Naturalist) + glyphCastTime,
			},
		},

		BonusCritRating:  2 * float64(druid.Talents.NaturesMajesty) * core.CritRatingPerCritChance,
		DamageMultiplier: spellModDamage(0.02*float64(druid.Talents.GiftOfNature), glyphDamage),
		CritMultiplier:   druid.DefaultHealingCritMultiplier(),
		ThreatMultiplier: druid.healThreatMultiplier(),

		Direct: core.SpellEffect{Effect: 0, Min: 3761, Max: 4439, SP: 1.61},
		Mods:   []core.SpellMod{{Op: core.SpellModBonusMultiplier, Flat: 20 * druid.Talents.EmpoweredTouch}},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.HealingPower(target))
			spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
		},
	})
}

func (druid *Druid) registerRegrowthSpell() {
	glyph := druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfRegrowth)
	glyphBonus := 1 + serverBonus(spellGlyphOfRegrowth, 0)/100
	druid.Regrowth = druid.RegisterSpell(healForms, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48443},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL | SpellFlagNaturesGrace | SpellFlagOmenTrigger,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.29,
			Multiplier: druid.hotCostMultiplier() - druid.moonglowCost(),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second * 2,
			},
		},

		BonusCritRating:  5 * float64(druid.Talents.NaturesBounty) * core.CritRatingPerCritChance,
		DamageMultiplier: druid.directHealMultiplier(),
		CritMultiplier:   druid.DefaultHealingCritMultiplier(),
		ThreatMultiplier: druid.healThreatMultiplier(),

		Direct: core.SpellEffect{Effect: 0, Min: 2257, Max: 2517, SP: 0.3},
		Mods:   druid.empoweredRejuvenation(),
		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Regrowth",
			},
			// Nature's Splendor adds 6 s
			NumberOfTicks: 7 + core.TernaryInt32(druid.Talents.NaturesSplendor, 2, 0),
			TickLength:    time.Second * 3,
			TicksCanCrit:  false,
			Tick:          core.SpellEffect{Effect: 1, Min: 335, Max: 335, SP: 0.1878},
			OnSnapshot: hotSnapshotWith(druid.periodicHealMultiplier(), 0, func(target *core.Unit) float64 {
				// Glyph of Regrowth reads the target's Regrowth as the amount is worked out
				return core.TernaryFloat64(glyph && druid.targetHasRegrowth(target), glyphBonus, 1)
			}),
			OnTick: tickHot,
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// the heal and its crit are worked out at the launch, the hot goes on at the hit, then the heal lands
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.HealingPower(target))
			if glyph && druid.targetHasRegrowth(target) {
				spell.DamageMultiplier *= glyphBonus
				defer func() { spell.DamageMultiplier /= glyphBonus }()
			}
			result := spell.CalcHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
			spell.Hot(target).Apply(sim)
			spell.DealHealing(sim, result)
		},
	})
}

func (druid *Druid) registerRejuvenationSpell() {
	hotFlat := druid.rejuvenationFlatHealing()
	t9Crit := druid.HasSetBonus(ItemSetMalfurionsGarb, 4)
	snapshot := hotSnapshotWith(druid.periodicHealMultiplier(0.05*float64(druid.Talents.ImprovedRejuvenation)), hotFlat, nil)
	druid.Rejuvenation = druid.RegisterSpell(healForms, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48441},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL | SpellFlagOmenTrigger,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.18,
			Multiplier: druid.hotCostMultiplier() - druid.moonglowCost(),
			FlatMod:    druid.rejuvenationFlatCost(),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   druid.DefaultHealingCritMultiplier(),
		ThreatMultiplier: druid.healThreatMultiplier(),

		Mods: druid.empoweredRejuvenation(),
		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Rejuvenation",
			},
			// Nature's Splendor adds 3 s
			NumberOfTicks: 5 + core.TernaryInt32(druid.Talents.NaturesSplendor, 1, 0),
			TickLength:    time.Second * 3,
			// Glyph of Rapid Rejuvenation: haste shortens the ticks and the hot with them
			AffectedByCastSpeed: druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfRapidRejuvenation),
			TickHaste:           core.SpellHasteScalesBoth,
			TicksCanCrit:        t9Crit,
			Tick:                core.SpellEffect{Effect: 0, Min: rejuvenationTickBase, Max: rejuvenationTickBase, SP: 0.376},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				snapshot(sim, target, dot, isRollover)
				dot.SnapshotCritChance = dot.Spell.HealingCritChance()
			},
			OnTick: tickRejuvenation,
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Hot(target).Apply(sim)
			druid.landHot(sim, spell, target)
			druid.rejuvenationAppliedT8(sim, target)
		},
	})
}

// landHot is the hit-phase proc of a cast that only puts a hot on: Omen of Clarity's CheckProc lets a druid
// spell with no heal of its own through.
func (druid *Druid) landHot(sim *core.Simulation, spell *core.Spell, target *core.Unit) {
	spell.CalcAndDealOutcome(sim, target, spell.OutcomeAlwaysHit)
}

// nourishTakenBonus is Unit::SpellHealingBonusTaken's 1.2f for a Nourish on a target with the caster's
// Rejuvenation, Regrowth, Lifebloom or Wild Growth, kept in float32.
var nourishTakenBonus = float64(float32(1.2))

func (druid *Druid) registerNourishSpell() {
	glyphPerHot := serverBonus(spellGlyphOfNourish, 0)
	glyph := druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfNourish)
	relicFlat := druid.nourishRelicFlat()
	t9Crit := core.TernaryFloat64(druid.HasSetBonus(ItemSetMalfurionsGarb, 2), serverBonus(spellT9Restoration2P, 0), 0)
	druid.Nourish = druid.RegisterSpell(healForms, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 50464},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL | SpellFlagNaturesGrace | SpellFlagOmenTrigger,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.18,
			Multiplier: 1 - druid.moonglowCost() - druid.tranquilSpiritCost(),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 1500,
			},
		},

		BonusCritRating: (2*float64(druid.Talents.NaturesMajesty) + 5*float64(druid.Talents.NaturesBounty) + t9Crit) *
			core.CritRatingPerCritChance,
		DamageMultiplier: druid.directHealMultiplier(),
		CritMultiplier:   druid.DefaultHealingCritMultiplier(),
		ThreatMultiplier: druid.healThreatMultiplier(),

		Direct: core.SpellEffect{Effect: 0, Min: 1883, Max: 2187, SP: 0.672},
		Mods:   []core.SpellMod{{Op: core.SpellModBonusMultiplier, Flat: 10 * druid.Talents.EmpoweredTouch}},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.HealingPower(target))
			if relicFlat != 0 {
				baseHealing += relicFlat / spell.CasterHealingMultiplier()
			}
			// spell_dru_nourish adds the glyph's percent per hot of the druid's, after the heal's terms and before the crit
			outcome := spell.OutcomeHealingCrit
			if hots := druid.countHots(target); glyph && hots > 0 {
				pct := float32(glyphPerHot) * float32(hots)
				outcome = func(sim *core.Simulation, result *core.SpellResult, table *core.AttackTable) {
					result.Damage += math.Trunc(float64(float32(result.Damage) * pct / 100))
					spell.OutcomeHealingCrit(sim, result, table)
				}
			}
			if !druid.hasNourishHot(target) {
				spell.CalcAndDealHealing(sim, target, baseHealing, outcome)
				return
			}
			// the bonus multiplies the taken percents, so it rides the caster's taken multiplier for this heal only
			table := druid.AttackTables[target.UnitIndex]
			taken := table.HealingDealtMultiplier
			table.HealingDealtMultiplier *= nourishTakenBonus
			result := spell.CalcHealing(sim, target, baseHealing, outcome)
			table.HealingDealtMultiplier = taken
			spell.DealHealing(sim, result)
		},
	})
}

// hasNourishHot is whether the target has one of the druid's own Rejuvenation, Regrowth, Lifebloom or Wild Growth.
func (druid *Druid) hasNourishHot(target *core.Unit) bool {
	for _, spell := range []*DruidSpell{druid.Rejuvenation, druid.Regrowth, druid.Lifebloom, druid.WildGrowth, druid.BlessingOfTheEternal} {
		if spell != nil && spell.Hot(target).IsActive() {
			return true
		}
	}
	return false
}

func (druid *Druid) registerSwiftmendSpell() {
	if !druid.Talents.Swiftmend {
		return
	}
	keepsHot := druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfSwiftmend)
	// T8 2 piece: SPELLMOD_DAMAGE on the whole amount, in float32
	t8 := druid.HasSetBonus(ItemSetNightsongRegalia, 2)
	t8Mod := float32(1) + float32(serverBonus(spellT8Restoration2P, 0))/100

	druid.Swiftmend = druid.RegisterSpell(healForms, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 18562},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		// the consumed ticks already carry the target's healing taken
		Flags: core.SpellFlagHelpful | core.SpellFlagAPL | SpellFlagNaturesGrace | SpellFlagOmenTrigger |
			core.SpellFlagIgnoreTargetModifiers,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.16,
			Multiplier: 1,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Second * 15,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   druid.DefaultHealingCritMultiplier(),
		ThreatMultiplier: druid.healThreatMultiplier(),

		Direct: core.SpellEffect{Effect: 0, Min: 1, Max: 1},

		// AURA_STATE_SWIFTMEND: a Rejuvenation or Regrowth on the target
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			hot, _ := druid.swiftmendHot(sim, target)
			return hot != nil
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			hot, ticks := druid.swiftmendHot(sim, target)
			if hot == nil {
				return
			}
			// Spell::EffectHeal: the effect's 1 plus the ticks of the hot as the target would take one, and no
			// healing done or taken of Swiftmend's own
			done := hot.HotTickHealing()
			// the server's tick takes the hot caster's whole healing done, Strength of Wrynn's included. Its
			// stand-in sits in the target's HealingTakenMultiplier, which TickHealingTaken skips
			if pct := target.HealingTakenPctMultiplier(); pct > 0 {
				// whole percents, so float drift in the multipliers can't cost a point
				wrynn := math.Round((target.PseudoStats.HealingTakenMultiplier/pct - 1) * 100)
				done = math.Trunc(done * (1 + wrynn/100))
			}
			tick := hot.TickHealingTaken(target, done)
			baseHealing := spell.Direct.Roll(sim) + float64(ticks)*tick
			if t8 {
				baseHealing = math.Trunc(float64(float32(baseHealing) * t8Mod))
			}
			if !keepsHot {
				hot.Cancel(sim)
			}
			spell.CalcAndDealHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
		},
	})
}

// swiftmendHot is the hot Swiftmend eats and how many of its ticks it heals for: a Rejuvenation (4) or Regrowth
// (6) with the least time left, a tie to the older one. T10's Blessing of the Eternal has Rejuvenation's family
// flag, so it counts as one. The druid's own come first, then another druid's: AURA_STATE_SWIFTMEND isn't per
// caster.
func (druid *Druid) swiftmendHot(sim *core.Simulation, target *core.Unit) (*core.Dot, int32) {
	if hot, ticks := shortestSwiftmendHot(sim, target, druid); hot != nil {
		return hot, ticks
	}
	var pick *core.Dot
	var ticks int32
	for _, unit := range druid.Env.Raid.AllPlayerUnits {
		other, ok := druid.Env.Raid.GetPlayerFromUnit(unit).(DruidAgent)
		if !ok || other.GetDruid() == druid {
			continue
		}
		if hot, n := shortestSwiftmendHot(sim, target, other.GetDruid()); hot != nil && isShorterHot(sim, hot, pick) {
			pick, ticks = hot, n
		}
	}
	return pick, ticks
}

func shortestSwiftmendHot(sim *core.Simulation, target *core.Unit, caster *Druid) (*core.Dot, int32) {
	var pick *core.Dot
	var ticks int32
	for _, c := range []struct {
		spell *DruidSpell
		ticks int32
	}{{caster.Rejuvenation, 4}, {caster.Regrowth, 6}, {caster.BlessingOfTheEternal, 4}} {
		if c.spell == nil {
			continue
		}
		if hot := c.spell.Hot(target); hot.IsActive() && isShorterHot(sim, hot, pick) {
			pick, ticks = hot, c.ticks
		}
	}
	return pick, ticks
}

func isShorterHot(sim *core.Simulation, hot *core.Dot, than *core.Dot) bool {
	return than == nil || hot.RemainingDuration(sim) < than.RemainingDuration(sim) ||
		hot.RemainingDuration(sim) == than.RemainingDuration(sim) && hot.StartedAt() < than.StartedAt()
}
