package deathknight

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (dk *Deathknight) registerIceboundFortitudeSpell() {
	actionID := core.ActionID{SpellID: 48792}
	cdTimer := dk.NewTimer()
	cd := time.Minute * 2

	hasGlyph := dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfIceboundFortitude)

	dmgTakenMult := 1.0
	dk.IceboundFortitudeAura = dk.RegisterAura(core.Aura{
		Label:    "Icebound Fortitude",
		ActionID: actionID,
		Duration: time.Second*12 + time.Second*2*time.Duration(float64(dk.Talents.GuileOfGorefiend)) + dk.scourgebornePlateIFDurationBonus(),
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			skill := core.MaxSkill(core.CharacterLevel) + aura.Unit.DefenseSkill()
			dmgTakenMult = iceboundFortitudeMultiplier(iceboundFortitudeReduction(skill, hasGlyph))
			aura.Unit.PseudoStats.DamageTakenMultiplier *= dmgTakenMult
		},

		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			if dmgTakenMult == iceboundFortitudeNoDamage {
				aura.Unit.PseudoStats.DamageTakenMultiplier *= iceboundFortitudeNoDamageUndo
			} else {
				aura.Unit.PseudoStats.DamageTakenMultiplier /= dmgTakenMult
			}
		},
	})

	dk.IceboundFortitude = dk.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		RuneCost: core.RuneCostOptions{
			RunicPowerCost: 20,
		},
		Cast: core.CastConfig{
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
			dk.bladeBarrierCastPhase(sim)
			dk.IceboundFortitudeAura.Activate(sim)
		},
	})

	if !dk.Inputs.IsDps {
		dk.AddMajorCooldown(core.MajorCooldown{
			Spell: dk.IceboundFortitude,
			Type:  core.CooldownTypeSurvival,
		})
	}
}

// At 100% or more, aura 87's AddPct takes Unit::GetTotalAuraMultiplier to 0 or below, and MeleeDamageBonusTaken and
// SpellDamageBonusTaken clamp the hit at 0. 2^-64 truncates every hit to 0 too, and unlike 0 it scales back out
// exactly, whatever else changed DamageTakenMultiplier in between.
const (
	iceboundFortitudeNoDamage     = 0x1p-64
	iceboundFortitudeNoDamageUndo = 0x1p64
)

// iceboundFortitudeReduction is spell_dk_icebound_fortitude's amount, as a positive percent: 30 plus a whole 0.15 per
// point of full defense skill over 400, skill bonuses included, at least 40 with the glyph. Nothing caps it, so it
// reaches 100 at 467 over 400.
func iceboundFortitudeReduction(skill int32, hasGlyph bool) int32 {
	reduction := 30 + int32(float64(max(0, skill-400))*0.15)
	if hasGlyph {
		reduction = max(reduction, 40)
	}
	return reduction
}

func iceboundFortitudeMultiplier(reduction int32) float64 {
	if reduction >= 100 {
		return iceboundFortitudeNoDamage
	}
	return 1 - float64(reduction)/100
}
