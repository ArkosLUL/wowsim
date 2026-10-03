package core

import (
	"fmt"
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func threatTestSim(enemies int) *Simulation {
	env := &Environment{}
	env.Encounter.Targets = make([]*Target, enemies)
	for i := range enemies {
		env.Encounter.TargetUnits = append(env.Encounter.TargetUnits,
			&Unit{Type: EnemyUnit, UnitIndex: int32(i), Label: fmt.Sprintf("enemy %d", i)})
	}
	return &Simulation{Environment: env}
}

// threatTestSpell is the server's spell on a player, set up the way RegisterSpell does it.
func threatTestSpell(t *testing.T, sim *Simulation, spellID int32, school SpellSchool) *Spell {
	t.Helper()
	s := serverdata.SpellByID(spellID)
	if s == nil {
		t.Fatalf("spell %d isn't in the server data", spellID)
	}
	unit := &Unit{Type: PlayerUnit, UnitIndex: int32(len(sim.Encounter.TargetUnits)), Env: sim.Environment, Label: "player"}
	unit.PseudoStats = stats.NewPseudoStats()
	spell := &Spell{
		ActionID: ActionID{SpellID: spellID}, Unit: unit, SpellSchool: school, ThreatMultiplier: 1, serverSpell: s,
		SpellMetrics: make([]SpellMetrics, len(sim.Encounter.TargetUnits)+1),
	}
	spell.setThreatFromServer(0, false)
	return spell
}

// threatTestHit works out a hit's threat and deals it, the way calcDamageInternal and DealDamage do.
func threatTestHit(spell *Spell, target *Unit, outcome HitOutcome, damage float64) {
	result := &SpellResult{Target: target, Outcome: outcome, Damage: damage}
	result.Threat = spell.ThreatFromDamage(outcome, damage) + spell.threatBonus(result)
	spell.SpellMetrics[target.UnitIndex].TotalThreat += result.Threat
}

func threatTestCast(sim *Simulation, spell *Spell, hits func()) {
	start := spell.openThreatCast(sim)
	hits()
	spell.closeThreatCast(sim, start)
}

// the rows are float32, like the server's
func threatNear(got, want float64) bool { return math.Abs(got-want) <= 1e-6*max(1, math.Abs(want)) }

func TestThreatFromDamage(t *testing.T) {
	sim := threatTestSim(1)
	// spell_threat gives Thunder Clap's first rank x1.85
	spell := threatTestSpell(t, sim, 47502, SpellSchoolPhysical)
	spell.ThreatMultiplier = 1.1
	spell.Unit.PseudoStats.ThreatMultiplier = 2.0735
	if got, want := spell.ThreatFromDamage(OutcomeHit, 1000), 1000*1.85*1.1*2.0735; !threatNear(got, want) {
		t.Errorf("threat %v, want %v", got, want)
	}
	if got := spell.ThreatFromDamage(OutcomeMiss, 1000); got != 0 {
		t.Errorf("a miss makes %v threat", got)
	}
}

func TestThreatSchoolAurasMultiplyOnce(t *testing.T) {
	unit := &Unit{Label: "mage"}
	unit.PseudoStats = stats.NewPseudoStats()
	unit.PseudoStats.ThreatMultiplier = 0.5
	unit.AddThreatSchoolMultiplier(SpellSchoolFire, 0.8)
	unit.AddThreatSchoolMultiplier(SpellSchoolArcane|SpellSchoolFire|SpellSchoolFrost, 0.9)

	for _, c := range []struct {
		school SpellSchool
		want   float64
	}{
		{SpellSchoolFire, 0.5 * 0.8 * 0.9},
		{SpellSchoolFrost, 0.5 * 0.9},
		// each aura once, however many of the spell's schools it covers
		{SpellSchoolFire | SpellSchoolFrost, 0.5 * 0.8 * 0.9},
		{SpellSchoolShadow, 0.5},
	} {
		if got := unit.ThreatSchoolMultiplier(c.school); !threatNear(got, c.want) {
			t.Errorf("school %v: x%v, want x%v", c.school, got, c.want)
		}
	}

	unit.RemoveThreatSchoolMultiplier(SpellSchoolFire, 0.8)
	if got := unit.ThreatSchoolMultiplier(SpellSchoolFire); !threatNear(got, 0.5*0.9) {
		t.Errorf("fire after removing its aura: x%v, want x%v", got, 0.5*0.9)
	}
	defer func() {
		if recover() == nil {
			t.Error("removing an aura that isn't there didn't panic")
		}
	}()
	unit.RemoveThreatSchoolMultiplier(SpellSchoolFire, 0.8)
}

func TestThreatBonus(t *testing.T) {
	for _, c := range []struct {
		name  string
		spell int32
		want  float64
	}{
		{"spell_threat's flat", 47450, 259},                                        // Heroic Strike
		{"spell_threat's attack power part, first rank's row", 47498, 0.05 * 3000}, // Devastate, row 20243
		{"the spell's level without a row", 47467, 77},                             // Sunder Armor, 7386's chain doesn't hold it
		{"a row with neither", 47502, 0},                                           // Thunder Clap
		{"SPELL_ATTR3_SUPPRESS_TARGET_PROCS", 47475, 0},                            // Slam's cast
		{"SPELL_ATTR1_NO_THREAT", 49895, 0},                                        // Death Coil's cast
		{"SPELL_ATTR0_CU_NO_INITIAL_THREAT", 59092, 0},                             // Dark Pact
	} {
		t.Run(c.name, func(t *testing.T) {
			sim := threatTestSim(1)
			spell := threatTestSpell(t, sim, c.spell, SpellSchoolPhysical)
			spell.Unit.stats[stats.AttackPower] = 3000
			// neither moves the bonus
			spell.ThreatMultiplier = 1.3
			spell.Unit.PseudoStats.ThreatMultiplier = 2.0735

			enemy := sim.Encounter.TargetUnits[0]
			threatTestCast(sim, spell, func() { threatTestHit(spell, enemy, OutcomeHit, 0) })
			if got := spell.SpellMetrics[enemy.UnitIndex].TotalThreat; !threatNear(got, c.want) {
				t.Errorf("bonus %v, want %v", got, c.want)
			}
		})
	}
}

func TestThreatBonusSplit(t *testing.T) {
	bonusOn := func(spell *Spell, sim *Simulation) []float64 {
		out := make([]float64, len(sim.Encounter.TargetUnits))
		for i, enemy := range sim.Encounter.TargetUnits {
			out[i] = spell.SpellMetrics[enemy.UnitIndex].TotalThreat
		}
		return out
	}

	t.Run("between the enemies the cast reached", func(t *testing.T) {
		sim := threatTestSim(3)
		cleave := threatTestSpell(t, sim, 47520, SpellSchoolPhysical) // 112
		enemies := sim.Encounter.TargetUnits
		threatTestCast(sim, cleave, func() {
			threatTestHit(cleave, enemies[0], OutcomeHit, 0)
			threatTestHit(cleave, enemies[1], OutcomeDodge, 0)
		})
		// the dodge still counts, it just gets no share
		if got := bonusOn(cleave, sim); !threatNear(got[0], 56) || got[1] != 0 || got[2] != 0 {
			t.Errorf("bonus %v, want [56 0 0]", got)
		}
	})

	t.Run("one share per enemy however often it's hit", func(t *testing.T) {
		sim := threatTestSim(2)
		spell := threatTestSpell(t, sim, 48638, SpellSchoolPhysical) // Sinister Strike, level 80
		enemies := sim.Encounter.TargetUnits
		threatTestCast(sim, spell, func() {
			threatTestHit(spell, enemies[0], OutcomeMiss, 0)
			threatTestHit(spell, enemies[0], OutcomeHit, 0)
			threatTestHit(spell, enemies[1], OutcomeHit, 0)
		})
		if got := bonusOn(spell, sim); !threatNear(got[0], 40) || !threatNear(got[1], 40) {
			t.Errorf("bonus %v, want [40 40]", got)
		}
	})

	t.Run("with the caster an effect lands on", func(t *testing.T) {
		sim := threatTestSim(1)
		evis := threatTestSpell(t, sim, 48668, SpellSchoolPhysical) // Eviscerate, level 79
		threatTestCast(sim, evis, func() { threatTestHit(evis, sim.Encounter.TargetUnits[0], OutcomeHit, 0) })
		if got := bonusOn(evis, sim); !threatNear(got[0], 39.5) {
			t.Errorf("bonus %v, want [39.5]", got)
		}
	})

	t.Run("a cast landing without a hit", func(t *testing.T) {
		sim := threatTestSim(4)
		consecration := threatTestSpell(t, sim, 48819, SpellSchoolHoly) // level 80
		threatTestCast(sim, consecration, func() {
			for _, enemy := range sim.Encounter.TargetUnits {
				consecration.LandThreatBonus(sim, enemy)
			}
		})
		for i, got := range bonusOn(consecration, sim) {
			if !threatNear(got, 20) {
				t.Errorf("enemy %d: bonus %v, want 20", i, got)
			}
		}
	})

	t.Run("a hit outside a cast, by the server data", func(t *testing.T) {
		sim := threatTestSim(3)
		cleave := threatTestSpell(t, sim, 47520, SpellSchoolPhysical) // chains to 2
		threatTestHit(cleave, sim.Encounter.TargetUnits[0], OutcomeHit, 0)
		if got := bonusOn(cleave, sim); !threatNear(got[0], 56) {
			t.Errorf("bonus %v, want 56 on the first", got)
		}
	})

	t.Run("not before the pull", func(t *testing.T) {
		sim := threatTestSim(1)
		sim.CurrentTime = -1
		spell := threatTestSpell(t, sim, 47450, SpellSchoolPhysical)
		threatTestCast(sim, spell, func() { threatTestHit(spell, sim.Encounter.TargetUnits[0], OutcomeHit, 0) })
		if got := bonusOn(spell, sim); got[0] != 0 {
			t.Errorf("bonus %v before the pull", got)
		}
	})
}

func TestThreatSplitFromServerData(t *testing.T) {
	for _, c := range []struct {
		name  string
		spell int32
		want  int32
	}{
		{"one enemy", 47450, 1},
		{"the caster too", 48668, 2},
		{"an area", 47502, 20},
		{"an area up to MaxTargets, and the caster", 53385, 5},
		{"a chain", 21992, 5},
	} {
		s := serverdata.SpellByID(c.spell)
		if got := threatSplit(s, serverdata.ThreatBySpellID(c.spell), 20); got != c.want {
			t.Errorf("%s (%d): %d units, want %d", c.name, c.spell, got, c.want)
		}
	}
}

func TestThreatNoneWithoutMultiplier(t *testing.T) {
	sim := threatTestSim(1)
	spell := threatTestSpell(t, sim, 47450, SpellSchoolPhysical)
	spell.ThreatMultiplier = 0
	enemy := sim.Encounter.TargetUnits[0]
	threatTestCast(sim, spell, func() { threatTestHit(spell, enemy, OutcomeHit, 1000) })
	if got := spell.SpellMetrics[enemy.UnitIndex].TotalThreat; got != 0 {
		t.Errorf("threat %v, want none", got)
	}
}

// Holy Shield's damage comes off its aura (a proc trigger damage effect), not a cast of the positive 48952
func TestThreatNoBonusForAuraDamage(t *testing.T) {
	for _, c := range []struct {
		noBonus bool
		want    float64
	}{{false, 1000 + 80}, {true, 1000}} {
		sim := threatTestSim(1)
		spell := threatTestSpell(t, sim, 48952, SpellSchoolHoly)
		spell.setThreatFromServer(0, c.noBonus)
		enemy := sim.Encounter.TargetUnits[0]
		threatTestCast(sim, spell, func() { threatTestHit(spell, enemy, OutcomeHit, 1000) })
		if got := spell.SpellMetrics[enemy.UnitIndex].TotalThreat; !threatNear(got, c.want) {
			t.Errorf("NoThreatBonus %v: threat %v, want %v", c.noBonus, got, c.want)
		}
	}
}

func TestThreatAssist(t *testing.T) {
	sim := threatTestSim(2)
	// Fire Nova's cast, a positive spell of level 80
	spell := threatTestSpell(t, sim, 61657, SpellSchoolFire)
	spell.Unit.AddThreatSchoolMultiplier(SpellSchoolFire|SpellSchoolFrost|SpellSchoolNature, 0.7)
	threatTestCast(sim, spell, func() {})
	for _, enemy := range sim.Encounter.TargetUnits {
		if got := spell.SpellMetrics[enemy.UnitIndex].TotalThreat; !threatNear(got, 80*0.7/2) {
			t.Errorf("%s: assist threat %v, want %v", enemy.Label, got, 80*0.7/2)
		}
	}
}
