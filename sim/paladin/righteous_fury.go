package paladin

import (
	"github.com/wowsims/wotlk/sim/core"
)

func (paladin *Paladin) ActivateRighteousFury() {
	dtmMul := 1 - 0.02*float64(paladin.Talents.ImprovedRighteousFury)

	paladin.RighteousFuryAura = paladin.RegisterAura(core.Aura{
		Label:    "Righteous Fury",
		ActionID: core.ActionID{SpellID: 25780},
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			paladin.PseudoStats.ThreatMultiplier *= 1.43
			paladin.PseudoStats.DamageTakenMultiplier *= dtmMul
			// a holy school aura, so a spell with holy among its schools takes it too
			paladin.AddThreatSchoolMultiplier(core.SpellSchoolHoly, 1.8)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			paladin.PseudoStats.ThreatMultiplier /= 1.43
			paladin.PseudoStats.DamageTakenMultiplier /= dtmMul
			paladin.RemoveThreatSchoolMultiplier(core.SpellSchoolHoly, 1.8)
		},
	})
}
