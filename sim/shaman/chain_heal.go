package shaman

import (
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// standInDeficit is the health a target dummy without a health bar counts as missing in a chain heal's search.
// The server only jumps to hurt units, so with no incoming damage the dummies stand in for a hurt raid: a real
// deficit above it goes first. With incoming damage a dummy has a health bar and its own deficit counts.
const standInDeficit = 1

// targetDummies are the raid's target dummies, which the first chain heal looks up.
func (shaman *Shaman) targetDummies() map[*core.Unit]bool {
	if shaman.dummyUnits == nil {
		shaman.dummyUnits = make(map[*core.Unit]bool)
		for _, party := range shaman.Env.Raid.Parties {
			for _, player := range party.Players {
				if dummy, ok := player.(*core.TargetDummy); ok {
					shaman.dummyUnits[&dummy.Unit] = true
				}
			}
		}
	}
	return shaman.dummyUnits
}

// chainHealJumps is Spell::SearchChainTargets for a chain heal: each jump goes to the unit with the largest
// health deficit the chain hasn't healed yet, and the chain stops when no one else is hurt. The sim has no
// positions, so everyone counts as in jump range and sight.
func (shaman *Shaman) chainHealJumps(sim *core.Simulation, target *core.Unit, jumps []*core.Unit, maxJumps int) []*core.Unit {
	dummies := shaman.targetDummies()
	for len(jumps) < maxJumps {
		var next *core.Unit
		var maxDeficit uint32
		for _, unit := range sim.Raid.AllUnits {
			// a pet that's away isn't in the world to jump to
			if unit == target || !unit.IsEnabled() || slices.Contains(jumps, unit) {
				continue
			}
			var deficit uint32
			switch {
			case unit.HasHealthBar():
				cur, most := unit.CurrentHealth(), unit.MaxHealth()
				// SpellInfo::CheckTarget keeps the dead out of the search
				if cur >= most || cur <= 0 {
					continue
				}
				// whole health on the server
				deficit = uint32(most) - uint32(cur)
			case dummies[unit]:
				deficit = standInDeficit
			default:
				continue
			}
			// a tie keeps the unit found first
			if deficit > maxDeficit {
				next, maxDeficit = unit, deficit
			}
		}
		if next == nil {
			break
		}
		jumps = append(jumps, next)
	}
	return jumps
}

func (shaman *Shaman) registerChainHealSpell() {
	impShieldChance := shaman.impWaterShieldChance(30)

	hasGlyph := shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfChainHeal)

	numHits := min(core.TernaryInt32(hasGlyph, 4, 3), int32(len(shaman.Env.Raid.AllUnits)))
	bounceTargets := make([]*core.Unit, 0, numHits)
	results := make([]*core.SpellResult, 0, numHits)

	bonusHeal := 0 +
		core.TernaryFloat64(shaman.Ranged().ID == 28523, 87, 0) +
		core.TernaryFloat64(shaman.Ranged().ID == 38368, 102, 0) +
		// Steamcaller's Totem gives 243 on the server, where Classic gave 257 (effects_review.csv)
		core.TernaryFloat64(shaman.Ranged().ID == 45114, 243, 0)

	manaDiscount := 0 +
		core.TernaryFloat64(shaman.Ranged().ID == 40709, 78, 0)

	// T7's 4 piece is a damage spell mod (+5%), T8's takes 0.2 s off the cast and T9's adds 5% crit chance
	setHealing := core.TernaryFloat64(shaman.HasSetBonus(ItemSetEarthshatterRegalia, 4), .05, 0)
	castTime := time.Millisecond * time.Duration(core.TernaryInt32(shaman.HasSetBonus(ItemSetWorldbreakerRegalia, 4), 2300, 2500))
	setCrit := core.TernaryFloat64(shaman.HasSetBonus(ItemSetThrallsGarb, 4), 5, 0)

	shaman.ChainHeal = shaman.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 55459},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			FlatCost:   0.19*shaman.BaseMana - manaDiscount,
			Multiplier: 1 - .01*float64(shaman.Talents.TidalFocus),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: castTime,
			},
		},
		BonusCritRating: (float64(shaman.Talents.TidalMastery) + setCrit) * core.CritRatingPerCritChance,
		DamageMultiplier: spellModDamage(
			.02*float64(shaman.Talents.Purification),
			0.1*float64(shaman.Talents.ImprovedChainHeal),
			setHealing,
		),
		CritMultiplier:   shaman.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1 - (float64(shaman.Talents.HealingGrace) * 0.05),

		Direct: core.SpellEffect{Effect: 0, Min: 1055, Max: 1205, SP: 1.3428},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if shaman.onChainHealCast != nil {
				shaman.onChainHealCast(sim)
			}

			// all jumps are picked before the first heal lands
			bounceTargets = shaman.chainHealJumps(sim, target, bounceTargets[:0], int(numHits-1))

			// spell_sha_chain_heal looks for the caster's Riptide on the first target only, consumes it
			// there, and raises every heal of the cast by 25%
			riptideBonus := 1.0
			if shaman.Riptide != nil {
				if riptide := shaman.Riptide.Hot(target); riptide.IsActive() {
					riptide.Deactivate(sim)
					riptideBonus = 1.25
				}
			}

			// every target's crit is rolled at launch, before any of the heals lands and spends a Tidal Force charge
			results = results[:0]
			bounceCoeff := 1.0
			curTarget := target
			for hitIndex := 0; hitIndex <= len(bounceTargets); hitIndex++ {
				if hitIndex > 0 {
					curTarget = bounceTargets[hitIndex-1]
				}
				healPower := shaman.healingPower(spell, curTarget)
				baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, healPower) + bonusHeal
				baseHealing *= bounceCoeff * riptideBonus
				results = append(results, spell.CalcHealing(sim, curTarget, baseHealing, spell.OutcomeHealingCrit))
				bounceCoeff *= 0.6
			}

			for _, result := range results {
				crit := result.Outcome.Matches(core.OutcomeCrit)
				shaman.dealEarthlivingHeal(sim, spell, result)
				if crit {
					shaman.procImpWaterShield(sim, impShieldChance)
				}
				if shaman.Talents.TidalWaves > 0 {
					shaman.tidalWaveProc.Activate(sim)
					shaman.tidalWaveProc.SetStacks(sim, 2)
				}
			}
		},
	})
}
