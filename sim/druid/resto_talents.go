package druid

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// livingSeedHitsPerMinute stands in for the hits a seeded unit takes when the raid has no incoming damage: a boss
// swing every 2 s, as the shaman page's Earth Shield rate.
const livingSeedHitsPerMinute = 30

// registerLivingSeed is spell_dru_living_seed: a crit of Healing Touch, Regrowth's heal, Nourish or Swiftmend has
// the rank's chance to cast 48504 on its target for 30% of the heal. The seed has one charge, which the target's
// next hit taken spends to cast 48503 for that amount (spell_dru_living_seed_proc).
func (druid *Druid) registerLivingSeed() {
	if druid.Talents.LivingSeed == 0 {
		return
	}
	chance := core.ServerProcFor([]int32{0, 48496, 48499, 48500}[druid.Talents.LivingSeed]).Chance
	amounts := make([]float64, len(druid.Env.AllUnits))

	druid.LivingSeedHeal = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48503},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		// SPELL_ATTR3_IGNORE_CASTER_MODIFIERS, and its family 0 takes no druid spell mod
		Flags: core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete | core.SpellFlagIgnoreAttackerModifiers,

		DamageMultiplier: 1,
		// SPELL_ATTR1_NO_THREAT
		ThreatMultiplier: 0,
		// no spell_bonus_data row
		HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(0),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// SPELL_ATTR2_CANT_CRIT
			spell.CalcAndDealHealing(sim, target, amounts[target.UnitIndex], spell.OutcomeHealing)
		},
	})

	var seeds core.AuraArray
	seeds = druid.NewAllyAuraArray(func(unit *core.Unit) *core.Aura {
		hits := core.NewHitTakenStandIn(livingSeedHitsPerMinute, func(sim *core.Simulation) {
			druid.LivingSeedHeal.Cast(sim, unit)
			seeds.Get(unit).Deactivate(sim)
		})
		return unit.RegisterAura(core.Aura{
			Label:    "Living Seed " + druid.Label,
			ActionID: core.ActionID{SpellID: 48504},
			Duration: time.Second * 15,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				if !druid.Env.Raid.HasIncomingDamage() {
					hits.Start(sim)
				}
			},
			OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, _ *core.Spell, result *core.SpellResult) {
				if result.Damage > 0 && druid.Env.Raid.HasIncomingDamage() {
					druid.LivingSeedHeal.Cast(sim, unit)
					aura.Deactivate(sim)
				}
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				hits.Stop(sim)
			},
		})
	})

	druid.LivingSeed = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48504},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,

		DamageMultiplier: 1,
		// SPELL_ATTR1_NO_THREAT
		ThreatMultiplier: 0,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// a new seed takes the old one's place, amount and all (Aura::TryRefreshStackOrCreate)
			seeds.Get(target).Activate(sim)
			druid.landHot(sim, spell, target)
		},
	})

	plants := core.FilterSlice([]*DruidSpell{druid.HealingTouch, druid.Regrowth, druid.Nourish, druid.Swiftmend},
		func(spell *DruidSpell) bool { return spell != nil })

	druid.RegisterAura(core.Aura{
		Label:    "Living Seed Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnHealDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.DidCrit() || !isAnyOf(spell, plants) || !sim.Proc(chance, "Living Seed") {
				return
			}
			// CalculatePct(heal, 30) in float32, the heal overhealing and all
			amounts[result.Target.UnitIndex] = float64(uint32(float32(float32(result.Damage)*30) / 100))
			druid.LivingSeed.Cast(sim, result.Target)
		},
	})
}

func isAnyOf(spell *core.Spell, spells []*DruidSpell) bool {
	for _, s := range spells {
		if s.IsEqual(spell) {
			return true
		}
	}
	return false
}

// revitalizeEnergize is one of spell_dru_revitalize's energizes, by the target's power type.
type revitalizeEnergize struct {
	actionID core.ActionID
	bar      core.PowerBarType
	has      func(unit *core.Unit) bool
	newGain  func(unit *core.Unit, actionID core.ActionID) *core.ResourceMetrics
	amount   func(unit *core.Unit) float64
	add      func(unit *core.Unit, sim *core.Simulation, amount float64, metrics *core.ResourceMetrics)
}

var revitalizeEnergizes = []revitalizeEnergize{
	{
		actionID: core.ActionID{SpellID: 48542},
		bar:      core.ManaBar,
		has:      (*core.Unit).HasManaBar,
		newGain:  (*core.Unit).NewManaMetrics,
		// Spell::EffectEnergize makes 48542's 1 a percent of max mana
		amount: func(unit *core.Unit) float64 { return float64(uint32(float32(unit.MaxMana()) / 100)) },
		add:    (*core.Unit).AddMana,
	},
	{
		actionID: core.ActionID{SpellID: 48541},
		bar:      core.RageBar,
		has:      (*core.Unit).HasRageBar,
		newGain:  (*core.Unit).NewRageMetrics,
		amount:   func(*core.Unit) float64 { return 4 },
		add:      (*core.Unit).AddRage,
	},
	{
		actionID: core.ActionID{SpellID: 48540},
		bar:      core.EnergyBar,
		has:      (*core.Unit).HasEnergyBar,
		newGain:  (*core.Unit).NewEnergyMetrics,
		amount:   func(*core.Unit) float64 { return 8 },
		add:      (*core.Unit).AddEnergy,
	},
	{
		actionID: core.ActionID{SpellID: 48543},
		bar:      core.RunicPower,
		has:      (*core.Unit).HasRunicPowerBar,
		newGain:  (*core.Unit).NewRunicPowerMetrics,
		amount:   func(*core.Unit) float64 { return 16 },
		add:      (*core.Unit).AddRunicPower,
	},
}

// applyRevitalize is spell_dru_revitalize: each tick of the druid's Rejuvenation or Wild Growth rolls the rank's
// 5% a point to energize its target by its power type. The energize makes the druid assist threat. T10's Blessing
// of the Eternal has Rejuvenation's family flag, so its ticks roll too.
func (druid *Druid) applyRevitalize() {
	if druid.Talents.Revitalize == 0 || (druid.Rejuvenation == nil && druid.WildGrowth == nil) {
		return
	}
	chance := 0.05 * float64(druid.Talents.Revitalize)

	gains := make([][]*core.ResourceMetrics, len(druid.Env.AllUnits))
	threat := make([]*core.ResourceMetrics, len(revitalizeEnergizes))
	for _, unit := range druid.Env.Raid.GetFirstNPlayersOrPets(math.MaxInt32) {
		gains[unit.UnitIndex] = make([]*core.ResourceMetrics, len(revitalizeEnergizes))
		for i, e := range revitalizeEnergizes {
			if !e.has(unit) {
				continue
			}
			gains[unit.UnitIndex][i] = e.newGain(unit, e.actionID)
			if unit == &druid.Unit {
				gains[unit.UnitIndex][i].EnergizeThreat(core.EnergizeBySpell)
			} else if threat[i] == nil {
				threat[i] = e.newGain(&druid.Unit, e.actionID).EnergizeThreat(core.EnergizeBySpell)
			}
		}
	}

	druid.RegisterAura(core.Aura{
		Label:    "Revitalize",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnPeriodicHealDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			// the proc entry's spell type is heal only
			if !druid.Rejuvenation.IsEqual(spell) && !druid.WildGrowth.IsEqual(spell) &&
				!druid.BlessingOfTheEternal.IsEqual(spell) || result.Damage <= 0 {
				return
			}
			target := result.Target
			if gains[target.UnitIndex] == nil {
				return
			}
			for i, e := range revitalizeEnergizes {
				metrics := gains[target.UnitIndex][i]
				if metrics == nil || target.GetCurrentPowerBar() != e.bar {
					continue
				}
				if !sim.Proc(chance, "Revitalize") {
					return
				}
				amount := e.amount(target)
				e.add(target, sim, amount, metrics)
				if target != &druid.Unit {
					druid.AddEnergizeTickThreat(sim, threat[i], amount)
				}
				return
			}
		},
	})
}
