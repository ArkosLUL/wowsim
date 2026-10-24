package druid

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// registerTreeOfLife is the form (33891) and the aura it casts (34123, a raid area aura of 100 yd, so every
// player and pet): +6% healing taken, which doesn't stack with other healing taken bonuses.
func (druid *Druid) registerTreeOfLife() {
	if !druid.Talents.TreeOfLife {
		return
	}
	startsInTree := druid.StartingForm.Matches(Tree)
	actionID := core.ActionID{SpellID: 33891}

	var raidAuras []*core.Aura
	for _, unit := range druid.Env.Raid.AllUnits {
		// each unit puts its own on at reset: another unit's aura can't go on before that unit has reset
		raidAuras = append(raidAuras, unit.RegisterAura(core.Aura{
			Label:    "Tree of Life " + druid.Label,
			ActionID: core.ActionID{SpellID: 34123},
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				if startsInTree {
					aura.Activate(sim)
				}
			},
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				aura.Unit.AddHealingTakenPct(6)
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				aura.Unit.RemoveHealingTakenPct(6)
			},
		}))
	}

	// Master Shapeshifter's 48422, cast in the form: 2% a point healing done (aura 136), which the direct heals
	// carry in their own multiplier and the hots take at each tick
	shapeshifterPct := 2 * druid.Talents.MasterShapeshifter
	shapeshifter := 1 + float64(shapeshifterPct)/100
	directHeals := core.FilterSlice([]*DruidSpell{druid.HealingTouch, druid.Regrowth, druid.Nourish, druid.LifebloomBloom,
		druid.t8RejuvenationHeal},
		func(spell *DruidSpell) bool { return spell != nil })

	// Improved Tree of Life holds only in the form (its Stances): 5% a point of Spirit as healing power, and
	// 67/133/200% more armor from items
	var spiritDep *stats.StatDependency
	improvedArmor := 1 + []float64{0, 0.67, 1.33, 2}[druid.Talents.ImprovedTreeOfLife]
	if druid.Talents.ImprovedTreeOfLife > 0 {
		spiritDep = druid.NewDynamicStatDependency(stats.Spirit, stats.SpellPower, 0.05*float64(druid.Talents.ImprovedTreeOfLife))
	}

	druid.TreeOfLifeAura = druid.RegisterAura(core.Aura{
		Label:    "Tree of Life",
		ActionID: actionID,
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if !druid.InForm(Humanoid | Tree) {
				druid.ClearForm(sim)
			}
			druid.form = Tree

			druid.AddHealingDonePct(shapeshifterPct)
			for _, spell := range directHeals {
				spell.DamageMultiplier *= shapeshifter
			}
			if spiritDep != nil {
				druid.EnableDynamicStatDep(sim, spiritDep)
				druid.ApplyDynamicEquipScaling(sim, stats.Armor, improvedArmor)
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.form = Humanoid
			for _, raidAura := range raidAuras {
				raidAura.Deactivate(sim)
			}

			druid.RemoveHealingDonePct(shapeshifterPct)
			for _, spell := range directHeals {
				spell.DamageMultiplier /= shapeshifter
			}
			if spiritDep != nil {
				druid.DisableDynamicStatDep(sim, spiritDep)
				druid.RemoveDynamicEquipScaling(sim, stats.Armor, improvedArmor)
			}
		},
	})

	druid.TreeOfLife = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.13,
			Multiplier: 1 - 0.1*float64(druid.Talents.NaturalShapeshifter),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},
		ThreatMultiplier: 1,

		ExtraCastCondition: func(_ *core.Simulation, _ *core.Unit) bool {
			return !druid.InForm(Tree)
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			druid.TreeOfLifeAura.Activate(sim)
			for _, raidAura := range raidAuras {
				raidAura.Activate(sim)
			}
		},
	})
}
