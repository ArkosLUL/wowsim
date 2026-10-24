package druid

import (
	"fmt"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The glyph, set bonus and relic spells the Restoration code reads its numbers from.
const (
	spellGlyphOfHealingTouch  = 54825
	spellGlyphOfLifebloom     = 54826
	spellGlyphOfRegrowth      = 54743
	spellGlyphOfRejuvenation  = 54754
	spellGlyphOfRejuvHeal     = 54755
	spellGlyphOfWildGrowth    = 62970
	spellGlyphOfNourish       = 62971
	spellGlyphOfInnervate     = 54832
	spellGlyphOfInnervateMana = 54833

	spellT7Restoration2P  = 60135
	spellT8Restoration2P  = 64756
	spellT8Restoration4P  = 64760
	spellT8RejuvenationHl = 64801
	spellT9Restoration2P  = 67127
	spellT10Restoration2P = 70658
	spellT10Restoration4P = 70664
	spellBlessing         = 70691

	rejuvenationTickBase = 338
)

const (
	idolOfBuddingLife       = 33508
	idolOfAwakening         = 40342
	idolOfPureThoughts      = 38366
	idolOfLushMoss          = 40711
	idolOfTheFlourishingLfe = 46138
	idolOfFlaringGrowth     = 47671
	idolOfTheBlackWillow    = 50454
)

// The relics' spells, which carry the numbers.
const (
	spellBuddingLife   = 43736
	spellAwakening     = 60780
	spellPureThoughts  = 57918
	spellLushMoss      = 60779
	spellFlourishing   = 64949
	spellFlaringGrowth = 67356
	spellRejuvenating  = 67358
	spellBlackWillow   = 71178
	spellSoothing      = 71184
)

// serverBonus is the value of effect i of a passive spell as the server rolls it.
func serverBonus(spellID int32, effect int) float64 {
	spell := serverdata.SpellByID(spellID)
	if spell == nil {
		panic(fmt.Sprintf("spell %d isn't in the server data", spellID))
	}
	lo, _ := spell.EffectRange(effect)
	return float64(lo)
}

var ItemSetDreamwalkerRegalia = core.NewItemSet(core.ItemSet{
	Name: "Dreamwalker Regalia",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Lifebloom's cost, in lifebloom.go
		},
		// 60137 is a class script (8477) on Nourish that no server code reads
		4: func(agent core.Agent) {},
	},
})

var ItemSetNightsongRegalia = core.NewItemSet(core.ItemSet{
	Name: "Nightsong Regalia",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Swiftmend's healing, in heals.go
		},
		4: func(agent core.Agent) {
			// Rejuvenation's instant heal, in rejuvenationAppliedT8
		},
	},
})

var ItemSetMalfurionsGarb = core.NewItemSet(core.ItemSet{
	Name:            "Malfurion's Garb",
	AlternativeName: "Runetotem's Garb",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Nourish's crit chance, in heals.go
		},
		4: func(agent core.Agent) {
			// Rejuvenation ticks can crit, in heals.go
		},
	},
})

var ItemSetLasherweaveGarb = core.NewItemSet(core.ItemSet{
	Name: "Lasherweave Garb",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Wild Growth's slower decay, in wild_growth.go
		},
		4: func(agent core.Agent) {
			// Blessing of the Eternal, in registerRejuvenationProcs
		},
	},
})

// healingTouchGlyph is Glyph of Healing Touch's cost and damage percent mods, as fractions to add to the cost
// multiplier and to multiply the heal by, and its cast time.
func (druid *Druid) healingTouchGlyph() (cost float64, damage float64, castTime time.Duration) {
	if !druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfHealingTouch) {
		return 0, 0, 0
	}
	return serverBonus(spellGlyphOfHealingTouch, 0) / 100, serverBonus(spellGlyphOfHealingTouch, 1) / 100,
		time.Duration(serverBonus(spellGlyphOfHealingTouch, 2)) * time.Millisecond
}

func (druid *Druid) lifebloomCostMod() float64 {
	if druid.HasSetBonus(ItemSetDreamwalkerRegalia, 2) {
		return serverBonus(spellT7Restoration2P, 0) / 100
	}
	return 0
}

// rejuvenationFlatCost is the flat SPELLMOD_COST of Idol of Budding Life and Idol of Awakening.
func (druid *Druid) rejuvenationFlatCost() float64 {
	switch druid.Ranged().ID {
	case idolOfBuddingLife:
		return serverBonus(spellBuddingLife, 0)
	case idolOfAwakening:
		return serverBonus(spellAwakening, 0)
	}
	return 0
}

// rejuvenationFlatHealing is Idol of Pure Thoughts' flat SPELLMOD_DOT.
func (druid *Druid) rejuvenationFlatHealing() float64 {
	if druid.Ranged().ID == idolOfPureThoughts {
		return serverBonus(spellPureThoughts, 0)
	}
	return 0
}

// lifebloomFlatHealing is spell_dru_idol_lifebloom's flat SPELLMOD_DOT: a seventh of the relic's amount.
func (druid *Druid) lifebloomFlatHealing() float64 {
	if druid.Ranged().ID == idolOfLushMoss {
		return float64(int32(serverBonus(spellLushMoss, 0)) / 7)
	}
	return 0
}

// lifebloomBloomBonus is the Idols of Tenacity's flat SPELLMOD_EFFECT2 on the bloom's base, per stack.
func (druid *Druid) lifebloomBloomBonus() int32 {
	// each season's idol and its spell
	switch druid.Ranged().ID {
	case 33841:
		return int32(serverBonus(43725, 0))
	case 35021:
		return int32(serverBonus(46100, 0))
	case 42576:
		return int32(serverBonus(60733, 0))
	case 42577:
		return int32(serverBonus(60736, 0))
	case 42578:
		return int32(serverBonus(60738, 0))
	case 42579:
		return int32(serverBonus(60739, 0))
	case 42580:
		return int32(serverBonus(60740, 0))
	case 51423:
		return int32(serverBonus(60741, 0))
	}
	return 0
}

// nourishRelicFlat is Idol of the Flourishing Life's flat SPELLMOD_DAMAGE on Nourish, which goes on after the
// percents. Its healing power branch in Unit::SpellDamageBonusDone never sees a heal.
func (druid *Druid) nourishRelicFlat() float64 {
	if druid.Ranged().ID == idolOfTheFlourishingLfe {
		return serverBonus(spellFlourishing, 0)
	}
	return 0
}

// wildGrowthT10Cut is the T10 2 piece's percent off Wild Growth's decay, 0 without it.
func (druid *Druid) wildGrowthT10Cut() float32 {
	if druid.HasSetBonus(ItemSetLasherweaveGarb, 2) {
		return float32(-serverBonus(spellT10Restoration2P, 0))
	}
	return 0
}

// wildGrowthDecay is the percent Wild Growth's bonus drops each tick once spell_dru_wild_growth_aura has worked
// out the amount: the T10 cut comes off what the aura held before. The script keeps the decay over a refresh,
// which works the amount out again, so each refresh takes the cut off once more.
func wildGrowthDecay(decay float32, cut float32) float32 {
	return decay + float32(decay*cut)/100
}

// targetHasRegrowth is whether any druid's Regrowth is on the target, which is what Glyph of Regrowth looks for.
func (druid *Druid) targetHasRegrowth(target *core.Unit) bool {
	for _, unit := range druid.Env.Raid.AllPlayerUnits {
		if other, ok := druid.Env.Raid.GetPlayerFromUnit(unit).(DruidAgent); ok {
			if regrowth := other.GetDruid().Regrowth; regrowth != nil && regrowth.Hot(target).IsActive() {
				return true
			}
		}
	}
	return false
}

// countHots is the druid's periodic heals on the target, which Glyph of Nourish counts.
func (druid *Druid) countHots(target *core.Unit) int {
	var n int
	for _, spell := range []*DruidSpell{druid.Rejuvenation, druid.Regrowth, druid.Lifebloom, druid.WildGrowth, druid.BlessingOfTheEternal} {
		if spell != nil && spell.Hot(target).IsActive() {
			n++
		}
	}
	return n
}

// tickRejuvenation rolls a T9 crit on the chance the hot took as it went on or refreshed
// (AuraEffect::CalculatePeriodicData), not the druid's chance at the tick.
func tickRejuvenation(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
	if !dot.TicksCanCrit {
		tickHot(sim, target, dot)
		return
	}
	dot.CalcAndDealPeriodicSnapshotHealing(sim, target, func(sim *core.Simulation, result *core.SpellResult, _ *core.AttackTable) {
		if sim.UnitRandomFloat(dot.Spell.Unit, "Healing Crit Roll") < dot.SnapshotCritChance {
			result.Outcome = core.OutcomeCrit
			result.Damage *= dot.Spell.CritMultiplier
			dot.Spell.SpellMetrics[result.Target.UnitIndex].Crits++
		} else {
			result.Outcome = core.OutcomeHit
		}
	})
}

// registerRestorationItems registers what only some gear and glyphs add: T10's hot, the Glyph of Rejuvenation's
// heal and the procs on Rejuvenation's ticks.
func (druid *Druid) registerRestorationItems() {
	druid.registerBlessingOfTheEternal()
	druid.registerRejuvenationProcs()
}

// registerRejuvenationT8 goes before Tree of Life, which lists the direct heals Master Shapeshifter raises.
func (druid *Druid) registerRejuvenationT8() {
	if !druid.HasSetBonus(ItemSetNightsongRegalia, 4) {
		return
	}
	// 64801 takes Rejuvenation's mask: Empowered Rejuvenation's coefficient, nothing of Gift of Nature's. It has
	// SPELL_ATTR2_CANT_CRIT.
	druid.t8RejuvenationHeal = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellT8RejuvenationHl},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,

		DamageMultiplier: 1,
		ThreatMultiplier: druid.healThreatMultiplier(),

		Direct: core.SpellEffect{Effect: 0, SP: 0.188},
		Mods:   druid.empoweredRejuvenation(),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// half of Rejuvenation's unmodified tick
			base := float64(int32(float32(rejuvenationTickBase) * 50 / 100))
			spell.CalcAndDealHealing(sim, target, base+core.PowerBonus(spell.Direct.SP, spell.HealingPower(target)), spell.OutcomeHealing)
		},
	})
}

// rejuvenationAppliedT8 is Aura::HandleAuraSpecificMods' T8 heal, on every Rejuvenation that goes on or refreshes.
func (druid *Druid) rejuvenationAppliedT8(sim *core.Simulation, target *core.Unit) {
	if druid.t8RejuvenationHeal != nil {
		druid.t8RejuvenationHeal.Cast(sim, target)
	}
}

func (druid *Druid) registerBlessingOfTheEternal() {
	if !druid.HasSetBonus(ItemSetLasherweaveGarb, 4) {
		return
	}
	// spell_dru_t10_restoration_4p_bonus_dummy hands the cast its heal as base points. 70691 has
	// SPELL_ATTR3_IGNORE_CASTER_MODIFIERS: no spell mods but the duration ones (Nature's Splendor, through
	// Rejuvenation's family flag), no healing power, only the healing done percents each tick takes.
	druid.BlessingOfTheEternal = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellBlessing},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,

		DamageMultiplier: 1,
		ThreatMultiplier: druid.healThreatMultiplier(),

		Hot: core.DotConfig{
			Aura:          core.Aura{Label: "Blessing of the Eternal"},
			NumberOfTicks: 5 + core.TernaryInt32(druid.Talents.NaturesSplendor, 1, 0),
			TickLength:    time.Second * 3,
			// Glyph of Rapid Rejuvenation's SPELL_AURA_PERIODIC_HASTE takes it by the same family flag
			AffectedByCastSpeed: druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfRapidRejuvenation),
			TickHaste:           core.SpellHasteScalesBoth,
			TicksCanCrit:        false,
			Tick:                core.SpellEffect{Effect: 0},
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = druid.blessingHeal
				dot.SnapshotHotMultiplier(1)
			},
			OnTick: tickHot,
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Hot(target).Apply(sim)
		},
	})
}

// blessingTarget is spell_dru_t10_restoration_4p_bonus' pick: grouped, the raid member with the lowest health
// percent and no Regrowth; alone, the druid. The proc casts 70691 with no target, so the healed unit can get it too:
// the script only leaves out the cast's explicit target, which there isn't.
func (druid *Druid) blessingTarget() *core.Unit {
	units := druid.Env.Raid.AllUnits
	if len(units) <= 1 {
		return &druid.Unit
	}
	var pick *core.Unit
	for _, unit := range units {
		if !unit.IsEnabled() || (unit.HasHealthBar() && unit.CurrentHealth() <= 0) || druid.targetHasRegrowth(unit) {
			continue
		}
		if pick == nil || healthPctOrder(unit) < healthPctOrder(pick) {
			pick = unit
		}
	}
	return pick
}

func (druid *Druid) registerGlyphOfRejuvenationHeal() {
	// 54755 has no damage class and no spell_bonus_data row, so it takes no healing power, no healing done percents
	// and no flat healing taken, only the taken percents. It can't crit (SPELL_ATTR2_CANT_CRIT).
	druid.glyphOfRejuvenationHeal = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellGlyphOfRejuvHeal},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete | core.SpellFlagIgnoreAttackerModifiers,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.CalcAndDealHealing(sim, target, druid.glyphHealAmount, spell.OutcomeHealing)
		},
	})
}

// registerRejuvenationProcs hangs what Rejuvenation's periodic heals proc on one aura: Glyph of Rejuvenation,
// the T10 4 piece, and Idols of Flaring Growth and the Black Willow.
func (druid *Druid) registerRejuvenationProcs() {
	glyph := druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfRejuvenation)
	t10 := druid.HasSetBonus(ItemSetLasherweaveGarb, 4)
	flaring := druid.Ranged().ID == idolOfFlaringGrowth
	willow := druid.Ranged().ID == idolOfTheBlackWillow
	if !glyph && !t10 && !flaring && !willow {
		return
	}

	var glyphPct, glyphThreshold float64
	if glyph {
		druid.registerGlyphOfRejuvenationHeal()
		glyphPct = serverBonus(spellGlyphOfRejuvenation, 0)
		glyphThreshold = serverBonus(spellGlyphOfRejuvenation, 0)
	}
	t10Chance := 0.0
	if t10 {
		t10Chance = float64(serverdata.ProcBySpellID(spellT10Restoration4P).Chance) / 100
	}

	var flaringAura *core.Aura
	var flaringIcd core.Cooldown
	var flaringChance float64
	if flaring {
		proc := serverdata.ProcBySpellID(spellFlaringGrowth)
		flaringChance = float64(proc.Chance) / 100
		flaringIcd = core.Cooldown{Timer: druid.NewTimer(), Duration: time.Duration(proc.CooldownMs) * time.Millisecond}
		flaringAura = druid.NewTemporaryStatsAura("Rejuvenating", core.ActionID{SpellID: spellRejuvenating},
			stats.Stats{stats.SpellPower: serverBonus(spellRejuvenating, 0)}, time.Duration(serverdata.SpellByID(spellRejuvenating).DurationMs)*time.Millisecond)
	}
	var willowAura *core.Aura
	var willowChance float64
	if willow {
		willowChance = float64(serverdata.ProcBySpellID(spellBlackWillow).Chance) / 100
		soothing := serverdata.SpellByID(spellSoothing)
		willowAura = core.MakeStackingAura(druid.GetCharacter(), core.StackingStatAura{
			Aura: core.Aura{
				Label:     "Soothing",
				ActionID:  core.ActionID{SpellID: spellSoothing},
				Duration:  time.Duration(soothing.DurationMs) * time.Millisecond,
				MaxStacks: soothing.StackAmount,
			},
			BonusPerStack: stats.Stats{stats.SpellPower: serverBonus(spellSoothing, 1)},
		})
	}

	druid.RegisterAura(core.Aura{
		Label:    "Rejuvenation procs",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnPeriodicHealDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			isBlessing := druid.BlessingOfTheEternal != nil && druid.BlessingOfTheEternal.IsEqual(spell)
			if !isBlessing && !druid.Rejuvenation.IsEqual(spell) {
				return
			}
			target := result.Target
			heal := result.Damage
			if heal <= 0 {
				return
			}

			if glyph && target.HasHealthBar() && target.CurrentHealth() < float64(int64(target.MaxHealth())*int64(glyphThreshold)/100) {
				druid.glyphHealAmount = float64(int32(float32(int32(heal)) * float32(glyphPct) / 100))
				druid.glyphOfRejuvenationHeal.Cast(sim, target)
			}
			if t10 && !isBlessing && (len(druid.Env.Raid.AllUnits) > 1 || target != &druid.Unit) &&
				sim.RandomFloat("Druid T10 Restoration 4P Bonus") < t10Chance {
				if pick := druid.blessingTarget(); pick != nil {
					druid.blessingHeal = heal
					druid.BlessingOfTheEternal.Cast(sim, pick)
				}
			}
			if flaring && flaringIcd.IsReady(sim) && sim.RandomFloat("Idol of Flaring Growth") < flaringChance {
				flaringIcd.Use(sim)
				flaringAura.Activate(sim)
			}
			if willow && sim.RandomFloat("Idol of the Black Willow") < willowChance {
				willowAura.Activate(sim)
				willowAura.AddStack(sim)
			}
		},
	})
}

// registerGlyphOfInnervate is spell_dru_glyph_of_innervate: each Innervate cast gives the druid a share of its
// base mana over 10 s, whoever it targets.
func (druid *Druid) registerGlyphOfInnervate() *core.Aura {
	manaMetrics := druid.NewManaMetrics(core.ActionID{SpellID: spellGlyphOfInnervateMana})
	manaMetrics.EnergizeThreat(core.EnergizeByTick)
	spell := serverdata.SpellByID(spellGlyphOfInnervateMana)
	duration := time.Duration(spell.DurationMs) * time.Millisecond
	ticks := int32(spell.DurationMs / spell.Effects[0].AmplitudeMs)
	perTick := float64(int32(druid.BaseMana) * int32(serverBonus(spellGlyphOfInnervate, 0)) / 100 / ticks)

	return druid.RegisterAura(core.Aura{
		Label:    "Glyph of Innervate",
		ActionID: core.ActionID{SpellID: spellGlyphOfInnervateMana},
		Duration: duration,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{
				Period:   time.Duration(spell.Effects[0].AmplitudeMs) * time.Millisecond,
				NumTicks: int(ticks),
				OnAction: func(sim *core.Simulation) {
					druid.AddMana(sim, perTick, manaMetrics)
				},
			})
		},
	})
}
