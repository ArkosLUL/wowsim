package shaman

import (
	"math"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// spell_sha_lightning_overload casts these with SPELLVALUE_BASE_POINT0 set from the main hit, and the bolt
// then goes through SpellDamageBonusDone, the target's taken percents and its own hit and crit rolls.
const (
	overloadBoltSpellID  int32 = 45284
	overloadChainSpellID int32 = 45297
)

// overloadBasePoints is the amount the script hands over: half of what the hit dealt, a crit's halved
// first, in integers.
func overloadBasePoints(result *core.SpellResult) int32 {
	damage := int32(result.Damage)
	if result.DidCrit() {
		damage /= 2
	}
	return damage / 2
}

type overloadBolt struct {
	// EffectBasePoints_1. The handed amount, less 1 (CalcBaseValue), stands in for it under Direct's level
	// scaling and die roll.
	basePoints   int32
	levelPenalty float32
}

func newOverloadBolt(spellID int32) overloadBolt {
	s := serverdata.SpellByID(spellID)
	return overloadBolt{basePoints: s.Effects[0].BasePoints, levelPenalty: levelPenalty(s)}
}

// levelPenalty is Unit::CalculateLevelPenalty for a level 80 player: below its MaxLevel a spell takes
// (SpellLevel + 6) / 80 of its spell power share, 3.75% less for each level under 20.
func levelPenalty(s *serverdata.Spell) float32 {
	if s.SpellLevel <= 0 || s.SpellLevel >= s.MaxLevel {
		return 1
	}
	var penalty float32
	if s.SpellLevel < 20 {
		penalty = float32(20-s.SpellLevel) * 3.75
	}
	factor := min((float32(s.SpellLevel)+6)/float32(serverdata.CasterLevel), 1)
	return factor + float32(factor*-penalty)/100
}

// calcDamage rolls one target's hit of an overload cast for amount. The bolts carry
// SPELL_ATTR6_IGNORE_CASTER_DAMAGE_MODIFIERS (SpellInfoCorrections), so of the done percents only the
// shaman's own (overloadDoneMultiplier) and the spell mods count, and no creature type spell power.
// bounce is the chain's falloff, 1 on the first target.
func (bolt overloadBolt) calcDamage(sim *core.Simulation, shaman *Shaman, spell *core.Spell, target *core.Unit, amount int32, relicSpellPower float64, bounce float64) *core.SpellResult {
	value := float64(amount-1-bolt.basePoints) + spell.Direct.Roll(sim)
	power := math.Trunc(shaman.GetStat(stats.SpellPower)+spell.BonusSpellPower) + relicSpellPower
	bonus := float64(int32(float32(float32(power)*float32(spell.Direct.SP)) * bolt.levelPenalty))
	done := math.Trunc((value + bonus) * bounce * shaman.overloadDoneMultiplier * spell.DamageMultiplier * spell.DamageMultiplierAdditive)
	return spell.CalcDamageDone(sim, target, done, spell.OutcomeMagicHitAndCrit)
}
