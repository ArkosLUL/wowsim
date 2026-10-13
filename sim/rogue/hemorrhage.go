package rogue

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (rogue *Rogue) registerHemorrhageSpell() {
	if !rogue.Talents.Hemorrhage {
		return
	}

	actionID := core.ActionID{SpellID: 48660}

	bonusDamage := 75.0
	if rogue.HasMajorGlyph(proto.RogueMajorGlyph_GlyphOfHemorrhage) {
		bonusDamage *= 1.4
	}

	// the stacks are the debuff's 10 charges
	hemoAuras := rogue.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return target.GetOrRegisterAura(core.Aura{
			Label:     "Hemorrhage",
			ActionID:  actionID,
			Duration:  time.Second * 15,
			MaxStacks: 10,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				aura.Unit.PseudoStats.BonusPhysicalDamageTaken += bonusDamage
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				aura.Unit.PseudoStats.BonusPhysicalDamageTaken -= bonusDamage
			},
			OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if spendsHemorrhageCharge(spell, result) {
					aura.RemoveStack(sim)
				}
			},
		})
	})

	daggerPct := rogue.daggerPct(core.MainHand)

	rogue.Hemorrhage = rogue.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | SpellFlagBuilder | SpellFlagColdBlooded | core.SpellFlagAPL,

		EnergyCost: core.EnergyCostOptions{
			Cost:       35,
			FlatMod:    -float64(rogue.Talents.SlaughterFromTheShadows),
			Multiplier: rogue.cheaperComboMoves(),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},

		BonusCritRating: core.TernaryFloat64(rogue.HasSetBonus(Tier9, 4), 5*core.CritRatingPerCritChance, 0),

		// Surprise Attacks' classMask reaches Hemorrhage too (Player::ApplySpellMod), same as Backstab
		// and Sinister Strike.
		DamageMultiplier: spellModDamage(
			0.02*float64(rogue.Talents.FindWeakness),
			core.TernaryFloat64(rogue.Talents.SurpriseAttacks, 0.1, 0),
			core.TernaryFloat64(rogue.HasSetBonus(Tier6, 4), 0.06, 0),
		),
		CritMultiplier:   rogue.MeleeCritMultiplier(true),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, WeaponPct: 1.1},
		Mods:   []core.SpellMod{rogue.sinisterCallingMod()},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			baseDamage := core.ApplyPct(normalizedStrike(sim, spell, &spell.Direct, true), daggerPct)

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				rogue.AddComboPoints(sim, 1, spell.ComboPointMetrics())
				hemoAura := hemoAuras.Get(target)
				hemoAura.Activate(sim)
				hemoAura.SetStacks(sim, 10)
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}

const attr3NotAProc = 0x00000200

// spendsHemorrhageCharge is the debuff's TAKEN proc flags: any hit on the target that lands, damaging or
// not, from a swing, a cast or a triggered spell with SPELL_ATTR3_NOT_A_PROC. Poisons are triggered without it.
func spendsHemorrhageCharge(spell *core.Spell, result *core.SpellResult) bool {
	if !result.Landed() {
		return false
	}
	if spell.ProcMask.Matches(core.ProcMaskWhiteHit) || spell.Flags.Matches(core.SpellFlagAPL) {
		return true
	}
	ss := spell.ServerSpell()
	return ss != nil && ss.Attributes[3]&attr3NotAProc != 0
}
