package core

import (
	"github.com/wowsims/wotlk/sim/core/stats"
)

// This function should do 3 things:
//  1. Set the Outcome of the hit effect.
//  2. Update spell outcome metrics.
//  3. Modify the damage if necessary.
type OutcomeApplier func(sim *Simulation, result *SpellResult, attackTable *AttackTable)

func (spell *Spell) OutcomeAlwaysHit(_ *Simulation, result *SpellResult, _ *AttackTable) {
	result.Outcome = OutcomeHit
	spell.SpellMetrics[result.Target.UnitIndex].Hits++
}
func (spell *Spell) OutcomeAlwaysMiss(_ *Simulation, result *SpellResult, _ *AttackTable) {
	result.Outcome = OutcomeMiss
	result.Damage = 0
	spell.SpellMetrics[result.Target.UnitIndex].Misses++
}

// A tick always hits, but we don't count them as hits in the metrics.
func (dot *Dot) OutcomeTick(_ *Simulation, result *SpellResult, _ *AttackTable) {
	result.Outcome = OutcomeHit
}

func (dot *Dot) OutcomeTickCounted(_ *Simulation, result *SpellResult, _ *AttackTable) {
	result.Outcome = OutcomeHit
	dot.Spell.SpellMetrics[result.Target.UnitIndex].Hits++
}

func (dot *Dot) OutcomeTickPhysicalCrit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	if dot.Spell.PhysicalCritCheck(sim, attackTable) {
		result.Outcome = OutcomeCrit
		result.Damage *= dot.Spell.CritMultiplier
	} else {
		result.Outcome = OutcomeHit
	}
}

func (dot *Dot) OutcomeSnapshotCrit(sim *Simulation, result *SpellResult, _ *AttackTable) {
	if dot.Spell.CritMultiplier == 0 {
		panic("Spell " + dot.Spell.ActionID.String() + " missing CritMultiplier")
	}
	if sim.RandomFloat("Snapshot Crit Roll") < dot.SnapshotCritChance {
		result.Outcome = OutcomeCrit
		result.Damage *= dot.Spell.CritMultiplier
		dot.Spell.SpellMetrics[result.Target.UnitIndex].Crits++
	} else {
		result.Outcome = OutcomeHit
		dot.Spell.SpellMetrics[result.Target.UnitIndex].Hits++
	}
}

func (dot *Dot) OutcomeMagicHitAndSnapshotCrit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	if dot.Spell.CritMultiplier == 0 {
		panic("Spell " + dot.Spell.ActionID.String() + " missing CritMultiplier")
	}
	if dot.Spell.MagicHitCheck(sim, attackTable) {
		if sim.RandomFloat("Snapshot Crit Roll") < dot.SnapshotCritChance {
			result.Outcome = OutcomeCrit
			result.Damage *= dot.Spell.CritMultiplier
			dot.Spell.SpellMetrics[result.Target.UnitIndex].Crits++
		} else {
			result.Outcome = OutcomeHit
			dot.Spell.SpellMetrics[result.Target.UnitIndex].Hits++
		}
	} else {
		result.Outcome = OutcomeMiss
		result.Damage = 0
		dot.Spell.SpellMetrics[result.Target.UnitIndex].Misses++
	}
}

func (spell *Spell) OutcomeMagicHitAndCrit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	if spell.CritMultiplier == 0 {
		panic("Spell " + spell.ActionID.String() + " missing CritMultiplier")
	}
	if spell.MagicHitCheck(sim, attackTable) {
		if spell.MagicCritCheck(sim, result.Target) {
			result.Outcome = OutcomeCrit
			result.Damage *= spell.CritMultiplier
			spell.SpellMetrics[result.Target.UnitIndex].Crits++
		} else {
			result.Outcome = OutcomeHit
			spell.SpellMetrics[result.Target.UnitIndex].Hits++
		}
	} else {
		result.Outcome = OutcomeMiss
		result.Damage = 0
		spell.SpellMetrics[result.Target.UnitIndex].Misses++
	}
}

func (spell *Spell) OutcomeMagicCrit(sim *Simulation, result *SpellResult, _ *AttackTable) {
	if spell.CritMultiplier == 0 {
		panic("Spell " + spell.ActionID.String() + " missing CritMultiplier")
	}
	if spell.MagicCritCheck(sim, result.Target) {
		result.Outcome = OutcomeCrit
		result.Damage *= spell.CritMultiplier
		spell.SpellMetrics[result.Target.UnitIndex].Crits++
	} else {
		result.Outcome = OutcomeHit
		spell.SpellMetrics[result.Target.UnitIndex].Hits++
	}
}

func (spell *Spell) OutcomeHealing(_ *Simulation, result *SpellResult, _ *AttackTable) {
	result.Outcome = OutcomeHit
	spell.SpellMetrics[result.Target.UnitIndex].Hits++
}

func (spell *Spell) OutcomeHealingCrit(sim *Simulation, result *SpellResult, _ *AttackTable) {
	if spell.CritMultiplier == 0 {
		panic("Spell " + spell.ActionID.String() + " missing CritMultiplier")
	}
	if spell.HealingCritCheck(sim) {
		result.Outcome = OutcomeCrit
		result.Damage *= spell.CritMultiplier
		spell.SpellMetrics[result.Target.UnitIndex].Crits++
	} else {
		result.Outcome = OutcomeHit
		spell.SpellMetrics[result.Target.UnitIndex].Hits++
	}
}

func (spell *Spell) OutcomeTickMagicHit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	if spell.MagicHitCheck(sim, attackTable) {
		result.Outcome = OutcomeHit
	} else {
		result.Outcome = OutcomeMiss
		result.Damage = 0
	}
}
func (spell *Spell) OutcomeMagicHit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	if spell.MagicHitCheck(sim, attackTable) {
		result.Outcome = OutcomeHit
		spell.SpellMetrics[result.Target.UnitIndex].Hits++
	} else {
		result.Outcome = OutcomeMiss
		result.Damage = 0
		spell.SpellMetrics[result.Target.UnitIndex].Misses++
	}
}

func (spell *Spell) OutcomeMeleeWhite(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	table := WhiteMeleeTableBP(spell.WhiteTableInput(attackTable))
	if !result.applyMeleeTable(spell, attackTable, table, sim.rollBP("White Hit Table"), false) {
		result.applyAttackTableHit(spell)
	}
}

func (spell *Spell) OutcomeMeleeSpecialHit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	spell.rollYellow(sim, result, attackTable, YellowOptions{}, false)
}

// Weapon abilities too: on this server a partial block is its own roll, so they
// crit and get blocked independently.
func (spell *Spell) OutcomeMeleeSpecialHitAndCrit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	spell.rollYellow(sim, result, attackTable, YellowOptions{}, true)
}

func (spell *Spell) OutcomeMeleeSpecialNoBlockDodgeParry(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	spell.rollYellow(sim, result, attackTable, YellowOptions{NoActiveDefense: true}, true)
}

func (spell *Spell) OutcomeMeleeSpecialNoBlockDodgeParryNoCrit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	spell.rollYellow(sim, result, attackTable, YellowOptions{NoActiveDefense: true}, false)
}

// For melee and ranged hits whose table roll happened elsewhere, like Mutilate's
// two strikes after the one roll for the cast.
func (spell *Spell) OutcomeMeleeSpecialCritOnly(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	result.Outcome = OutcomeHit
	result.rollCrit(sim, spell, attackTable)
	result.rollPartialBlock(sim, spell, attackTable)
	result.countLanded(spell)
}

func (spell *Spell) OutcomeRangedHit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	spell.rollYellow(sim, result, attackTable, YellowOptions{Ranged: true}, false)
}

func (spell *Spell) OutcomeRangedHitAndCrit(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	spell.rollYellow(sim, result, attackTable, YellowOptions{Ranged: true}, true)
}

func (dot *Dot) OutcomeRangedHitAndCritSnapshot(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	spell := dot.Spell
	table := YellowMeleeTableBP(spell.YellowTableInput(attackTable), YellowOptions{Ranged: true})
	if result.applyMeleeTable(spell, attackTable, table, sim.rollBP("White Hit Table"), true) {
		return
	}

	if spell.CritMultiplier == 0 {
		panic("Spell " + spell.ActionID.String() + " missing CritMultiplier")
	}
	result.Outcome = OutcomeHit
	if sim.RandomFloat("Physical Crit Roll") < dot.SnapshotCritChance {
		result.Outcome = OutcomeCrit
		result.Damage *= spell.CritMultiplier
	}
	result.rollPartialBlock(sim, spell, attackTable)
	result.countLanded(spell)
}

func (spell *Spell) OutcomeEnemyMeleeWhite(sim *Simulation, result *SpellResult, attackTable *AttackTable) {
	table := WhiteMeleeTableBP(spell.WhiteTableInput(attackTable))
	if !result.applyMeleeTable(spell, attackTable, table, sim.rollBP("Enemy White Hit Table"), false) {
		result.applyAttackTableHit(spell)
	}
}

func (spell *Spell) fixedCritCheck(sim *Simulation, critChance float64) bool {
	return sim.RandomFloat("Fixed Crit Roll") < critChance
}

// rollBP is urand(0, 10000): the melee table's roll, inclusive at both ends.
func (sim *Simulation) rollBP(label string) int32 {
	return int32(sim.RandomFloat(label) * (MaxRollBP + 1))
}

// rollYellow is MeleeSpellHitResult followed by the independent crit and block
// rolls CalculateSpellDamageTaken makes.
func (spell *Spell) rollYellow(sim *Simulation, result *SpellResult, attackTable *AttackTable, opts YellowOptions, canCrit bool) {
	opts.NoDodge = opts.NoDodge || spell.Flags.Matches(SpellFlagCannotBeDodged)
	opts.NoParry = opts.NoParry || spell.Flags.Matches(SpellFlagCannotBeParried)
	opts.NoActiveDefense = opts.NoActiveDefense || spell.Flags.Matches(SpellFlagNoActiveDefense)
	opts.BlockedInTable = opts.BlockedInTable || spell.Flags.Matches(SpellFlagCompletelyBlocked)

	table := YellowMeleeTableBP(spell.YellowTableInput(attackTable), opts)
	if result.applyMeleeTable(spell, attackTable, table, sim.rollBP("White Hit Table"), true) {
		return
	}

	result.Outcome = OutcomeHit
	if canCrit {
		result.rollCrit(sim, spell, attackTable)
	}
	if !opts.NoActiveDefense {
		result.rollPartialBlock(sim, spell, attackTable)
	}
	result.countLanded(spell)
}

// applyMeleeTable walks the table in the server's order, taking the first outcome
// the roll falls into. Reports whether it found one; a false leaves the result
// untouched and the plain hit to the caller, which may still owe it a crit roll.
//
// blockIsFull is for the yellow table, where a block only shows up for
// COMPLETELY_BLOCKED spells and stops them outright (SPELL_MISS_BLOCK). A white
// block is a partial one that still lands.
func (result *SpellResult) applyMeleeTable(spell *Spell, attackTable *AttackTable, table MeleeTableBP, roll int32, blockIsFull bool) bool {
	metrics := &spell.SpellMetrics[result.Target.UnitIndex]
	sum := table.Miss

	if roll < sum {
		result.Outcome = OutcomeMiss
		result.Damage = 0
		metrics.Misses++
		return true
	}

	if sum += table.Dodge; roll < sum {
		result.Outcome = OutcomeDodge
		result.Damage = 0
		metrics.Dodges++
		return true
	}

	if sum += table.Parry; roll < sum {
		result.Outcome = OutcomeParry
		result.Damage = 0
		metrics.Parries++
		return true
	}

	if sum += table.Block; roll < sum {
		if blockIsFull {
			result.Outcome = OutcomeBlock
			result.Damage = 0
		} else {
			result.Outcome = OutcomeHit | OutcomeBlock
			result.Damage = max(0, result.Damage-result.Target.BlockValue())
		}
		metrics.Blocks++
		return true
	}

	if sum += table.Glance; roll < sum {
		result.Outcome = OutcomeGlance
		result.Damage *= attackTable.GlanceMultiplier
		metrics.Glances++
		return true
	}

	if sum += table.Crush; roll < sum {
		result.Outcome = OutcomeCrush
		result.Damage *= 1.5
		metrics.Crushes++
		return true
	}

	if sum += table.Crit; roll < sum {
		if spell.CritMultiplier == 0 {
			panic("Spell " + spell.ActionID.String() + " missing CritMultiplier")
		}
		result.Outcome = OutcomeCrit
		result.Damage *= spell.CritMultiplier
		metrics.Crits++
		return true
	}

	return false
}

// rollPartialBlock is isSpellBlocked, which CalculateSpellDamageTaken rolls on
// physical damage after the crit, so a yellow hit can be both. A hit check with
// no damage (CalcOutcome) never gets that far on the server, so it can't be
// blocked either.
func (result *SpellResult) rollPartialBlock(sim *Simulation, spell *Spell, attackTable *AttackTable) {
	if result.Damage <= 0 || !spell.SpellSchool.Matches(SpellSchoolPhysical) || !spell.Unit.PseudoStats.InFrontOfTarget {
		return
	}
	chance := attackTable.partialBlockBP()
	if chance <= 0 || sim.rollBP("Partial Block") >= chance {
		return
	}

	result.Outcome |= OutcomeBlock
	result.Damage = max(0, result.Damage-result.Target.BlockValue())
}

// partialBlockBP is the creature's cached chance, or a player's live one.
func (at *AttackTable) partialBlockBP() int32 {
	defender := at.Defender
	if defender.Type == EnemyUnit {
		return at.PartialBlockBP
	}
	if !defender.PseudoStats.CanBlock || defender.PseudoStats.Stunned {
		return 0
	}
	return PartialBlockBP(defender.blockChancePct(), MaxSkill(at.Attacker.Level), MaxSkill(defender.Level))
}

// blockChancePct is a player's sheet block chance. Unlike dodge and parry, it
// takes every point of defense skill and none of it diminishes.
func (unit *Unit) blockChancePct() float32 {
	defenseSkill := int32(unit.stats[stats.Defense] / DefenseRatingPerDefense)
	return 5 + float32(unit.stats[stats.Block]/BlockRatingPerBlockChance) + float32(defenseSkill)*PercentPerSkillPoint
}

func (result *SpellResult) rollCrit(sim *Simulation, spell *Spell, attackTable *AttackTable) {
	if spell.CritMultiplier == 0 {
		panic("Spell " + spell.ActionID.String() + " missing CritMultiplier")
	}
	if spell.PhysicalCritCheck(sim, attackTable) {
		result.Outcome = OutcomeCrit
		result.Damage *= spell.CritMultiplier
	}
}

// countLanded counts a hit that got past the table exactly once, since the
// results page adds the counters up into attempts: a blocked crit is a crit, a
// blocked hit is a block.
func (result *SpellResult) countLanded(spell *Spell) {
	metrics := &spell.SpellMetrics[result.Target.UnitIndex]
	switch {
	case result.Outcome.Matches(OutcomeCrit):
		metrics.Crits++
	case result.Outcome.Matches(OutcomeBlock):
		metrics.Blocks++
	default:
		metrics.Hits++
	}
}

func (result *SpellResult) applyAttackTableHit(spell *Spell) {
	result.Outcome = OutcomeHit
	spell.SpellMetrics[result.Target.UnitIndex].Hits++
}

func (spell *Spell) WhiteTableInput(attackTable *AttackTable) MeleeTableInput {
	in := spell.YellowTableInput(attackTable)
	in.DualWield = spell.Unit.AutoAttacks.IsDualWielding && !spell.Unit.PseudoStats.DisableDWMissPenalty
	in.CritPct = float32(spell.PhysicalCritChance(attackTable) * 100)
	return in
}

func (spell *Spell) YellowTableInput(attackTable *AttackTable) MeleeTableInput {
	if spell.Unit.Type == EnemyUnit {
		return spell.enemyTableInput(attackTable, attackTable.Defender)
	}

	in := attackTable.meleeTableInput()
	in.InFront = spell.Unit.PseudoStats.InFrontOfTarget
	in.HitPct = float32(spell.PhysicalHitChance(attackTable) * 100)
	in.ExpertisePct = float32(spell.ExpertisePercentage() * 100)
	return in
}

// enemyTableInput is the creature-attacking-player side. The defender's avoidance
// comes off the character sheet, and only crit and crushing read defense skill;
// dodge, parry, block and miss all use the flat level*5.
func (spell *Spell) enemyTableInput(attackTable *AttackTable, defender *Unit) MeleeTableInput {
	in := attackTable.meleeTableInput()
	in.InFront = true
	in.DefenderSkill = attackTable.DefenderMaxSkill + int32(defender.stats[stats.Defense]/DefenseRatingPerDefense)

	stunned := defender.PseudoStats.Stunned
	in.CanDodge = !stunned
	in.CanParry = defender.PseudoStats.CanParry && !stunned
	in.CanBlock = defender.PseudoStats.CanBlock && !stunned

	in.MissPct = attackTable.BaseMissPct + float32((spell.Unit.PseudoStats.IncreasedMissChance+
		defender.GetDiminishedMissChance()+
		defender.PseudoStats.ReducedPhysicalHitTakenChance)*100)
	in.DodgePct = float32((defender.PseudoStats.BaseDodge + defender.GetDiminishedDodgeChance()) * 100)
	in.ParryPct = float32((defender.PseudoStats.BaseParry + defender.GetDiminishedParryChance()) * 100)
	in.BlockPct = defender.blockChancePct()

	return in
}

// enemyCritChance is a creature's crit against a player, white or yellow: its
// own rating, less the player's resilience and crit reductions, and the skill
// term with the player's full defense skill.
func (spell *Spell) enemyCritChance(attackTable *AttackTable) float64 {
	defender := attackTable.Defender
	defenseSkill := attackTable.DefenderMaxSkill + int32(defender.stats[stats.Defense]/DefenseRatingPerDefense)
	critChance := (spell.Unit.stats[stats.MeleeCrit]+spell.BonusCritRating)/(CritRatingPerCritChance*100) -
		defender.stats[stats.Resilience]/ResilienceRatingPerCritReductionChance/100 -
		defender.PseudoStats.ReducedCritTakenChance -
		MeleeCritSuppressionPct(attackTable.AttackerSkill, defenseSkill)/100
	return max(0, critChance)
}

func (spell *Spell) OutcomeExpectedTick(_ *Simulation, _ *SpellResult, _ *AttackTable) {
	// result.Damage *= 1
}
func (spell *Spell) OutcomeExpectedMagicAlwaysHit(_ *Simulation, _ *SpellResult, _ *AttackTable) {
	// result.Damage *= 1
}
func (spell *Spell) OutcomeExpectedMagicHit(_ *Simulation, result *SpellResult, attackTable *AttackTable) {
	averageMultiplier := 1.0
	averageMultiplier -= spell.SpellChanceToMiss(attackTable)

	result.Damage *= averageMultiplier
}

func (spell *Spell) OutcomeExpectedMagicCrit(_ *Simulation, result *SpellResult, _ *AttackTable) {
	if spell.CritMultiplier == 0 {
		panic("Spell " + spell.ActionID.String() + " missing CritMultiplier")
	}

	averageMultiplier := 1.0
	averageMultiplier += spell.SpellCritChance(result.Target) * (spell.CritMultiplier - 1)

	result.Damage *= averageMultiplier
}

func (spell *Spell) OutcomeExpectedMagicHitAndCrit(_ *Simulation, result *SpellResult, attackTable *AttackTable) {
	if spell.CritMultiplier == 0 {
		panic("Spell " + spell.ActionID.String() + " missing CritMultiplier")
	}

	averageMultiplier := 1.0
	averageMultiplier -= spell.SpellChanceToMiss(attackTable)
	averageMultiplier += averageMultiplier * spell.SpellCritChance(result.Target) * (spell.CritMultiplier - 1)

	result.Damage *= averageMultiplier
}

func (dot *Dot) OutcomeExpectedMagicSnapshotCrit(_ *Simulation, result *SpellResult, _ *AttackTable) {
	if dot.Spell.CritMultiplier == 0 {
		panic("Spell " + dot.Spell.ActionID.String() + " missing CritMultiplier")
	}

	averageMultiplier := 1.0
	averageMultiplier += dot.SnapshotCritChance * (dot.Spell.CritMultiplier - 1)

	result.Damage *= averageMultiplier
}
