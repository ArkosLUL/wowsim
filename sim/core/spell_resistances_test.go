package core

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func newResistSim() *Simulation {
	return NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{},
		Encounter:  &proto.Encounter{},
		Raid:       &proto.Raid{},
	})
}

// averageOf folds the bucket table back into a mean, which has to come out at the
// average resist the buckets were built from.
func averageOf(thresholds Thresholds) float64 {
	var previous, average float64
	for _, th := range thresholds {
		average += (th.cumulativeChance - previous) * 0.1 * float64(th.bracket)
		if th.cumulativeChance >= 1 {
			break
		}
		previous = th.cumulativeChance
	}
	return average
}

func Test_PartialResistsVsPlayer(t *testing.T) {
	attacker := &Unit{Type: EnemyUnit, Level: 83, IsWorldBoss: true, stats: stats.Stats{}}
	defender := &Unit{Type: PlayerUnit, Level: 80, stats: stats.Stats{}}

	attackTable := NewAttackTable(attacker, defender)
	sim := newResistSim()
	spell := &Spell{SpellSchool: SpellSchoolFire}

	for resist := 0; resist < 5_000; resist += 1 {
		defender.stats[stats.FireResistance] = float64(resist)

		averageResist := attackTable.AverageResist(spell)
		thresholds := partialResistRollThresholds(averageResist)

		// The attacker is the higher level here, so the defender gets no free
		// resistance, only what it is wearing.
		expectedAr := min(float64(resist)/(506.5+float64(resist)), 0.75)

		if math.Abs(averageOf(thresholds)-expectedAr) > 1e-9 {
			t.Errorf("resist = %d, thresholds = %s, resultingAr = %.2f%%, expectedAr = %.2f%%", resist, thresholds, averageOf(thresholds), expectedAr)
			return
		}

		const n = 1_000

		outcomes := make(map[HitOutcome]int, n)
		var totalDamage float64
		for iter := 0; iter < n; iter++ {
			result := SpellResult{
				Outcome: OutcomeHit,
				Damage:  1000,
			}

			result.applyResistances(sim, spell, false, attackTable)

			outcomes[result.Outcome]++
			totalDamage += result.Damage
		}

		if math.Abs(expectedAr-(1-totalDamage/float64(1000*n))) > 0.01 {
			t.Logf("after %d iterations, resist = %d, ar = %.2f%% vs. damage lost = %.2f%%, outcomes = %v\n", n, resist, expectedAr*100, 100-100*totalDamage/float64(1000*n), outcomes)
		}
	}
}

func Test_PartialResistsVsBoss(t *testing.T) {
	attacker := &Unit{Type: PlayerUnit, Level: 80, stats: stats.Stats{}}
	defender := &Unit{Type: EnemyUnit, Level: 83, IsWorldBoss: true, stats: stats.Stats{}}

	attackTable := NewAttackTable(attacker, defender)
	sim := newResistSim()
	spell := &Spell{SpellSchool: SpellSchoolNature}

	for resist := 0.0; resist < 50; resist += 0.01 {
		defender.stats[stats.NatureResistance] = resist

		averageResist := attackTable.AverageResist(spell)
		thresholds := partialResistRollThresholds(averageResist)

		// 5 free resistance per level of difference, which spell penetration
		// can't touch, over the level 80 caster's constant of 400.
		expectedAr := (resist + 15) / (415 + resist)

		if math.Abs(averageOf(thresholds)-expectedAr) > 1e-9 {
			t.Errorf("resist = %.2f, thresholds = %s, resultingAr = %.2f%%, expectedAr = %.2f%%", resist, thresholds, averageOf(thresholds), expectedAr)
			return
		}

		const n = 1_000

		outcomes := make(map[HitOutcome]int, n)
		var totalDamage float64
		for iter := 0; iter < n; iter++ {
			result := SpellResult{
				Outcome: OutcomeHit,
				Damage:  1000,
			}

			result.applyResistances(sim, spell, false, attackTable)

			outcomes[result.Outcome]++
			totalDamage += result.Damage
		}

		if math.Abs(expectedAr-(1-totalDamage/float64(1000*n))) > 0.01 {
			t.Logf("after %d iterations, resist = %.2f, ar = %.2f%% vs. damage lost = %.2f%%, outcomes = %v\n", n, resist, expectedAr*100, 100-100*totalDamage/float64(1000*n), outcomes)
		}
	}
}

// The numbers .simval spell 42842 reported against the boss dummy.
func Test_ResistsVsBossDummy(t *testing.T) {
	attacker := &Unit{Type: PlayerUnit, Level: 80, stats: stats.Stats{}}
	defender := &Unit{Type: EnemyUnit, Level: 83, IsWorldBoss: true, stats: stats.Stats{}}
	attackTable := NewAttackTable(attacker, defender)

	frostbolt := &Spell{SpellSchool: SpellSchoolFrost}
	if got, want := attackTable.AverageResist(frostbolt), 15.0/415.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("average resist = %.6f, want %.6f", got, want)
	}

	buckets := ResistBuckets(15.0 / 415.0)
	for bracket, want := range map[int]float64{0: 0.7289, 1: 0.1807, 2: 0.0904} {
		if math.Abs(buckets[bracket]-want) > 5e-5 {
			t.Errorf("%d%% resist bucket = %.4f, want %.4f", bracket*10, buckets[bracket], want)
		}
	}

	// A binary spell rolls its resist as part of the hit check, so nothing is
	// partially resisted and the level difference never enters.
	binary := &Spell{SpellSchool: SpellSchoolFrost, Flags: SpellFlagBinary}
	if got := binary.ResistanceMultiplier(newResistSim(), false, attackTable); got != 1 {
		t.Errorf("binary resistance multiplier = %.4f, want 1", got)
	}
	if got := EffectiveResistChance(0, 0, 80, 83, true); got != 0 {
		t.Errorf("binary resist chance = %.6f, want 0", got)
	}

	// Holy has no resistance stat but still takes the level difference.
	holy := &Spell{SpellSchool: SpellSchoolHoly}
	if got, want := attackTable.AverageResist(holy), 15.0/415.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("holy average resist = %.6f, want %.6f", got, want)
	}
}

// A spell of several schools meets the weakest of their resistances, and holy has
// none at all.
func TestMultiSchoolResistance(t *testing.T) {
	attacker := &Unit{Type: PlayerUnit, Level: 80}
	defender := &Unit{Type: EnemyUnit, Level: 83, IsWorldBoss: true}
	defender.stats[stats.FireResistance] = 100
	defender.stats[stats.FrostResistance] = 60
	attackTable := NewAttackTable(attacker, defender)

	frostfire := &Spell{SpellSchool: SpellSchoolFire | SpellSchoolFrost}
	// 60 frost and the 15 a level 83 target gets for free, over 400 more.
	if got, want := attackTable.AverageResist(frostfire), 75.0/475.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("frostfire average resist = %.6f, want %.6f", got, want)
	}

	holyfire := &Spell{SpellSchool: SpellSchoolFire | SpellSchoolHoly}
	if got, want := attackTable.AverageResist(holyfire), 15.0/415.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("fire and holy average resist = %.6f, want %.6f", got, want)
	}
}
