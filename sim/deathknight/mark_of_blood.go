package deathknight

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (dk *Deathknight) registerMarkOfBloodSpell() {
	if !dk.Talents.MarkOfBlood {
		return
	}

	actionID := core.ActionID{SpellID: 49005}
	cdTimer := dk.NewTimer()
	cd := time.Minute * 3

	marks := dk.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return core.MarkOfBloodAura(target, &dk.Unit)
	})
	dk.MarkOfBlood = dk.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		RuneCost: core.RuneCostOptions{
			BloodRuneCost: 1,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    cdTimer,
				Duration: cd,
			},
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			dk.bladeBarrierCastPhase(sim)
			marks.Get(target).Activate(sim)
			if dk.RuneWeapon != nil {
				dk.RuneWeapon.copyMarkOfBlood(sim, target)
			}
		},
	})

	if !dk.Inputs.IsDps {
		dk.AddMajorCooldown(core.MajorCooldown{
			Spell: dk.MarkOfBlood,
			Type:  core.CooldownTypeSurvival,
		})
	}
}
