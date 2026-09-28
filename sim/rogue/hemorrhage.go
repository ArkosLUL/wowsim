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

	var numPlayers int
	for _, u := range rogue.Env.Raid.AllUnits {
		if u.Type == core.PlayerUnit {
			numPlayers++
		}
	}

	var hemoAuras core.AuraArray

	// Hemo debuff disabled except in raid sim
	// in a raid environment each melee will get very little debuffs, which is hard to model
	if numPlayers >= 2 {
		bonusDamage := 75.0
		if rogue.HasMajorGlyph(proto.RogueMajorGlyph_GlyphOfHemorrhage) {
			bonusDamage *= 1.4
		}

		hemoAuras = rogue.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
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
					if spell.SpellSchool != core.SpellSchoolPhysical {
						return
					}
					if !result.Landed() || result.Damage == 0 {
						return
					}

					aura.RemoveStack(sim)
				},
			})
		})
	}

	daggerPct := rogue.daggerPct(core.MainHand)

	rogue.Hemorrhage = rogue.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | SpellFlagBuilder | SpellFlagColdBlooded | core.SpellFlagAPL,

		EnergyCost: core.EnergyCostOptions{
			Cost:   rogue.costModifier(35 - float64(rogue.Talents.SlaughterFromTheShadows)),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},

		BonusCritRating: core.TernaryFloat64(rogue.HasSetBonus(Tier9, 4), 5*core.CritRatingPerCritChance, 0) +
			[]float64{0, 2, 4, 6}[rogue.Talents.TurnTheTables]*core.CritRatingPerCritChance,

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
			baseDamage := normalizedStrike(sim, spell, &spell.Direct, true) * daggerPct

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				rogue.AddComboPoints(sim, 1, spell.ComboPointMetrics())
				if len(hemoAuras) > 0 {
					hemoAura := hemoAuras.Get(target)
					hemoAura.Activate(sim)
					hemoAura.SetStacks(sim, 10)
				}
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}
