package deathknight

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (dk *Deathknight) registerVampiricBloodSpell() {
	if !dk.Talents.VampiricBlood {
		return
	}

	actionID := core.ActionID{SpellID: 55233}
	healthMetrics := dk.NewHealthMetrics(actionID)

	cdTimer := dk.NewTimer()
	cd := time.Minute*1 - dk.thassariansPlateCooldownReduction(dk.VampiricBlood)

	var bonusHealth float64
	dk.VampiricBloodAura = dk.RegisterAura(core.Aura{
		Label:    "Vampiric Blood",
		ActionID: actionID,
		Duration: time.Second*10 + core.TernaryDuration(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfVampiricBlood), 5*time.Second, 0),
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			// spell_dk_vampiric_blood makes the aura's 15 a percent of max health
			bonusHealth = dk.CountPctFromMaxHealth(15)
			dk.GainBonusHealth(sim, bonusHealth, healthMetrics)
			dk.AddHealingTakenPct(35)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			dk.LoseBonusHealth(sim, bonusHealth)
			dk.RemoveHealingTakenPct(35)
		},
	})

	dk.VampiricBlood = dk.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		RuneCost: core.RuneCostOptions{
			BloodRuneCost:  1,
			RunicPowerGain: 10,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    cdTimer,
				Duration: cd,
			},
			IgnoreHaste: true,
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			dk.bladeBarrierCastPhase(sim)
			dk.VampiricBloodAura.Activate(sim)
		},
	})

	if !dk.Inputs.IsDps {
		dk.AddMajorCooldown(core.MajorCooldown{
			Spell: dk.VampiricBlood,
			Type:  core.CooldownTypeSurvival,
		})
	}
}
