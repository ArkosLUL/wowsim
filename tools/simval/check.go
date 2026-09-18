package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/wowsims/wotlk/sim/core"
)

// Tolerances: a computed chance has to land on the server's basis point, a rolled
// rate within 5 standard errors of the threshold it was rolled against, and a
// multiplier within a tenth of a percent.
const (
	maxBPDiff     = 1
	maxZ          = 5.0
	maxMultiplier = 0.001
)

func itoa(v int32) string { return strconv.Itoa(int(v)) }

// Check is one comparison between the sim and the server, ready to print.
type Check struct {
	Name   string
	Detail string
	Passed bool
}

func pass(name, detail string) Check { return Check{Name: name, Detail: detail, Passed: true} }
func fail(name, detail string) Check { return Check{Name: name, Detail: detail} }

func checkBP(name string, got, want int32) Check {
	detail := fmt.Sprintf("%d bp, server %d", got, want)
	if diff := got - want; diff <= maxBPDiff && diff >= -maxBPDiff {
		return pass(name, detail)
	}
	return fail(name, detail)
}

func checkMultiplier(name string, got, want float64) Check {
	detail := fmt.Sprintf("%.6f, server %.6f", got, want)
	if math.Abs(got-want) <= maxMultiplier {
		return pass(name, detail)
	}
	return fail(name, detail)
}

// checkRate is a two-sided binomial test of a rolled count against the threshold
// the server rolled it with.
func checkRate(name string, count, iterations float64, chanceBP int32) Check {
	p := float64(chanceBP) / core.MaxRollBP
	observed := count / iterations
	stderr := math.Sqrt(max(p*(1-p), 1e-12) / iterations)
	z := (observed - p) / stderr

	detail := fmt.Sprintf("%.4f%% over %.0f rolls, expected %.4f%% (z %+.2f)", observed*100, iterations, p*100, z)
	if math.Abs(z) <= maxZ {
		return pass(name, detail)
	}
	return fail(name, detail)
}

// meleeInput rebuilds what the server fed its own table out of the two snapshots.
func meleeInput(rec record, attack attackSnapshot) core.MeleeTableInput {
	attacker, target := rec.Attacker, rec.Target

	return core.MeleeTableInput{
		AttackerLevel:    attacker.Level,
		AttackerSkill:    attack.WeaponSkillVsOther,
		AttackerMaxSkill: attacker.MaxSkillForOther,
		DefenderLevel:    target.LevelForOther,
		DefenderSkill:    target.DefenseSkill,
		DefenderMaxSkill: target.MaxSkillForOther,

		AttackerIsPlayerOrPet: attacker.IsPlayer || attacker.IsPet,
		DefenderIsPlayerOrPet: target.IsPlayer || target.IsPet,
		// The snapshot has no IsControlledByPlayer. A guardian attacker would need
		// it, but no guardian is four levels above the dummies, so none can crush.
		AttackerControlledByPlayer: attacker.IsPlayer || attacker.IsPet,
		DefenderIsPlayer:           target.IsPlayer,

		MissPct:  float32(target.attack(attack.Type).MissChanceTaken),
		DodgePct: float32(target.Defense.Dodge),
		ParryPct: float32(target.Defense.Parry),
		BlockPct: float32(target.Defense.Block),
		CritPct:  float32(attack.CritVsOther),

		HitPct:       float32(hitMod(attacker, attack.Type)),
		ExpertisePct: float32(attack.ExpertiseReduction),

		InFront:  target.OtherInFront,
		CanDodge: target.canDodge(),
		CanParry: target.canParry(),
		CanBlock: target.canBlock(),

		EnemyDodgeMultiplier: 1,
	}
}

func hitMod(unit unitSnapshot, attackType string) float64 {
	if attackType == "ranged" {
		return unit.HitMods.Ranged
	}
	return unit.HitMods.Melee
}

// CheckRecord runs every comparison a record supports.
func CheckRecord(rec record) []Check {
	switch rec.Command {
	case "melee", "taken":
		return checkWhite(rec)
	case "yellow", "spell":
		return checkSpell(rec)
	case "armor":
		return checkArmor(rec)
	case "info":
		return checkInfo(rec)
	default:
		return nil
	}
}

func checkWhite(rec record) []Check {
	var server whiteTable
	if err := json.Unmarshal(rec.Derived, &server); err != nil {
		return []Check{fail("white table", err.Error())}
	}

	in := meleeInput(rec, rec.Attacker.attack(rec.attackType()))
	table := core.WhiteMeleeTableBP(in)

	checks := []Check{
		checkBP("skill bonus", in.SkillBonusBP(), server.SkillBonus),
		checkBP("white miss", table.Miss, server.MissBp),
		checkBP("white dodge", table.Dodge, server.DodgeBp),
		checkBP("white parry", table.Parry, server.ParryBp),
		checkBP("white block", table.Block, server.BlockBp),
		checkBP("white glancing", table.Glance, server.GlancingBp),
		checkBP("white crushing", table.Crush, server.CrushingBp),
		checkBP("white crit", table.Crit, server.CritBp),
	}

	for _, rolled := range []struct {
		outcome  string
		chanceBP int32
	}{
		{"miss", table.Miss},
		{"dodge", table.Dodge},
		{"parry", table.Parry},
		{"block", table.Block},
		{"glancing", table.Glance},
		{"crushing", table.Crush},
		{"crit", table.Crit},
	} {
		if rolled.chanceBP <= 0 {
			continue
		}
		if counts, ok := rec.Outcomes[rolled.outcome]; ok {
			checks = append(checks, checkRate("rolled "+rolled.outcome, counts.Count, rec.Iterations, rolled.chanceBP))
		}
	}

	return checks
}

func checkSpell(rec record) []Check {
	var server spellDerived
	if err := json.Unmarshal(rec.Derived, &server); err != nil {
		return []Check{fail("spell table", err.Error())}
	}

	var checks []Check

	if server.Yellow != nil {
		in := meleeInput(rec, rec.Attacker.attack(rec.attackType()))
		opts := core.YellowOptions{
			Ranged:          rec.attackType() == "ranged",
			NoActiveDefense: server.Yellow.NoActiveDefense,
			BlockedInTable:  server.Yellow.CanBlock,
			NoDodge:         !server.Yellow.CanDodge,
			NoParry:         !server.Yellow.CanParry,
		}

		table := core.YellowMeleeTableBP(in, opts)
		checks = append(checks,
			checkBP("yellow miss", table.Miss, server.Yellow.MissBp),
			checkBP("yellow dodge", table.Dodge, server.Yellow.DodgeBp),
			checkBP("yellow parry", table.Parry, server.Yellow.ParryBp),
			checkBP("yellow block", table.Block, server.Yellow.BlockBp),
		)

		// isSpellBlocked is only rolled for physical damage, and only from the
		// front.
		var partial int32
		if rec.physical() && rec.Target.OtherInFront {
			partial = core.PartialBlockBP(
				float32(rec.Target.Defense.Block),
				core.MaxSkill(rec.Attacker.Level),
				core.MaxSkill(rec.Target.Level))
		}
		checks = append(checks, checkBP("partial block", partial, int32(server.Yellow.PartialBlockChance*100)))
		if rec.PartialBlock.Rolled && partial > 0 {
			checks = append(checks, checkRate("rolled partial block", rec.PartialBlock.Count, rec.Iterations, partial))
		}

		// critDone is the sheet crit, critTaken what the skill difference leaves
		// of it. Magic damage class spells get no suppression at all, which is why
		// this only runs on the weapon branch.
		suppression := core.MeleeCritSuppressionPct(rec.Attacker.MaxSkillForOther, rec.Target.DefenseSkill)
		checks = append(checks, checkMultiplier("crit suppression", suppression, server.CritDone-server.CritTaken))
	}

	if server.Magic != nil {
		levelDiff := rec.Target.LevelForOther - rec.Attacker.LevelForOther
		miss := core.SpellMissBP(levelDiff, float32(rec.Attacker.HitMods.Spell), rec.Target.IsPlayer)
		checks = append(checks, checkBP("spell miss threshold", miss, server.Magic.MissThreshold))

		if counts, ok := rec.HitResults["miss"]; ok && miss > 0 {
			checks = append(checks, checkRate("rolled spell miss", counts.Count, rec.Iterations, miss))
		}
	}

	// Physical spells never resist; the server still reports a number for them,
	// but it is the 75% cap read off armor.
	if !rec.physical() {
		checks = append(checks, checkResists(rec, server)...)
	}

	return checks
}

func checkResists(rec record, server spellDerived) []Check {
	// The server reports the chance both ways: without the spell, which is what
	// the partial resist buckets are built from, and with it, which is what a
	// binary spell folds into its hit roll instead.
	partial := core.EffectiveResistChance(0, 0, rec.Attacker.Level, rec.Target.Level, false)
	checks := []Check{checkMultiplier("average resist", partial, server.EffectiveResist)}

	if rec.Spell.Binary {
		binary := core.EffectiveResistChance(0, 0, rec.Attacker.Level, rec.Target.Level, true)
		checks = append(checks, checkMultiplier("binary resist", binary, server.EffectiveResistWithSpell))
	}

	if rec.Resists == nil {
		return checks
	}

	buckets := core.ResistBuckets(partial)
	for i, count := range rec.Resists.Buckets {
		if i >= len(buckets) || (count == 0 && buckets[i] == 0) {
			continue
		}
		checks = append(checks, checkRate(
			fmt.Sprintf("rolled %d%% resist", i*10), count, rec.Iterations, int32(buckets[i]*core.MaxRollBP)))
	}

	checks = append(checks, checkMultiplier("mean resisted", partial, rec.Resists.MeanResistedFraction))
	return checks
}

func checkArmor(rec record) []Check {
	// Battle Stance and the like add to what the rating gives, all of it capped by
	// the victim's level rather than the attacker's.
	var attacker core.Unit
	attacker.PseudoStats.BonusArmorPenPct = rec.Attacker.ArmorPenAuraPct
	attacker.PseudoStats.ArmorPenRatingPerPercent = core.CombatRatingBase[core.CRArmorPenetration]
	if class, ok := classes[rec.Attacker.Class]; ok && rec.Attacker.IsPlayer {
		attacker.PseudoStats.ArmorPenRatingPerPercent = core.RatingPerPercent(class.class, core.CRArmorPenetration)
	}
	armorPen, _ := rec.Attacker.rating("armorPenetration")

	var checks []Check
	for i, scenario := range rec.Scenarios {
		reducible := core.ArmorPenetrationCap(scenario.Armor, rec.Target.Level)
		effective := scenario.Armor - reducible*attacker.ArmorPenetrationPercentage(armorPen)
		got := core.ArmorMultiplier(effective, rec.Attacker.Level)
		checks = append(checks, checkMultiplier(
			fmt.Sprintf("armor scenario %d (%.0f armor)", i+1, scenario.Armor), got, scenario.Multiplier))
	}
	return checks
}
