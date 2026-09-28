package rogue

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (rogue *Rogue) registerKillingSpreeSpell() {
	// Each swing is its own server spell (57841/57842), so RegisterSpell finds its data by ActionID.
	// Both have empty family flags, so no classMask'd mod such as Find Weakness reaches them.
	mhWeaponSwing := rogue.GetOrRegisterSpell(core.SpellConfig{
		ActionID:         core.ActionID{SpellID: 57841},
		SpellSchool:      core.SpellSchoolPhysical,
		ProcMask:         core.ProcMaskMeleeMHSpecial,
		Flags:            core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,
		DamageMultiplier: 1,
		CritMultiplier:   rogue.MeleeCritMultiplier(false),
		ThreatMultiplier: 1,
		Direct:           core.SpellEffect{Effect: 0, WeaponPct: 1},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := normalizedStrike(sim, spell, &spell.Direct, true)
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialNoBlockDodgeParry)
		},
	})
	ohWeaponSwing := rogue.GetOrRegisterSpell(core.SpellConfig{
		ActionID:         core.ActionID{SpellID: 57842},
		SpellSchool:      core.SpellSchoolPhysical,
		ProcMask:         core.ProcMaskMeleeOHSpecial,
		Flags:            core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,
		DamageMultiplier: rogue.dwsMultiplier(),
		CritMultiplier:   rogue.MeleeCritMultiplier(false),
		ThreatMultiplier: 1,
		Direct:           core.SpellEffect{Effect: 0, WeaponPct: 1},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := normalizedStrike(sim, spell, &spell.Direct, false)
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialNoBlockDodgeParry)
		},
	})
	rogue.KillingSpreeAura = rogue.RegisterAura(core.Aura{
		Label:    "Killing Spree",
		ActionID: core.ActionID{SpellID: 51690},
		Duration: time.Second*2 + 1,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			rogue.SetGCDTimer(sim, core.NeverExpires)
			rogue.PseudoStats.DamageDealtMultiplier *= 1.2
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{
				Period:          time.Millisecond * 500,
				NumTicks:        5,
				TickImmediately: true,
				OnAction: func(s *core.Simulation) {
					targetCount := sim.GetNumTargets()
					target := rogue.CurrentTarget
					if targetCount > 1 {
						newUnitIndex := int32(math.Ceil(float64(targetCount)*sim.RandomFloat("Killing Spree"))) - 1
						target = sim.GetTargetUnit(newUnitIndex)
					}
					mhWeaponSwing.Cast(sim, target)
					ohWeaponSwing.Cast(sim, target)
				},
			})
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			rogue.SetGCDTimer(sim, sim.CurrentTime)
			rogue.PseudoStats.DamageDealtMultiplier /= 1.2
		},
	})
	killingSpreeSpell := rogue.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 51690},
		Flags:    core.SpellFlagAPL,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    rogue.NewTimer(),
				Duration: time.Minute*2 - core.TernaryDuration(rogue.HasMajorGlyph(proto.RogueMajorGlyph_GlyphOfKillingSpree), time.Second*45, 0),
			},
		},

		ApplyEffects: func(sim *core.Simulation, u *core.Unit, s2 *core.Spell) {
			rogue.BreakStealth(sim)
			rogue.KillingSpreeAura.Activate(sim)
		},
	})

	rogue.AddMajorCooldown(core.MajorCooldown{
		Spell:    killingSpreeSpell,
		Type:     core.CooldownTypeDPS,
		Priority: core.CooldownPriorityLow,
		ShouldActivate: func(sim *core.Simulation, c *core.Character) bool {
			if bf := rogue.GetMajorCooldown(BladeFlurryActionID); bf != nil && bf.IsReady(sim) {
				return false
			}
			if rogue.CurrentEnergy() > 60 || (rogue.CurrentEnergy() > 30 && rogue.AdrenalineRushAura.IsActive()) {
				return false
			}
			return true
		},
	})
}
