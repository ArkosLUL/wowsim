package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// TyphoonDamageActionID is the spell Typhoon's cast (61384) triggers to deal its damage
// (spells_auto_gen.go: 61384 effect1 triggers 53227), and the id whose own binary flag governs its
// hit roll.
var TyphoonDamageActionID = core.ActionID{SpellID: 53227}

func (druid *Druid) registerTyphoonSpell() {
	if !druid.Talents.Typhoon {
		return
	}

	typhoonDamage := druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:     TyphoonDamageActionID,
		SpellSchool:  core.SpellSchoolNature,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        core.SpellFlagNoOnCastComplete | SpellFlagOmenTrigger,
		MissileSpeed: 30,

		DamageMultiplier: spellModDamage(0.15 * float64(druid.Talents.GaleWinds)),
		ThreatMultiplier: 1,
		// critCapable in the spelldump, so a crit pays the full spell multiplier, not 1x
		CritMultiplier: druid.BalanceCritMultiplier(),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := 1190 + 0.193*spell.SpellPower()
			baseDamage *= sim.Encounter.AOECapMultiplier()
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			}
		},
	})

	druid.Typhoon = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 61384},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.25,
			Multiplier: 1,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Second * (20 - core.TernaryDuration(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfMonsoon), 3, 0)),
			},
		},

		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			typhoonDamage.WaitTravelTime(sim, func(sim *core.Simulation) {
				typhoonDamage.Cast(sim, target)
			})
		},
	})
}
