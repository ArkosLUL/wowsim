package rogue

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Glyph of Backstab (56800): a landed Backstab adds 2 s to the rogue's Rupture on the target, 6 s at most.
const (
	glyphOfBackstabStep = 2 * time.Second
	glyphOfBackstabCap  = 6 * time.Second
)

func (rogue *Rogue) registerBackstabSpell() {
	hasGlyph := rogue.HasMajorGlyph(proto.RogueMajorGlyph_GlyphOfBackstab)
	if hasGlyph && !rogue.Server().SpellTweaks.GlyphOfBackstabTicks {
		rogue.Env.RegisterPostFinalizeEffect(rogue.capRuptureTicks)
	}

	rogue.Backstab = rogue.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48657},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | SpellFlagBuilder | SpellFlagColdBlooded | core.SpellFlagAPL,

		EnergyCost: core.EnergyCostOptions{
			Cost:       60,
			FlatMod:    -4 * float64(rogue.Talents.SlaughterFromTheShadows),
			Multiplier: rogue.cheaperComboMoves(),
			Refund:     0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return !rogue.PseudoStats.InFrontOfTarget && rogue.HasDagger(core.MainHand)
		},

		BonusCritRating: core.TernaryFloat64(rogue.HasSetBonus(Tier9, 4), 5*core.CritRatingPerCritChance, 0) +
			10*core.CritRatingPerCritChance*float64(rogue.Talents.PuncturingWounds),
		// Each of these is its own SPELLMOD_DAMAGE (Player::ApplySpellMod multiplies them, it doesn't
		// add them).
		DamageMultiplier: spellModDamage(
			0.02*float64(rogue.Talents.FindWeakness),
			0.1*float64(rogue.Talents.Opportunity),
			0.03*float64(rogue.Talents.Aggression),
			0.05*float64(rogue.Talents.BladeTwisting),
			core.TernaryFloat64(rogue.Talents.SurpriseAttacks, 0.1, 0),
			core.TernaryFloat64(rogue.HasSetBonus(Tier6, 4), 0.06, 0),
		),
		CritMultiplier:   rogue.MeleeCritMultiplier(true),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 310, Max: 310, WeaponPct: 1.5},
		Mods:   []core.SpellMod{rogue.sinisterCallingMod()},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			baseDamage := normalizedStrike(sim, spell, &spell.Direct, true)

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				rogue.AddComboPoints(sim, 1, spell.ComboPointMetrics())
				if hasGlyph {
					rogue.extendRupture(sim, target)
				}
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}

// extendRupture is Glyph of Backstab's proc on a landed Backstab.
func (rogue *Rogue) extendRupture(sim *core.Simulation, target *core.Unit) {
	dot := rogue.Rupture.Dot(target)
	if !dot.IsActive() {
		return
	}
	if rogue.Server().SpellTweaks.GlyphOfBackstabTicks {
		// mod-spell-tweaks raises the max duration too, so the added seconds tick, at the interval Rupture
		// went on with. A recast starts the 6 s over: rupture.go keeps the cast's tick count in MaxStacks.
		if dot.NumberOfTicks-dot.MaxStacks >= int32(glyphOfBackstabCap/glyphOfBackstabStep) {
			return
		}
		// NumberOfTicks counts 2 s intervals, the glyph's step
		dot.NumberOfTicks++
		dot.Duration += glyphOfBackstabStep
	} else if dot.ExpiresAt() >= dot.StartedAt()+dot.Duration+5*time.Second {
		// core's script lets Rupture end up to 5 s past its apply time plus max duration: three Backstabs
		return
	}
	dot.UpdateExpires(sim.NextServerTick(dot.ExpiresAt() + glyphOfBackstabStep))
}

// capRuptureTicks is for core's Glyph of Backstab, which only adds to the time left: a dot stops at
// max duration / amplitude ticks (AuraEffect::GetTotalTicks), so the longer Rupture deals no more.
func (rogue *Rogue) capRuptureTicks() {
	for _, target := range rogue.Env.Encounter.TargetUnits {
		dot := rogue.Rupture.Dot(target)
		tick := dot.OnTick
		dot.OnTick = func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
			if dot.TickCount <= dot.TotalTicks() {
				tick(sim, target, dot)
			}
		}
	}
}
