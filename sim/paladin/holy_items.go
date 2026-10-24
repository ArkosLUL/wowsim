package paladin

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// T7 holy
var ItemSetRedemptionRegalia = core.NewItemSet(core.ItemSet{
	Name: "Redemption Regalia",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Holy Shock crit, in holy_shock.go
		},
		4: func(agent core.Agent) {
			// Holy Light cost, in heals.go
		},
	},
})

// T8 holy
var ItemSetAegisRegalia = core.NewItemSet(core.ItemSet{
	Name: "Aegis Regalia",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Holy Mending, in holy_items.go
		},
		4: func(agent core.Agent) {
			// Sacred Shield's cooldown, in sacred_shield.go
		},
	},
})

// T9 holy (Alliance/Horde)
var ItemSetTuralyonsGarb = core.NewItemSet(core.ItemSet{
	Name:            "Turalyon's Garb",
	AlternativeName: "Liadrin's Garb",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// judgement durations, in judgement.go
		},
		4: func(agent core.Agent) {
			// Infusion of Light's Flash of Light hot, in holy_talents.go
		},
	},
})

// T10 holy
var ItemSetLightswornGarb = core.NewItemSet(core.ItemSet{
	Name: "Lightsworn Garb",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {
			// Divine Illumination's healing, in heals.go
		},
		4: func(agent core.Agent) {
			// Holiness, in holy_items.go
		},
	},
})

const (
	libramOfMostlyHolyDeeds   = 38364
	libramOfRenewal           = 40705
	libramOfTolerance         = 40268
	libramOfTheResolute       = 45436
	libramOfVeracity          = 47662
	libramOfBlindingLight     = 50460
	glyphOfHolyLightHealCount = 5
)

// holyItems is the holy glyphs', sets' and librams' state that outlives a cast.
type holyItems struct {
	// SPELLMOD_DAMAGE flats of the librams, added after every percent
	flatHolyLight, flatHolyShock float64
	// SPELLMOD_COST flat of the librams on Holy Light
	holyLightCostFlat float64

	// set bonuses, read once: HasSetBonus can't be called after the sim is built
	t9Judgements, t9InfusionHot, t10DivineIllumination bool

	glyphHeal       *core.Spell
	glyphHealAmount float64
	glyphTargets    []*core.Unit

	holyMending       *core.Spell
	holyMendingAmount float64

	holiness *core.Aura
	// the Holy Light being cast had its cast time cut by Holiness
	holinessUsed bool

	veracityAura *core.Aura
	veracityIcd  core.Cooldown
	blessedAura  *core.Aura
}

// libramEffects reads the set bonuses the casts check and the worn libram's flat heal and cost mods, once.
func (paladin *Paladin) libramEffects() {
	items := &paladin.holyItems
	items.t9Judgements = paladin.HasSetBonus(ItemSetTuralyonsGarb, 2)
	items.t9InfusionHot = paladin.HasSetBonus(ItemSetTuralyonsGarb, 4)
	items.t10DivineIllumination = paladin.HasSetBonus(ItemSetLightswornGarb, 2)
	switch paladin.Ranged().ID {
	case libramOfTolerance:
		items.flatHolyLight = 141
	case libramOfTheResolute:
		items.flatHolyLight = 160
	case libramOfMostlyHolyDeeds:
		items.flatHolyShock = 69
	case libramOfRenewal:
		items.holyLightCostFlat = -113
	}
}

// flatHealing is the libram's SPELLMOD_DAMAGE flat on a direct heal.
func (paladin *Paladin) flatHealing(spell *core.Spell) float64 {
	items := &paladin.holyItems
	switch spell {
	case paladin.HolyLight:
		return items.flatHolyLight
	case paladin.HolyShockHeal:
		return items.flatHolyShock
	}
	return 0
}

func (paladin *Paladin) registerHolyItems() {
	paladin.registerGlyphEffects()
	paladin.registerHolyMending()
	paladin.registerHoliness()
	paladin.registerLibramProcs()
}

// registerGlyphEffects: Glyph of Seal of Wisdom (54940) takes 5% off Holy Light, Flash of Light, Holy Shock and Beacon
// of Light while Seal of Wisdom is up, and Glyph of Seal of Light (54943) adds 5% to Holy Light, Flash of Light and
// Holy Shock's heal while Seal of Light is up. Both raise effect 2 of the seal (a cost and a damage percent, 0 by
// themselves).
func (paladin *Paladin) registerGlyphEffects() {
	if paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfHolyLight) {
		paladin.registerGlyphOfHolyLightHeal()
	}
	if paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfSealOfWisdom) {
		var spells []*core.Spell
		for _, spell := range []*core.Spell{paladin.HolyLight, paladin.FlashOfLight, paladin.HolyShock, paladin.BeaconOfLight} {
			if spell != nil {
				spells = append(spells, spell)
			}
		}
		paladin.SealOfWisdomAura.ApplyOnGain(func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range spells {
				spell.CostMultiplier -= 0.05
			}
		})
		paladin.SealOfWisdomAura.ApplyOnExpire(func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range spells {
				spell.CostMultiplier += 0.05
			}
		})
	}
	if paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfSealOfLight) {
		var spells []*core.Spell
		for _, spell := range []*core.Spell{paladin.HolyLight, paladin.FlashOfLight, paladin.HolyShockHeal} {
			if spell != nil {
				spells = append(spells, spell)
			}
		}
		paladin.SealOfLightAura.ApplyOnGain(func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range spells {
				spell.DamageMultiplier *= 1.05
			}
		})
		paladin.SealOfLightAura.ApplyOnExpire(func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range spells {
				spell.DamageMultiplier /= 1.05
			}
		})
	}
}

// setHolyShockCrit is T7's 2 piece, 10% crit on both of Holy Shock's.
func (paladin *Paladin) setHolyShockCrit() float64 {
	return core.TernaryFloat64(paladin.HasSetBonus(ItemSetRedemptionRegalia, 2), 10*core.CritRatingPerCritChance, 0)
}

// lengthenJudgement is T9's 2 piece (67190), SPELLMOD_DURATION +10 s on the Judgements of Light and Wisdom the
// paladin puts up.
func (paladin *Paladin) lengthenJudgement(sim *core.Simulation, aura *core.Aura) {
	if paladin.holyItems.t9Judgements {
		aura.AddDuration(sim, 10*time.Second)
	}
}

func (paladin *Paladin) glyphOfFlashOfLightCrit() float64 {
	return core.TernaryFloat64(paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfFlashOfLight), 5*core.CritRatingPerCritChance, 0)
}

// glyphOfHolyShockCooldown is 63224's -1 s.
func (paladin *Paladin) glyphOfHolyShockCooldown() time.Duration {
	return core.TernaryDuration(paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfHolyShock), time.Second, 0)
}

// glyphOfBeaconDuration is 63218's +30 s.
func (paladin *Paladin) glyphOfBeaconDuration() time.Duration {
	return core.TernaryDuration(paladin.HasMajorGlyph(proto.PaladinMajorGlyph_GlyphOfBeaconOfLight), 30*time.Second, 0)
}

// registerGlyphOfHolyLightHeal is 54968, which spell_pal_glyph_of_holy_light_dummy casts on the Holy Light's target
// for 10% of the heal. Its area search drops the target and keeps the five lowest in health percent. Class mask 0,
// ignore-caster-modifiers and no coefficient: no spell power, spell mod or healing done percent, and it can't crit.
func (paladin *Paladin) registerGlyphOfHolyLightHeal() {
	items := &paladin.holyItems
	units := paladin.Env.Raid.GetFirstNPlayersOrPets(math.MaxInt32)
	items.glyphHeal = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 54968},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete | core.SpellFlagIgnoreAttackerModifiers,
		// no spell_bonus_data row, and an area heal gets half an instant's 1.5 s
		HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(float32(750) / 3500),

		DamageMultiplier: 1,
		CritMultiplier:   1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, healed *core.Unit, spell *core.Spell) {
			items.glyphTargets = glyphOfHolyLightTargets(items.glyphTargets[:0], units, healed)
			for _, target := range items.glyphTargets {
				spell.CalcAndDealHealing(sim, target, items.glyphHealAmount, spell.OutcomeHealing)
			}
		},
	})
}

// glyphOfHolyLightTargets are the living raid units and pets but the healed one, the five lowest in health percent
// when more are left (a stable sort keeps raid order among equals).
func glyphOfHolyLightTargets(targets []*core.Unit, units []*core.Unit, healed *core.Unit) []*core.Unit {
	for _, unit := range units {
		if unit == healed || !unit.IsEnabled() || (unit.HasHealthBar() && unit.CurrentHealth() <= 0) {
			continue
		}
		targets = append(targets, unit)
	}
	if len(targets) > glyphOfHolyLightHealCount {
		slices.SortStableFunc(targets, func(a, b *core.Unit) int {
			return cmp.Compare(healthPct(a), healthPct(b))
		})
		targets = targets[:glyphOfHolyLightHealCount]
	}
	return targets
}

// healthPct is Acore::HealthPctOrderPred's float ratio, full for a unit with no health bar.
func healthPct(unit *core.Unit) float32 {
	if !unit.HasHealthBar() {
		return 1
	}
	cur, most := uint32(unit.CurrentHealth()), uint32(unit.MaxHealth())
	return float32(min(cur, most)) / float32(most)
}

// glyphOfHolyLight heals through 54968 for CalculatePct(heal, 10) of a Holy Light that landed. Nothing hurts the
// sim's dummies, so the heal itself stands in for the script's effective-heal branch.
func (paladin *Paladin) glyphOfHolyLight(sim *core.Simulation, target *core.Unit, heal float64) {
	items := &paladin.holyItems
	if items.glyphHeal == nil || heal <= 0 {
		return
	}
	items.glyphHealAmount = float64(int32(float32(heal) * 10 / 100))
	items.glyphHeal.Cast(sim, target)
}

// registerHolyMending is T8's 2 piece, spell_pal_t8_2p_bonus: a Holy Shock crit casts 64891 on its target, 15% of the
// heal over 3 ticks of int32(int32(heal × 15%) / 3). Family 10 with no mask and no coefficient: each tick takes
// only the healing done percents and the target's taken terms.
func (paladin *Paladin) registerHolyMending() {
	if !paladin.HasSetBonus(ItemSetAegisRegalia, 2) {
		return
	}
	items := &paladin.holyItems
	items.holyMending = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 64891},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,
		// no spell_bonus_data row: 9 s of 15, over its 3 ticks
		HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(9.0 / 15 / 3),

		DamageMultiplier: 1,
		CritMultiplier:   1,
		ThreatMultiplier: 1,

		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Holy Mending",
			},
			NumberOfTicks: 3,
			TickLength:    time.Second * 3,
			TicksCanCrit:  false,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = items.holyMendingAmount
				dot.SnapshotHotMultiplier(1)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Hot(target).Apply(sim)
			spell.LandOn(sim, target)
		},
	})
}

func (paladin *Paladin) holyMendingOnCrit(sim *core.Simulation, target *core.Unit, heal float64) {
	items := &paladin.holyItems
	if items.holyMending == nil || heal <= 0 {
		return
	}
	items.holyMendingAmount = float64(int32(float32(heal)*15/100) / 3)
	items.holyMending.Cast(sim, target)
}

// registerHoliness is T10's 4 piece: a Holy Shock heal puts 70757 on the paladin, 0.3 s off the next Holy Light's
// cast within 10 s. Its one charge goes on the hit of the Holy Light it sped up (PROC_ATTR_REQ_SPELLMOD).
func (paladin *Paladin) registerHoliness() {
	if !paladin.HasSetBonus(ItemSetLightswornGarb, 4) {
		return
	}
	items := &paladin.holyItems
	items.holiness = paladin.RegisterAura(core.Aura{
		Label:    "Holiness",
		ActionID: core.ActionID{SpellID: 70757},
		Duration: time.Second * 10,
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			items.holinessUsed = false
		},
	})
}

func (paladin *Paladin) holinessCastTime(cast *core.Cast) {
	items := &paladin.holyItems
	items.holinessUsed = items.holiness != nil && items.holiness.IsActive()
	if items.holinessUsed {
		cast.CastTime = max(0, cast.CastTime-300*time.Millisecond)
	}
}

// registerLibramProcs: Libram of Veracity gives 234 spell power for 15 s (67364) at 70% off a Holy Light, 8 s apart,
// and Libram of Blinding Light a stack of 85 spell power for 15 s, up to 3 (71192), off every Holy Shock heal.
func (paladin *Paladin) registerLibramProcs() {
	items := &paladin.holyItems
	switch paladin.Ranged().ID {
	case libramOfVeracity:
		items.veracityAura = paladin.NewTemporaryStatsAura("Holy Judgement", core.ActionID{SpellID: 67364}, stats.Stats{stats.SpellPower: 234}, time.Second*15)
		items.veracityIcd = core.Cooldown{Timer: paladin.NewTimer(), Duration: time.Second * 8}
	case libramOfBlindingLight:
		items.blessedAura = paladin.RegisterAura(core.Aura{
			Label:     "Blessed",
			ActionID:  core.ActionID{SpellID: 71192},
			Duration:  time.Second * 15,
			MaxStacks: 3,
			OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
				aura.Unit.AddStatDynamic(sim, stats.SpellPower, 85*float64(newStacks-oldStacks))
			},
		})
	}
}

// onHolyLight is what a Holy Light that landed sets off beyond the talents.
func (paladin *Paladin) onHolyLight(sim *core.Simulation, target *core.Unit, heal float64) {
	items := &paladin.holyItems
	paladin.glyphOfHolyLight(sim, target, heal)
	if items.holinessUsed {
		items.holinessUsed = false
		items.holiness.Deactivate(sim)
	}
	if items.veracityAura != nil && items.veracityIcd.IsReady(sim) && sim.RandomFloat("Libram of Veracity") < 0.7 {
		items.veracityIcd.Use(sim)
		items.veracityAura.Activate(sim)
	}
}

// onHolyShockHeal is what a Holy Shock heal that landed sets off beyond the talents.
func (paladin *Paladin) onHolyShockHeal(sim *core.Simulation, target *core.Unit, crit bool, heal float64) {
	items := &paladin.holyItems
	if items.holiness != nil {
		items.holiness.Activate(sim)
	}
	if items.blessedAura != nil {
		items.blessedAura.Activate(sim)
		items.blessedAura.AddStack(sim)
	}
	if crit {
		paladin.holyMendingOnCrit(sim, target, heal)
	}
}
