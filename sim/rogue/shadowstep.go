package rogue

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (rogue *Rogue) registerShadowstepCD() {
	if !rogue.Talents.Shadowstep {
		return
	}

	actionID := core.ActionID{SpellID: 36554}
	baseCost := 10 - 5*float64(rogue.Talents.FilthyTricks)

	// classMask decode (spelldump 36563): its SPELLMOD_DAMAGE effect ([100794886,266,0]) reaches
	// Sinister Strike, Backstab, Ambush, Eviscerate, Hemorrhage, Ghostly Strike, Mutilate's main hand
	// (not its off hand) and Envenom. Its SPELLMOD_EFFECT1 effect ([1048832,0,0]) reaches only
	// Garrote's and Rupture's own family flags: their tick's base points, not their AP
	// (garrote.go/rupture.go read the aura directly for that). Both effects share one proc mask, so
	// any of these ten landing consumes the charge.
	var damageSpells []*core.Spell
	var affectedSpells []*core.Spell

	rogue.ShadowstepAura = rogue.RegisterAura(core.Aura{
		Label:    "Shadowstep",
		ActionID: actionID,
		Duration: time.Second * 10,
		OnInit: func(aura *core.Aura, sim *core.Simulation) {
			damageSpells = []*core.Spell{
				rogue.SinisterStrike, rogue.Backstab, rogue.Ambush, rogue.Eviscerate,
				rogue.Hemorrhage, rogue.GhostlyStrike, rogue.MutilateMH, rogue.Envenom,
			}
			affectedSpells = append(append([]*core.Spell{}, damageSpells...), rogue.Garrote, rogue.Rupture)
		},
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			// Damage of your next ability is increased by 20% and the threat caused is reduced by 50%.
			for _, spell := range damageSpells {
				if spell != nil {
					spell.DamageMultiplier *= 1.2
				}
			}
			for _, spell := range affectedSpells {
				if spell != nil {
					spell.ThreatMultiplier *= 0.5
				}
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range damageSpells {
				if spell != nil {
					spell.DamageMultiplier /= 1.2
				}
			}
			for _, spell := range affectedSpells {
				if spell != nil {
					spell.ThreatMultiplier /= 0.5
				}
			}
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			for _, affectedSpell := range affectedSpells {
				if spell == affectedSpell {
					aura.Deactivate(sim)
				}
			}
		},
	})

	rogue.Shadowstep = rogue.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagAPL,
		EnergyCost: core.EnergyCostOptions{
			Cost:   baseCost,
			Refund: 0,
		},

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				Cost: baseCost,
				GCD:  0,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    rogue.NewTimer(),
				Duration: time.Second * time.Duration(30-5*rogue.Talents.FilthyTricks),
			},
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			rogue.ShadowstepAura.Activate(sim)
		},
	})
}
