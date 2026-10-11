package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (shaman *Shaman) registerFeralSpirit() {
	if !shaman.Talents.FeralSpirit {
		return
	}

	spiritWolvesActiveAura := shaman.RegisterAura(core.Aura{
		Label:    "Feral Spirit",
		ActionID: core.ActionID{SpellID: 51533},
		Duration: time.Second * 45,
	})

	shaman.FeralSpirit = shaman.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 51533},

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.12,
			Multiplier: 1 - 0.02*float64(shaman.Talents.MentalQuickness),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    shaman.NewTimer(),
				Duration: time.Minute * 3,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			shaman.SpiritWolves.EnableWithTimeout(sim)
			shaman.SpiritWolves.CancelGCDTimer(sim)

			// Add a dummy aura to show in metrics
			spiritWolvesActiveAura.Activate(sim)
		},
	})

	shaman.AddMajorCooldown(core.MajorCooldown{
		Spell: shaman.FeralSpirit,
		// Ahead of Bloodlust, which stock wolves only get if they're out for the cast. With
		// SpellTweaks.FeralSpiritHaste they follow the shaman's haste whenever they come out.
		Priority: core.CooldownPriorityDrums + 1000,
		Type:     core.CooldownTypeDPS,
	})
}
