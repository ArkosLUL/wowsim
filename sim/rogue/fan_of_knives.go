package rogue

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

const FanOfKnivesSpellID int32 = 51723

// both hands' knives are missiles on the server: 51723 and its linked off-hand cast 52874
const fanOfKnivesMissileSpeed = 18

func (rogue *Rogue) makeFanOfKnivesWeaponHitSpell(isMH bool) *core.Spell {
	var procMask core.ProcMask
	var actionID core.ActionID
	multiplier := spellModDamage(
		0.02*float64(rogue.Talents.FindWeakness),
		core.TernaryFloat64(rogue.HasMajorGlyph(proto.RogueMajorGlyph_GlyphOfFanOfKnives), 0.2, 0.0),
	)
	direct := core.SpellEffect{Effect: 0, WeaponPct: 0.7}
	if isMH {
		actionID = core.ActionID{SpellID: FanOfKnivesSpellID}.WithTag(1)
		procMask = core.ProcMaskMeleeMHSpecial
	} else {
		actionID = core.ActionID{SpellID: FanOfKnivesSpellID}.WithTag(2)
		multiplier *= rogue.dwsMultiplier()
		procMask = core.ProcMaskMeleeOHSpecial
		direct.FromSpellID = 52874
	}

	return rogue.RegisterSpell(core.SpellConfig{
		ActionID:     actionID,
		SpellSchool:  core.SpellSchoolPhysical,
		ProcMask:     procMask,
		Flags:        core.SpellFlagMeleeMetrics | SpellFlagColdBlooded,
		MissileSpeed: fanOfKnivesMissileSpeed,

		DamageMultiplier: multiplier,
		CritMultiplier:   rogue.MeleeCritMultiplier(false),
		ThreatMultiplier: 1,

		Direct: direct,
	})
}

func (rogue *Rogue) registerFanOfKnives() {
	mhSpell := rogue.makeFanOfKnivesWeaponHitSpell(true)
	ohSpell := rogue.makeFanOfKnivesWeaponHitSpell(false)
	mhDaggerPct := rogue.daggerPct(core.MainHand)
	ohDaggerPct := rogue.daggerPct(core.OffHand)
	// 52874 needs an off-hand weapon, so its linked cast fails without one
	hasOH := rogue.HasOHWeapon()

	rogue.FanOfKnives = rogue.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: FanOfKnivesSpellID},
		SpellSchool:  core.SpellSchoolPhysical,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MissileSpeed: fanOfKnivesMissileSpeed,

		EnergyCost: core.EnergyCostOptions{
			Cost: 50,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},

		ApplyEffects: func(sim *core.Simulation, unit *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			// both hands roll at launch, main hand first since the server casts 52874 once 51723 has
			// launched, and land together in that order. Every target's at the same distance, so one wait
			targets := sim.Encounter.TargetUnits
			mhResults := make([]*core.SpellResult, len(targets))
			for i, aoeTarget := range targets {
				baseDamage := (mhSpell.Unit.MHWeaponDamage(sim, mhSpell.MeleeAttackPower()) + mhSpell.BonusWeaponDamage()) *
					mhSpell.Direct.WeaponPct * mhDaggerPct
				baseDamage *= sim.Encounter.AOECapMultiplier()
				mhResults[i] = mhSpell.CalcDamage(sim, aoeTarget, baseDamage, mhSpell.OutcomeMeleeSpecialHitAndCrit)
			}
			var ohResults []*core.SpellResult
			if hasOH {
				ohResults = make([]*core.SpellResult, len(targets))
				for i, aoeTarget := range targets {
					baseDamage := (ohSpell.Unit.OHWeaponDamage(sim, ohSpell.MeleeAttackPower()) + ohSpell.BonusWeaponDamage()) *
						ohSpell.Direct.WeaponPct * ohDaggerPct
					baseDamage *= sim.Encounter.AOECapMultiplier()
					ohResults[i] = ohSpell.CalcDamage(sim, aoeTarget, baseDamage, ohSpell.OutcomeMeleeSpecialHitAndCrit)
				}
			}
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				for _, result := range mhResults {
					mhSpell.DealDamage(sim, result)
				}
				for _, result := range ohResults {
					ohSpell.DealDamage(sim, result)
				}
			})
		},
	})
}
