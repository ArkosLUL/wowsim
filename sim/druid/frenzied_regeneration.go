package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (druid *Druid) registerFrenziedRegenerationCD() {
	actionID := core.ActionID{SpellID: 22842}
	healthMetrics := druid.HealThreat(druid.NewHealthMetrics(actionID), 22845)
	rageMetrics := druid.NewRageMetrics(actionID)

	cdTimer := druid.NewTimer()
	cd := time.Minute * 3
	// the glyph is a flat 20 on the spell's own healing taken effect, 0 without it
	healingTaken := core.TernaryInt32(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfFrenziedRegeneration), 20, 0)

	druid.FrenziedRegenerationAura = druid.RegisterAura(core.Aura{
		Label:    "Frenzied Regeneration",
		ActionID: actionID,
		Duration: druid.DurationThroughLastTick(time.Second * 10), // so the tenth tick still finds it on
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.AddHealingTakenPct(healingTaken)
		},

		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.RemoveHealingTakenPct(healingTaken)
		},
	})

	druid.FrenziedRegeneration = druid.RegisterSpell(Bear, core.SpellConfig{
		ActionID: actionID,
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    cdTimer,
				Duration: cd,
			},
			IgnoreHaste: true,
		},
		SpellSchool:      core.SpellSchoolPhysical,
		ProcMask:         core.ProcMaskEmpty,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{
				NumTicks: 10,
				Period:   time.Second * 1,
				OnAction: func(sim *core.Simulation) {
					rageDumped := min(druid.CurrentRage(), 10.0)
					healthGained := core.ApplyPct(druid.MaxHealth(), rageDumped*0.3/100)

					if druid.FrenziedRegenerationAura.IsActive() {
						druid.SpendRage(sim, rageDumped, rageMetrics)
						druid.GainHealing(sim, healthGained, healthMetrics)
					}
				},
			})

			druid.FrenziedRegenerationAura.Activate(sim)
		},
	})

	druid.AddMajorCooldown(core.MajorCooldown{
		Spell: druid.FrenziedRegeneration.Spell,
		Type:  core.CooldownTypeSurvival,
	})
}
