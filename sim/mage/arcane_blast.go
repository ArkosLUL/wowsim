package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const ArcaneBlastBaseCastTime = time.Millisecond * 2500

func (mage *Mage) registerArcaneBlastSpell() {
	abAuraMultiplierPerStack := core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfArcaneBlast), .18, .15)
	hasteCost := mage.arcaneBlastHasteCost()
	// the cost percent the stacks add right now
	var stackCostPct int32
	// Aura::SetStackAmount works the cost effect out again on every stack gained and on a refresh at four,
	// so a haste change shows at the next Arcane Blast to land, never in between
	setStackCost := func(stacks int32) {
		pct := stacks * mage.arcaneBlastStackCostPct(hasteCost)
		mage.ArcaneBlast.CostMultiplier += float64(pct-stackCostPct) / 100
		stackCostPct = pct
	}
	mage.ArcaneBlastAura = mage.GetOrRegisterAura(core.Aura{
		Label:     "Arcane Blast",
		ActionID:  core.ActionID{SpellID: 36032},
		Duration:  time.Second * 6,
		MaxStacks: 4,
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
			oldMultiplier := 1 + float64(oldStacks)*abAuraMultiplierPerStack
			newMultiplier := 1 + float64(newStacks)*abAuraMultiplierPerStack
			mage.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexArcane] *= newMultiplier / oldMultiplier
			setStackCost(newStacks)
		},
	})

	actionID := core.ActionID{SpellID: 42897}

	mage.ArcaneBlast = mage.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | BarrageSpells | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.07,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: ArcaneBlastBaseCastTime,
			},
		},

		BonusHitRating: float64(mage.Talents.ArcaneFocus) * core.SpellHitRatingPerHitChance,
		BonusCritRating: 0 +
			float64(mage.Talents.Incineration)*2*core.CritRatingPerCritChance +
			core.TernaryFloat64(mage.HasSetBonus(ItemSetKhadgarsRegalia, 4), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplierAdditive: 1 +
			.02*float64(mage.Talents.SpellImpact),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 1185, Max: 1377, SP: 0.714},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 3 * mage.Talents.ArcaneEmpowerment},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())) * mage.tormentTheWeak(target)
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			mage.ArcaneBlastAura.Activate(sim)
			mage.ArcaneBlastAura.AddStack(sim)
			setStackCost(mage.ArcaneBlastAura.GetStacks())
		},
	})
}

// arcaneBlastHasteCost is whether spell_tweaks_arcane_blast_haste_cost scales this mage's Arcane Blast
// stacks, which it does with any rank of Netherwind Presence.
func (mage *Mage) arcaneBlastHasteCost() bool {
	return mage.Talents.NetherwindPresence > 0 && mage.Server().SpellTweaks.ArcaneBlastHasteCost
}

// arcaneBlastStackCostPct is what one Arcane Blast stack adds to the cost now: 36032's 175%, times the cast
// speed in whole percent when the tweak scales it. A slow changes nothing.
func (mage *Mage) arcaneBlastStackCostPct(hasteCost bool) int32 {
	const stock = 175
	if !hasteCost {
		return stock
	}
	// the server multiplies in float32 before it truncates
	return int32(stock * float32(min(mage.CastSpeed, 1)))
}
