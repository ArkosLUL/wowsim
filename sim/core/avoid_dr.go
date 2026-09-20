package core

import (
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Player avoidance as Player::UpdateDodgePercentage, UpdateParryPercentage and
// GetMissPercentageFromDefence compute it: a part that diminishes, through
//
//	nondiminishing + diminishing·cap / (diminishing + cap·k)
//
// with per-class caps and k, and a part that doesn't. Creatures, pets included, have no diminishing
// returns and just add their avoidance up. The server works in percent, so the helpers here do too.

// CreatureDodgeChance is what a non-boss creature dodges with, pets included
// (Unit::GetUnitDodgeChance). A world boss has its own, and the attack table holds both for enemies.
const CreatureDodgeChance = 0.05

type playerAvoidance struct {
	StatScaling
	baseAgility float64
}

func newPlayerAvoidance(scaling StatScaling, baseStats stats.Stats) *playerAvoidance {
	return &playerAvoidance{StatScaling: scaling, baseAgility: baseStats[stats.Agility]}
}

func diminish(nondiminishing, diminishing, capPct, k float64) float64 {
	return nondiminishing + diminishing*capPct/(diminishing+capPct*k)
}

// DefenseSkillFromRating is the whole skill points defense rating is worth. The server truncates the
// rating's bonus, which it computes in floats, before anything reads it.
func DefenseSkillFromRating(rating float64) int32 {
	return int32(float32(rating) * (float32(1) / float32(DefenseRatingPerDefense)))
}

func (unit *Unit) defenseRatingAvoidancePct() float64 {
	return float64(DefenseSkillFromRating(unit.stats[stats.Defense])) * PercentPerSkillPoint
}

// DodgeChance is the dodge chance creatures roll against.
func (unit *Unit) DodgeChance() float64 {
	fromRating := unit.stats[stats.Dodge]/DodgeRatingPerDodgeChance + unit.defenseRatingAvoidancePct()
	a := unit.playerAvoidance
	if a == nil {
		return unit.PseudoStats.BaseDodge + fromRating/100
	}

	// Only agility above the character's base diminishes.
	dodgePerAgility := 100 * a.MeleeCritPerAgility * a.CritToDodge
	nondiminishing := 100*(a.DodgeBase+unit.PseudoStats.BaseDodge) + a.baseAgility*dodgePerAgility
	diminishing := fromRating + (unit.stats[stats.Agility]-a.baseAgility)*dodgePerAgility
	return max(0, diminish(nondiminishing, diminishing, a.DodgeCap, a.DiminishingK)) / 100
}

// ParryChance is the parry chance creatures roll against, 0 for anyone who can't parry.
func (unit *Unit) ParryChance() float64 {
	fromRating := unit.stats[stats.Parry]/ParryRatingPerParryChance + unit.defenseRatingAvoidancePct()
	a := unit.playerAvoidance
	if a == nil {
		return unit.PseudoStats.BaseParry + fromRating/100
	}
	if !unit.PseudoStats.CanParry || a.ParryCap == 0 {
		return 0
	}

	nondiminishing := 5 + 100*unit.PseudoStats.BaseParry
	return max(0, diminish(nondiminishing, fromRating, a.ParryCap, a.DiminishingK)) / 100
}

// DefenseMissChance is what defense adds to the base 5% chance to be missed.
func (unit *Unit) DefenseMissChance() float64 {
	fromRating := unit.defenseRatingAvoidancePct()
	a := unit.playerAvoidance
	if a == nil {
		return fromRating / 100
	}
	return diminish(0, fromRating, a.MissCap, a.DiminishingK) / 100
}
