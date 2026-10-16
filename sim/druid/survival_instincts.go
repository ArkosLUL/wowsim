package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (druid *Druid) registerSurvivalInstinctsCD() {
	if !druid.Talents.SurvivalInstincts {
		return
	}

	actionID := core.ActionID{SpellID: 61336}
	healthMetrics := druid.NewHealthMetrics(actionID)

	cdTimer := druid.NewTimer()
	cd := time.Minute * 3
	healthPct := core.TernaryInt32(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfSurvivalInstincts), 45, 30)

	var bonusHealth float64
	druid.SurvivalInstinctsAura = druid.RegisterAura(core.Aura{
		Label:    "Survival Instincts",
		ActionID: actionID,
		Duration: time.Second*20 + core.TernaryDuration(druid.HasSetBonus(ItemSetNightsongBattlegear, 4), 8*time.Second, 0),
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			// spell_dru_survival_instincts_aura casts 50322 for that percent of max health
			bonusHealth = druid.CountPctFromMaxHealth(healthPct)
			druid.GainBonusHealth(sim, bonusHealth, healthMetrics)
		},

		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.LoseBonusHealth(sim, bonusHealth)
		},
	})

	druid.SurvivalInstincts = druid.RegisterSpell(Cat|Bear, core.SpellConfig{
		ActionID: actionID,
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    cdTimer,
				Duration: cd,
			},
		},
		SpellSchool:      core.SpellSchoolPhysical,
		ProcMask:         core.ProcMaskEmpty,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			druid.SurvivalInstinctsAura.Activate(sim)
		},
	})

	druid.AddMajorCooldown(core.MajorCooldown{
		Spell: druid.SurvivalInstincts.Spell,
		Type:  core.CooldownTypeSurvival,
	})
}
