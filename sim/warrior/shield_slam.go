package warrior

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warrior *Warrior) registerShieldSlamSpell() {
	gagOrderPct := 5 * int32(warrior.Talents.GagOrder)
	hasGlyph := warrior.HasMajorGlyph(proto.WarriorMajorGlyph_GlyphOfBlocking)
	var glyphOfBlockingAura *core.Aura = nil
	if hasGlyph {
		glyphOfBlockingAura = warrior.GetOrRegisterAura(core.Aura{
			Label:    "Glyph of Blocking",
			ActionID: core.ActionID{SpellID: 58397},
			Duration: 10 * time.Second,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				warrior.PseudoStats.BlockValueMultiplier += 0.1
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				warrior.PseudoStats.BlockValueMultiplier -= 0.1
			},
		})
	}

	warrior.ShieldSlam = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47488},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial, // TODO: Is this right?
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost:   20 - float64(warrior.Talents.FocusedRage),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: time.Second * 6,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warrior.PseudoStats.CanBlock
		},

		BonusCritRating: 5 * core.CritRatingPerCritChance * float64(warrior.Talents.CriticalBlock),
		DamageMultiplier: 1 +
			core.TernaryFloat64(warrior.HasSetBonus(ItemSetOnslaughtArmor, 4), .10, 0) +
			core.TernaryFloat64(warrior.HasSetBonus(ItemSetDreadnaughtPlate, 2), .10, 0) +
			core.TernaryFloat64(warrior.HasSetBonus(ItemSetYmirjarLordsPlate, 2), .20, 0), // TODO: All additive multipliers?
		CritMultiplier:   warrior.critMultiplier(mh),
		ThreatMultiplier: 1.3,
		FlatThreatBonus:  770,

		Direct: core.SpellEffect{Effect: 1, Min: 990, Max: 1040, SP: 1},
		Mods:   []core.SpellMod{{Op: core.SpellModEffect2, Pct: gagOrderPct}},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Spell::EffectSchoolDMG adds the block value, halved past 24.5 a level and capped at 34.5 a
			// level (both doubled under Shield Block), with the effect's mods, in whole points
			limit := core.TernaryFloat64(warrior.ShieldBlockAura.IsActive(), 2, 1)
			soft, hard := uint32(core.CharacterLevel*24.5*limit), uint32(core.CharacterLevel*34.5*limit)
			block := uint32(max(warrior.BlockValue(), 0))
			if block >= hard {
				block = (soft + hard) / 2
			} else if block > soft {
				block = soft + (block-soft)/2
			}

			baseDamage := spell.Direct.Roll(sim) + float64(int32(float32(block)*(1+float32(gagOrderPct)/100)))
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				if glyphOfBlockingAura != nil {
					glyphOfBlockingAura.Activate(sim)
				}
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}
