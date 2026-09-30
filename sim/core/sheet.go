package core

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// PlayerSheet is one raider's character sheet plus what the encounter's bosses can do to them.
type PlayerSheet struct {
	// Final stats with gear, buffs and talents, as ComputeStats reports them to the UI.
	FinalStats stats.Stats
	// The worst chance any enemy has to crit them with a melee swing; 0 means crit immune.
	MeleeCritTakenChance float64
	// False when nothing in the encounter swings at them, which leaves the crit chance unknown
	// rather than zero. Targets only swing when a tank is assigned to them.
	MeleeAttacker bool

	// The chance a yellow (special) attack misses the encounter's first target, for whichever of the
	// rotation's attacks misses most, each with its own bonus hit rating (a ranged scope's, say). Ranged
	// attacks share the same hit rating and formula in this sim (WotLK has no separate ranged hit stat),
	// so this also reads a hunter's ranged miss chance. Just the stat's when the rotation casts none.
	PhysicalMissChance float64
	// The chance that target dodges a yellow melee attack, after its own avoidance and the player's
	// expertise. Ranged attacks can't be dodged.
	ExpertiseDodgeChance float64
	// The chance a damage spell misses that target, for whichever of the rotation's spells misses most,
	// each with its own bonus hit rating: Shadow Focus, Arcane Focus and Elemental Precision add theirs
	// per spell, not to the stat. Just the stat's when the rotation casts none. A binary spell's resist
	// share stays out, no hit rating removes it.
	SpellMissChance float64
	// The multiplier ApplyCastSpeed scales a spell's cast time and the GCD by.
	CastSpeed float64
	// The multiplier RangedSwingSpeed scales ranged attacks by, Steady Shot's cast time included.
	RangedSwingSpeed float64
}

// ComputePlayerSheet builds the raid and encounter the way ComputeStats does, then reads one
// player's sheet out of it. The crit chance comes from the enemy's own auto attack against that
// player, so it carries defense, resilience and every crit-taken reduction the sim models, rather
// than assuming a fixed 540 defense.
func ComputePlayerSheet(raidProto *proto.Raid, encounterProto *proto.Encounter, raidIndex int) (sheet PlayerSheet, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("computing raid index %d's sheet: %v", raidIndex, e)
		}
	}()
	if encounterProto == nil {
		encounterProto = &proto.Encounter{}
	}
	// false: NewEnvironment's own fake prepull tears down with Cleanup before returning, which reverts
	// permanent auras (Improved Faerie Fire and the like) along with everything else combat resets
	// between iterations. Resetting here instead, and never cleaning up, leaves them active for the
	// PhysicalMissChance, ExpertiseDodgeChance, SpellMissChance and RangedSwingSpeed reads below;
	// FillPlayerStats already ran inside NewEnvironment, so FinalStats and MeleeCritTakenChance read
	// exactly what they did before.
	env, raidStats, _ := NewEnvironment(raidProto, encounterProto, false)
	newSimWithEnv(env, &proto.SimOptions{Iterations: 1}).reset()

	parties := raidStats.GetParties()
	party, slot := raidIndex/5, raidIndex%5
	if raidIndex < 0 || party >= len(parties) || slot >= len(parties[party].GetPlayers()) {
		return sheet, fmt.Errorf("no stats for raid index %d", raidIndex)
	}
	sheet.FinalStats = stats.FromFloatArray(parties[party].GetPlayers()[slot].GetFinalStats().GetStats())

	var defender *Unit
	for _, unit := range env.Raid.AllPlayerUnits {
		if unit.Index == int32(raidIndex) {
			defender = unit
		}
	}
	if defender == nil {
		return sheet, fmt.Errorf("raid index %d is an empty slot", raidIndex)
	}
	sheet.CastSpeed = defender.CastSpeed
	sheet.RangedSwingSpeed = defender.RangedSwingSpeed()

	if boss := env.Encounter.Targets; len(boss) > 0 && int(boss[0].UnitIndex) < len(defender.AttackTables) {
		target := boss[0]
		at := defender.AttackTables[target.UnitIndex]
		in := at.meleeTableInput()
		in.InFront = defender.PseudoStats.InFrontOfTarget
		in.HitPct = float32((defender.stats[stats.MeleeHit] + target.PseudoStats.BonusMeleeHitRatingTaken) / MeleeHitRatingPerHitChance)
		in.ExpertisePct = float32(defender.stats[stats.Expertise] / ExpertisePerQuarterPercentReduction / 4)
		sheet.PhysicalMissChance = float64(MeleeMissBP(in, true)) / 10000
		sheet.ExpertiseDodgeChance = float64(YellowMeleeTableBP(in, YellowOptions{}).Dodge) / 10000

		spellHitPct := float32((defender.stats[stats.SpellHit] + target.PseudoStats.BonusSpellHitRatingTaken) / SpellHitRatingPerHitChance)
		missBP := SpellMissBP(at.DefenderLevel-at.AttackerLevel, spellHitPct, target.Type == PlayerUnit)
		sheet.SpellMissChance = float64(missBP) / 10000

		// each spell through the same path its own roll takes
		yellow, magic := rotationDamageSpells(defender)
		if len(yellow) > 0 {
			worst := int32(0)
			for _, spell := range yellow {
				worst = max(worst, MeleeMissBP(spell.YellowTableInput(at), true))
			}
			sheet.PhysicalMissChance = float64(worst) / 10000
		}
		if len(magic) > 0 {
			worst := int32(0)
			for _, spell := range magic {
				hitPct := float32(spell.SpellHitChance(at.Defender) * 100)
				worst = max(worst, SpellMissBP(at.DefenderLevel-at.AttackerLevel, hitPct, at.Defender.Type == PlayerUnit))
			}
			sheet.SpellMissChance = float64(worst) / 10000
		}
	}
	for _, target := range env.Encounter.Targets {
		auto := target.AutoAttacks.MHAuto()
		// CurrentTarget is the tank the encounter points this one at; everyone else is never swung at
		if auto == nil || target.CurrentTarget != defender || int(defender.UnitIndex) >= len(target.AttackTables) {
			continue
		}
		chance := auto.enemyCritChance(target.AttackTables[defender.UnitIndex])
		if !sheet.MeleeAttacker || chance > sheet.MeleeCritTakenChance {
			sheet.MeleeCritTakenChance = chance
		}
		sheet.MeleeAttacker = true
	}
	return sheet, nil
}

// rotationDamageSpells is what unit's APL casts, prepull included, that deals damage, by the hit it
// rolls: yellow for melee and ranged specials, spell hit for spell damage. Any other mask goes in
// both, since it doesn't say which: Serpent Sting rolls ranged hit on ProcMaskEmpty, without a
// scope's bonus. Summons, buffs and cooldowns deal none (a zero DamageMultiplier), so no hit they'd
// need counts.
func rotationDamageSpells(unit *Unit) (yellow, magic []*Spell) {
	if unit.Rotation == nil {
		return nil, nil
	}
	for _, action := range append(unit.Rotation.allAPLActions(), unit.Rotation.allPrepullActions()...) {
		var spell *Spell
		switch impl := action.impl.(type) {
		case *APLActionCastSpell:
			spell = impl.spell
		case *APLActionChannelSpell:
			spell = impl.spell
		case *APLActionMultidot:
			spell = impl.spell
		}
		if spell == nil || spell.DamageMultiplier == 0 || spell.Flags.Matches(SpellFlagHelpful) {
			continue
		}
		spellDamage := spell.ProcMask.Matches(ProcMaskSpellDamage)
		special := spell.ProcMask.Matches(ProcMaskMeleeOrRangedSpecial)
		if spellDamage || !special {
			magic = append(magic, spell)
		}
		if special || !spellDamage {
			yellow = append(yellow, spell)
		}
	}
	return yellow, magic
}
