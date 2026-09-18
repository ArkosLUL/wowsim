package core

import "math"

// Combat tables as AzerothCore rolls them: `Unit::RollMeleeOutcomeAgainst`,
// `MeleeSpellHitResult`, `isSpellBlocked`, `MagicSpellHitResult` and
// `GetEffectiveResistChance`.
//
// Everything here is integer basis points built from 32-bit floats, because that
// is what the server does. Both matter: the roll is urand(0, 10000) against int32
// chances, and a float64 version of the same arithmetic lands a point off on
// values like the 4.7% a boss misses a player for.

// The melee roll is urand(0, 10000), so 10001 outcomes including both ends.
const MaxRollBP = 10000

// Skill and defense both run 5 per level, and every table term turns one point of
// skill difference into 0.04%.
const (
	SkillPerLevel        = 5
	PercentPerSkillPoint = 0.04
)

// worldserver.conf WorldBossLevelDiff.
const WorldBossLevelDiff = 3

// worldserver.conf Rate.MissChanceMultiplier.*, truncated to int32 the way
// MagicSpellHitResult reads them.
const (
	SpellMissMultiplierVsCreature = 11
	SpellMissMultiplierVsPlayer   = 7
)

// chanceToBP truncates the way the int32 casts around the server's tables do.
func chanceToBP(pct float32) int32 {
	return int32(pct * 100)
}

// LevelForTarget is Creature::getLevelForTarget. A world boss reads three levels
// above whoever it is fighting whatever its template says, which is where the
// skill difference behind the whole table comes from. Resists and the armor
// penetration cap use the template level instead.
func LevelForTarget(level int32, isWorldBoss bool, viewerLevel int32) int32 {
	if !isWorldBoss {
		return level
	}
	return min(max(viewerLevel+WorldBossLevelDiff, 1), 255)
}

// MaxSkill is GetMaxSkillValueForLevel.
func MaxSkill(level int32) int32 {
	return level * SkillPerLevel
}

// MeleeTableInput is everything RollMeleeOutcomeAgainst and MeleeSpellHitResult
// read, with the chances as percentages the way the server holds them.
type MeleeTableInput struct {
	AttackerLevel    int32 // raw level, for glancing damage
	AttackerSkill    int32 // GetWeaponSkillValue(attType, defender)
	AttackerMaxSkill int32 // GetMaxSkillValueForLevel(defender)
	DefenderLevel    int32 // getLevelForTarget(attacker)
	DefenderSkill    int32 // GetDefenseSkillValue(attacker)
	DefenderMaxSkill int32 // GetMaxSkillValueForLevel(attacker)

	// Unit::IsPlayer() || IsPet() on each side: only those glance, and never
	// against each other. Guardians aren't pets, so they don't glance.
	AttackerIsPlayerOrPet bool
	DefenderIsPlayerOrPet bool

	// IsControlledByPlayer, guardians included: nothing a player controls crushes.
	AttackerControlledByPlayer bool

	// The miss skill term is scaled differently against a player.
	DefenderIsPlayer bool

	MissPct  float32 // GetUnitMissChance, before hit and the skill term
	DodgePct float32
	ParryPct float32
	BlockPct float32
	CritPct  float32

	HitPct       float32
	ExpertisePct float32

	DualWield bool
	InFront   bool

	CanDodge bool
	CanParry bool
	CanBlock bool

	// SPELL_AURA_MOD_COMBAT_RESULT_CHANCE for dodge, taken off the chance flat.
	DodgeReductionPct float32

	// SPELL_AURA_MOD_ENEMY_DODGE, 1 when nothing modifies it.
	EnemyDodgeMultiplier float32
}

// MeleeTableBP is one roll's table in the server's order. Each field is that
// outcome's own width in basis points; walking them in order and taking the first
// one where the roll falls under the running sum reproduces the server's roll.
// Widths can be negative, which the server also allows, and then that outcome
// simply shifts everything after it down.
type MeleeTableBP struct {
	Miss   int32
	Dodge  int32
	Parry  int32
	Block  int32
	Glance int32
	Crush  int32
	Crit   int32
}

// SkillBonusBP is the term every avoidance chance is shifted by, negative when
// the defender out-skills the attacker, which is why it reads as a bonus to the
// defender: +0.6% dodge, parry and block against a level 83 boss.
func (in MeleeTableInput) SkillBonusBP() int32 {
	return 4 * (in.AttackerSkill - in.DefenderMaxSkill)
}

// MeleeMissBP is MeleeSpellMissChance. Only white hits take the dual wield
// penalty, and an attacker who out-skills the defender pays 0.02 per point where
// a defender who out-skills the attacker collects 0.04.
func MeleeMissBP(in MeleeTableInput, isSpell bool) int32 {
	miss := in.MissPct
	if !isSpell && in.DualWield {
		miss += 19
	}

	diff := float32(in.DefenderMaxSkill - in.AttackerSkill)
	if in.DefenderIsPlayer {
		if diff > 0 {
			miss += diff * 0.04
		} else {
			miss += diff * 0.02
		}
	} else if diff > 10 {
		miss += 1 + (diff-10)*0.4
	} else {
		miss += diff * 0.1
	}

	miss -= in.HitPct

	if miss < 0 {
		return 0
	}
	if miss > 60 {
		miss = 60
	}
	return chanceToBP(miss)
}

// GlanceBP is capped at 40% and only players and their pets land one, against
// something above their own level that is neither.
func GlanceBP(in MeleeTableInput) int32 {
	if !in.AttackerIsPlayerOrPet || in.DefenderIsPlayerOrPet || in.AttackerLevel >= in.DefenderLevel {
		return 0
	}
	return min((10+in.DefenderSkill-min(in.AttackerSkill, in.AttackerMaxSkill))*100, 4000)
}

// GlancingMultiplier is 0.1 off per level, three levels deep at most, so 0.70
// against a level 83 boss. It reads the raw levels, not the boss flag's.
func GlancingMultiplier(attackerLevel int32, defenderLevel int32) float64 {
	return 1 - float64(min(max(defenderLevel-attackerLevel, 0), 3))*0.1
}

// CrushBP needs four levels and fifteen points of skill, so nothing a level 83
// boss does to a level 80 player crushes.
func CrushBP(in MeleeTableInput) int32 {
	if in.AttackerControlledByPlayer || in.AttackerLevel < in.DefenderLevel+4 {
		return 0
	}
	lacking := in.AttackerMaxSkill - min(in.DefenderSkill, in.DefenderMaxSkill)
	if lacking < 15 {
		return 0
	}
	return lacking*200 - 1500
}

// WhiteMeleeTableBP is RollMeleeOutcomeAgainst: miss, dodge, parry, block,
// glancing, crushing, crit.
//
// Dodge, parry and block each take the skill bonus only once the chance is still
// positive after expertise, so the last fraction of a percent vanishes early:
// white dodge against a boss is gone at 23.4 expertise, not 25.8.
func WhiteMeleeTableBP(in MeleeTableInput) MeleeTableBP {
	skillBonus := in.SkillBonusBP()
	expertise := chanceToBP(in.ExpertisePct)

	table := MeleeTableBP{Miss: MeleeMissBP(in, false)}

	if in.CanDodge {
		dodge := chanceToBP(in.DodgePct) - expertise - chanceToBP(in.DodgeReductionPct)
		dodge = int32(float32(dodge) * in.EnemyDodgeMultiplier)
		if dodge > 0 && dodge-skillBonus > 0 {
			table.Dodge = dodge - skillBonus
		}
	}

	if in.InFront {
		if in.CanParry {
			parry := chanceToBP(in.ParryPct) - expertise
			if parry > 0 && parry-skillBonus > 0 {
				table.Parry = parry - skillBonus
			}
		}
		if in.CanBlock {
			block := chanceToBP(in.BlockPct)
			if block > 0 && block-skillBonus > 0 {
				table.Block = block - skillBonus
			}
		}
	}

	table.Glance = GlanceBP(in)
	table.Crush = CrushBP(in)

	if crit := chanceToBP(in.CritPct); crit > 0 {
		table.Crit = crit
	}

	return table
}

// YellowOptions are the spell attributes MeleeSpellHitResult branches on.
type YellowOptions struct {
	Ranged bool // SPELL_DAMAGE_CLASS_RANGED: no dodge, no parry

	// SPELL_ATTR0_NO_ACTIVE_DEFENSE: can still miss, nothing else.
	NoActiveDefense bool

	// SPELL_ATTR3_COMPLETELY_BLOCKED without CU_DIRECT_DAMAGE, the only case where
	// block is in the table rather than its own roll.
	BlockedInTable bool

	NoDodge bool // SPELL_ATTR7_NO_ATTACK_DODGE
	NoParry bool // SPELL_ATTR7_NO_ATTACK_PARRY
}

// YellowMeleeTableBP is MeleeSpellHitResult. It differs from the white table in
// more than the missing dual wield penalty: the skill bonus goes in before
// expertise rather than after, so avoidance survives to the full 25.8 and 56
// expertise, and there is no glancing, crushing or crit here at all. Crit is its
// own roll, and so is block for everything but COMPLETELY_BLOCKED spells.
func YellowMeleeTableBP(in MeleeTableInput, opts YellowOptions) MeleeTableBP {
	table := MeleeTableBP{Miss: MeleeMissBP(in, true)}
	if opts.NoActiveDefense {
		return table
	}

	skillBonus := in.SkillBonusBP()
	expertise := chanceToBP(in.ExpertisePct)

	canDodge := in.CanDodge && !opts.NoDodge && !opts.Ranged
	canParry := in.InFront && in.CanParry && !opts.NoParry && !opts.Ranged
	canBlock := in.InFront && in.CanBlock && opts.BlockedInTable

	if canDodge {
		// Expertise comes off after the multiplier here and before it in the
		// white table, which is the other half of why the caps differ.
		dodge := chanceToBP(in.DodgePct) - skillBonus - chanceToBP(in.DodgeReductionPct)
		dodge = int32(float32(dodge) * in.EnemyDodgeMultiplier)
		table.Dodge = max(dodge-expertise, 0)
	}
	if canParry {
		table.Parry = max(chanceToBP(in.ParryPct)-skillBonus-expertise, 0)
	}
	if canBlock {
		table.Block = max(chanceToBP(in.BlockPct)-skillBonus, 0)
	}

	return table
}

// PartialBlockBP is isSpellBlocked, rolled on its own after the yellow table so a
// hit can crit and be blocked at the same time. It looks up both skills without a
// target, which flips the sign of the skill term against the table's: a boss
// blocks 4.4% of yellow hits where it blocks 5.6% of white ones.
//
// The server rolls this as a float percentage rather than basis points, so the
// bottom hundredth of a point can differ.
func PartialBlockBP(blockPct float32, attackerOwnSkill int32, defenderOwnMaxSkill int32) int32 {
	chance := blockPct + float32(attackerOwnSkill-defenderOwnMaxSkill)*PercentPerSkillPoint
	if chance < 0 {
		return 0
	}
	return chanceToBP(chance)
}

// MeleeCritSuppressionPct is the term GetUnitCriticalChance adds from the skill
// difference, and SpellDoneCritChance repeats for melee and ranged class spells.
// It is worth -0.6% against a level 83 boss, not retail's -4.8%. Spells of the
// magic damage class get nothing at all.
func MeleeCritSuppressionPct(attackerMaxSkill int32, defenderSkill int32) float64 {
	return float64(defenderSkill-attackerMaxSkill) * PercentPerSkillPoint
}

// SpellMissBP is MagicSpellHitResult's miss threshold. The server rolls
// irand(1, 10000) against it, so the real miss rate is a hundredth of a point
// under the threshold: 16.99% for the nominal 17%.
func SpellMissBP(levelDiff int32, hitPct float32, defenderIsPlayer bool) int32 {
	multiplier := int32(SpellMissMultiplierVsCreature)
	if defenderIsPlayer {
		multiplier = SpellMissMultiplierVsPlayer
	}

	var modHitChance int32
	if levelDiff < 3 {
		modHitChance = 96 - levelDiff
	} else {
		modHitChance = 94 - (levelDiff-2)*multiplier
	}

	hitChance := modHitChance*100 + int32(hitPct*100)
	if hitChance < 100 {
		hitChance = 100
	} else if hitChance > MaxRollBP {
		hitChance = MaxRollBP
	}
	return MaxRollBP - hitChance
}

// ResistanceConstant is the denominator GetEffectiveResistChance divides by,
// which comes from the caster's level: 400 at level 80.
func ResistanceConstant(casterLevel int32) float64 {
	level := float64(casterLevel)
	switch {
	case level > 60:
		return 150 + (level-60)*(level-67.5)
	case level > 20:
		return 50 + (level-20)*2.5
	default:
		return 50
	}
}

// EffectiveResistChance is Unit::GetEffectiveResistChance. Spell penetration only
// eats the target's own resistance, never the 5 per level a higher level target
// gets for free, and a binary spell skips the level part entirely. Against a level
// 83 boss with no resistance this is 15/415, so 3.6145%.
func EffectiveResistChance(resistance float64, spellPenetration float64, casterLevel int32, defenderLevel int32, binary bool) float64 {
	victimResistance := max(resistance-spellPenetration, 0)
	if !binary {
		victimResistance += max(float64(defenderLevel-casterLevel)*5, 0)
	}
	return min(victimResistance/(victimResistance+ResistanceConstant(casterLevel)), 0.75)
}

// ResistBuckets is CalcAbsorbResist's discrete table: index i is the chance of
// resisting i*10% of the damage.
func ResistBuckets(averageResist float64) [11]float64 {
	var buckets [11]float64
	for i := range buckets {
		buckets[i] = max(0.5-2.5*math.Abs(0.1*float64(i)-averageResist), 0)
	}

	if averageResist <= 0.1 {
		buckets[0] = 1 - 7.5*averageResist
		buckets[1] = 5 * averageResist
		buckets[2] = 2.5 * averageResist
	}

	return buckets
}

// ArmorMultiplier is CalcArmorReducedDamage's mitigation, capped at 75%.
func ArmorMultiplier(effectiveArmor float64, attackerLevel int32) float64 {
	levelModifier := float64(attackerLevel)
	if levelModifier > 59 {
		levelModifier += 4.5 * (levelModifier - 59)
	}

	reduction := 0.1 * effectiveArmor / (8.5*levelModifier + 40)
	reduction /= 1 + reduction
	return 1 - min(max(reduction, 0), 0.75)
}

// ArmorPenetrationCap is the most armor penetration can reach through, which the
// server takes from the victim's own level rather than the boss flag's.
func ArmorPenetrationCap(armor float64, defenderLevel int32) float64 {
	level := float64(defenderLevel)
	maxArmorPen := 400 + 85*level
	if defenderLevel >= 60 {
		maxArmorPen += 4.5 * 85 * (level - 59)
	}
	return min((armor+maxArmorPen)/3, armor)
}

// CreatureBlockValue is Creature::GetShieldBlockValue, 41 for a level 83 boss with
// no strength.
func CreatureBlockValue(level int32, strength float64) float64 {
	return float64(level/2) + math.Floor(strength/20)
}
