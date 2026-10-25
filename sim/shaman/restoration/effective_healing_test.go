package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// A heal on a dummy that takes damage counts up to its missing health, the rest is overheal.
func TestHealOnADummyCountsUpToMissingHealth(t *testing.T) {
	sim, shaman := healSetup{dummies: 2, damage: 1}.build(t)
	sim.Reset()
	dummy := targetDummies(sim)[0]
	neverCrit(sim, shaman, shaman.HealingWave)
	spell := shaman.HealingWave
	idx := dummy.UnitIndex

	dummy.RemoveHealth(sim, 1000)
	heal := castHeal(sim, spell, dummy)
	if heal <= 1000 {
		t.Fatalf("heal of %v, want one bigger than the 1000 missing", heal)
	}
	if got := spell.SpellMetrics[idx].TotalOverheal; got != heal-1000 {
		t.Errorf("overheal %v, want %v of %v", got, heal-1000, heal)
	}
	if dummy.CurrentHealth() != dummy.MaxHealth() {
		t.Errorf("health %v of %v after a heal past full", dummy.CurrentHealth(), dummy.MaxHealth())
	}

	second := castHeal(sim, spell, dummy)
	if got := spell.SpellMetrics[idx].TotalOverheal; got != heal-1000+second {
		t.Errorf("overheal %v after a heal on a full dummy, want all %v of it added", got, second)
	}
}

// Without incoming damage a dummy has no health bar and every heal counts in full.
func TestHealOnADummyWithoutDamageCountsInFull(t *testing.T) {
	sim, shaman := healSetup{dummies: 2}.build(t)
	sim.Reset()
	dummy := targetDummies(sim)[0]
	neverCrit(sim, shaman, shaman.HealingWave)
	castHeal(sim, shaman.HealingWave, dummy)
	if got := shaman.HealingWave.SpellMetrics[dummy.UnitIndex].TotalOverheal; got != 0 {
		t.Errorf("overheal %v on a dummy without a health bar", got)
	}
}

func TestPageRequestReportsEffectiveAndRawHPS(t *testing.T) {
	run := func(damage float64) *proto.UnitMetrics {
		rsr := pageRequest("p1")
		rsr.SimOptions = &proto.SimOptions{Iterations: 100, IsTest: true, RandomSeed: 101}
		rsr.Raid.IncomingDamagePerSecond = damage
		result := core.RunRaidSim(rsr)
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		return result.RaidMetrics.Parties[0].Players[0]
	}

	idle := run(100)
	if idle.RawHps == nil || idle.Overheal == nil || idle.UnhealedDps == nil {
		t.Fatal("a request with incoming damage reports no raw HPS, overheal or unhealed damage")
	}
	if idle.Hps.Avg > idle.RawHps.Avg/5 {
		t.Errorf("HPS %.0f of %.0f raw under 100 damage a second, want most of it overheal", idle.Hps.Avg, idle.RawHps.Avg)
	}
	if got, want := idle.RawHps.Avg-idle.Hps.Avg, idle.Overheal.Avg; math.Abs(got-want) > 1e-6*idle.RawHps.Avg {
		t.Errorf("raw less effective HPS is %.3f, overheal %.3f", got, want)
	}
	// the rotation lets a dummy sit short of 1500 health before it heals
	if idle.UnhealedDps.Avg > 20 {
		t.Errorf("%.1f damage a second unhealed under 100 damage a second, want the heals to answer it", idle.UnhealedDps.Avg)
	}

	flood := run(1e6)
	if flood.Hps.Avg < 0.99*flood.RawHps.Avg {
		t.Errorf("HPS %.0f of %.0f raw when the dummies sit at 1 health", flood.Hps.Avg, flood.RawHps.Avg)
	}
	if flood.UnhealedDps.Avg < 0.9*1e6 {
		t.Errorf("%.0f damage a second unhealed of 1e6", flood.UnhealedDps.Avg)
	}

	rsr := pageRequest("p1")
	rsr.SimOptions = &proto.SimOptions{Iterations: 20, IsTest: true, RandomSeed: 101}
	rsr.Raid.IncomingDamagePerSecond = 0
	unit := core.RunRaidSim(rsr).RaidMetrics.Parties[0].Players[0]
	if unit.RawHps != nil || unit.Overheal != nil || unit.UnhealedDps != nil {
		t.Error("a request without incoming damage reports effective healing metrics")
	}
}

func hurtDummies(sim *core.Simulation, missing ...float64) []*core.Unit {
	dummies := targetDummies(sim)
	for i, lost := range missing {
		if lost > 0 {
			dummies[i].RemoveHealth(sim, lost)
		}
	}
	return dummies
}

const lesserHealingWave = "49276"

// MostHurt picks the unit with the lowest health percent each time the action runs.
func TestAPLHealsTheMostHurtUnit(t *testing.T) {
	sim, shaman := healSetup{dummies: 3, damage: 1, apl: `{"type": "TypeAPL", "priorityList": [
		{"action": {"castSpell": {"spellId": {"spellId": ` + lesserHealingWave + `}, "target": {"type": "MostHurt"}}}}
	]}`}.build(t)
	order := watchHeals(shaman, func() *core.Spell { return shaman.LesserHealingWave })
	startFight(sim)
	// percents 98%, 98.5% and 99.5%
	dummies := hurtDummies(sim, 0, 400, 300)
	tank := dummies[0]
	tank.RemoveHealth(sim, 200)
	runUntil(sim, 5*time.Second)

	if len(*order) < 3 {
		t.Fatalf("%d heals in 5 s, want 3", len(*order))
	}
	if want := []*core.Unit{dummies[1], dummies[2], tank}; (*order)[0] != want[0] || (*order)[1] != want[1] || (*order)[2] != want[2] {
		t.Errorf("healed %v, want %v", unitLabels((*order)[:3]), unitLabels(want))
	}
}

// With everyone at full health a tie goes to the first unit in raid order.
func TestMostHurtTieGoesToTheFirstInRaidOrder(t *testing.T) {
	sim, shaman := healSetup{dummies: 3, damage: 1, apl: `{"type": "TypeAPL", "priorityList": [
		{"action": {"castSpell": {"spellId": {"spellId": ` + lesserHealingWave + `}, "target": {"type": "MostHurt"}}}}
	]}`}.build(t)
	order := watchHeals(shaman, func() *core.Spell { return shaman.LesserHealingWave })
	startFight(sim)
	runUntil(sim, 1900*time.Millisecond)
	if len(*order) == 0 || (*order)[0] != &shaman.Unit {
		t.Errorf("healed %v, want the shaman first in raid order", unitLabels(*order))
	}
}

// Tank is dummy 1 whoever the current target is.
func TestAPLTankIsDummyOne(t *testing.T) {
	sim, shaman := healSetup{dummies: 3, damage: 1, apl: `{"type": "TypeAPL", "priorityList": [
		{"action": {"condition": {"not": {"val": {"dotIsActive": {"targetUnit": {"type": "Tank"}, "spellId": {"spellId": 49284}}}}},
			"castSpell": {"spellId": {"spellId": 49284}, "target": {"type": "Tank"}}}}
	]}`, talents: StandardTalents, glyphs: StandardGlyphs}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	shaman.CurrentTarget = dummies[2]
	runUntil(sim, 6*time.Second)
	if !shaman.EarthShield.Hot(dummies[0]).IsActive() {
		t.Error("Earth Shield isn't on the tank")
	}
	if shaman.EarthShield.Hot(dummies[2]).IsActive() {
		t.Error("Earth Shield is on the current target")
	}
}

// Missing health is the whole health a unit is short of.
func TestAPLMissingHealthCondition(t *testing.T) {
	apl := `{"type": "TypeAPL", "priorityList": [
		{"action": {"condition": {"cmp": {"op": "OpGt", "lhs": {"missingHealth": {"sourceUnit": {"type": "MostHurt"}}}, "rhs": {"const": {"val": "1000"}}}},
			"castSpell": {"spellId": {"spellId": ` + lesserHealingWave + `}, "target": {"type": "MostHurt"}}}}
	]}`
	for _, c := range []struct {
		missing float64
		casts   bool
	}{{900, false}, {1100, true}} {
		sim, shaman := healSetup{dummies: 2, damage: 1, apl: apl}.build(t)
		order := watchHeals(shaman, func() *core.Spell { return shaman.LesserHealingWave })
		startFight(sim)
		hurtDummies(sim, 0, c.missing)
		runUntil(sim, 1900*time.Millisecond)
		if got := len(*order) > 0; got != c.casts {
			t.Errorf("missing %v: cast %v, want %v", c.missing, got, c.casts)
		}
	}
}

// Multihot puts the hot on the most hurt units that lack it, and none on a full unit.
func TestAPLMultihotSkipsFullAndCoveredUnits(t *testing.T) {
	sim, shaman := healSetup{dummies: 4, damage: 1, apl: `{"type": "TypeAPL", "priorityList": [
		{"action": {"multihot": {"spellId": {"spellId": 61301}}}}
	]}`}.build(t)
	startFight(sim)
	dummies := hurtDummies(sim, 0, 400, 300, 0)
	runUntil(sim, 8*time.Second)

	for i, want := range []bool{false, true, true, false} {
		if got := shaman.Riptide.Hot(dummies[i]).IsActive(); got != want {
			t.Errorf("Riptide on dummy %d: %v, want %v", i+1, got, want)
		}
	}
	if got := shaman.Riptide.SpellMetrics[dummies[1].UnitIndex].Casts; got != 1 {
		t.Errorf("%d Riptides on the most hurt dummy, want 1 while it lasts", got)
	}
}
