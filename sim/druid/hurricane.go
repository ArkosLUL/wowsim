package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (druid *Druid) registerHurricaneSpell() {
	druid.HurricaneTickSpell = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48466},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskProc,
		Flags:       SpellFlagOmenTrigger,
		// critCapable in the spelldump, so a crit pays the full spell multiplier, not 1x
		CritMultiplier: druid.BalanceCritMultiplier(),
		DamageMultiplier: spellModDamage(
			0.15*float64(druid.Talents.GaleWinds),
			0.01*float64(druid.Talents.Genesis),
		),
		ThreatMultiplier: 1,
		Direct:           core.SpellEffect{Effect: 0, Min: 451, Max: 451, SP: 0.12898},
		Mods: []core.SpellMod{
			// Glyph of Hurricane's -20 is meant for the slow, but its class mask takes this tick's damage too
			{Op: core.SpellModEffect1, Flat: core.TernaryInt32(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfHurricane), -20, 0)},
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			damage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			damage *= sim.Encounter.AOECapMultiplier()
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				spell.CalcAndDealDamage(sim, aoeTarget, damage, spell.OutcomeMagicHitAndCrit)
			}
		},
	})

	druid.Hurricane = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48467},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagChanneled | core.SpellFlagAPL,
		ManaCost: core.ManaCostOptions{
			BaseCost:   0.81,
			Multiplier: 1,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},
		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "Hurricane",
			},
			NumberOfTicks:       10,
			TickLength:          time.Second * 1,
			AffectedByCastSpeed: true,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				druid.HurricaneTickSpell.Cast(sim, target)
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			spell.AOEDot().Apply(sim)
		},
	})
}
