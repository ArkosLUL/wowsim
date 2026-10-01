package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// checkWhiteDamage compares the mean damage the server rolled for each white swing outcome of a naked
// player, bare-handed or holding a main-hand weapon, with what the sim's formulas give: its attack
// power, armor mitigation after armor penetration, glancing and crit multipliers and block value, over
// the server's integer rolls (Unit::CalculateDamage truncates the range, CalcArmorReducedDamage rounds up).
func checkWhiteDamage(rec record) []Check {
	attacker := rec.Attacker
	attack := attacker.attack("mainhand")
	if rec.attackType() != "mainhand" || !nakedButMainHand(attacker) ||
		attack.UnhastedMs == 0 || attack.HastedMs != attack.UnhastedMs {
		return nil
	}
	race, ok := races[attacker.Race]
	if !ok {
		return nil
	}

	var ratings stats.Stats
	ratings[stats.Defense], _ = attacker.rating("defenseSkill")
	ratings[stats.Dodge], _ = attacker.rating("dodge")
	character, err := nakedCharacter(race, attacker.Class, ratings)
	if err != nil {
		return []Check{fail("white damage", err.Error())}
	}
	if character.PseudoStats.DamageDealtMultiplier*character.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] != 1 {
		return nil
	}

	attackPower := character.GetStats()[stats.AttackPower]
	// a weapon with stats on it puts the server's attack power above the bare character's
	if attack.Weapon != nil && math.Floor(attackPower+1e-6) != attack.AttackPower {
		return nil
	}
	return whiteDamageChecks(rec, attackPower, character.DefaultMeleeCritMultiplier())
}

// nakedButMainHand is naked() with a main-hand weapon allowed, since the record carries its damage range
// and speed.
func nakedButMainHand(u unitSnapshot) bool {
	u.Attacks = slices.DeleteFunc(slices.Clone(u.Attacks), func(a attackSnapshot) bool { return a.Type == "mainhand" })
	return u.naked()
}

// whiteDamageChecks is the comparison for one attack power and crit multiplier, so a test can feed it
// the server's own for a record the sim has no character for.
func whiteDamageChecks(rec record, attackPower, critMultiplier float64) []Check {
	attacker := rec.Attacker
	attack := attacker.attack("mainhand")
	speed := float32(attack.UnhastedMs) / 1000
	bonus := float32(attackPower) / core.DefaultAttackPowerPerDPS * speed
	lo, hi := uint32(float32(attack.DamageMin)+bonus), uint32(float32(attack.DamageMax)+bonus)
	if hi < lo || hi-lo > 100000 {
		return nil
	}

	armor := float32(armorMultiplier(rec, rec.Target.Resistances[0]))
	glance := float32(core.GlancingMultiplier(attacker.Level, rec.Target.Level))
	crit := uint32(critMultiplier)
	var blockValue uint32
	if !rec.Target.IsPlayer {
		blockValue = uint32(core.CreatureBlockValue(rec.Target.Level, rec.Target.Stats.Strength))
	}

	outcomes := []struct {
		name   string
		damage func(armored uint32) uint32
	}{
		{"normal", func(d uint32) uint32 { return d }},
		{"crit", func(d uint32) uint32 { return d * crit }},
		{"glancing", func(d uint32) uint32 { return uint32(glance * float32(d)) }},
		{"block", func(d uint32) uint32 {
			if d < blockValue {
				return 0
			}
			return d - blockValue
		}},
	}

	var checks []Check
	for _, outcome := range outcomes {
		counts, ok := rec.Outcomes[outcome.name]
		if !ok || counts.Count < 30 {
			continue
		}

		var sum, sumSquares float64
		for roll := lo; roll <= hi; roll++ {
			damage := float64(outcome.damage(uint32(math.Ceil(float64(float32(roll) * armor)))))
			sum += damage
			sumSquares += damage * damage
		}
		n := float64(hi-lo) + 1
		mean := sum / n
		stderr := math.Sqrt(max(sumSquares/n-mean*mean, 0) / counts.Count)

		name := "white damage " + outcome.name
		detail := fmt.Sprintf("sim %.3f, server %.3f (stderr %.3f)", mean, counts.MeanDamage, stderr)
		if math.Abs(counts.MeanDamage-mean) <= max(maxZ*stderr, 1e-6) {
			checks = append(checks, pass(name, detail))
		} else {
			checks = append(checks, fail(name, detail))
		}
	}
	return checks
}
