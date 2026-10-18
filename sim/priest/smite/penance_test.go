package smite

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// DefaultTalents' Discipline tree with Penance; Mental Agility 3/3, no Improved Healing
const penanceTalents = "0533203101300502331000100001-005551002020152-00502"

// newPenancePriest is a naked priest with penanceTalents against a boss and an add.
func newPenancePriest(t *testing.T) (*core.Simulation, *SmitePriest) {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	// an add takes no partial resists
	encounter.Targets = append(encounter.Targets, &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid})
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsBasic,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: penanceTalents,
		}, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].(*SmitePriest)
}

// Damage Penance on the server: Mental Agility's class mask leaves 53007 out, so it costs 16% of base mana;
// 53007 and its channel can't miss and aren't a harmful magic hit, so only the ticks roll and proc; a tick's
// 53000 takes spell_bonus_data's 0.1905 of spell power.
func TestDamagePenance(t *testing.T) {
	sim, priest := newPenancePriest(t)
	if priest.Penance == nil || priest.Talents.MentalAgility != 3 || priest.Talents.ImprovedHealing != 0 {
		t.Fatalf("Penance %v, Mental Agility %d, Improved Healing %d", priest.Penance != nil, priest.Talents.MentalAgility,
			priest.Talents.ImprovedHealing)
	}
	spell, target := priest.Penance, sim.GetTargetUnit(0)
	if got := spell.StaticCost(); got != 618 {
		t.Errorf("Penance costs %v, want 618", got)
	}

	// no gear, so a spell that rolls hit misses about one cast in six
	hitsDealt := 0
	priest.RegisterAura(core.Aura{
		Label:    "Penance test",
		Duration: core.NeverExpires,
		OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, hit *core.Spell, _ *core.SpellResult) {
			if hit == spell {
				hitsDealt++
			}
		},
	}).Activate(sim)
	metrics := &spell.SpellMetrics[target.UnitIndex]
	dot := spell.Dot(target)
	refill := priest.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen})
	for i := 0; i < 40; i++ {
		spell.CD.Reset()
		priest.GCD.Reset()
		dot.Deactivate(sim)
		priest.AddMana(sim, priest.MaxMana(), refill)
		if !spell.Cast(sim, target) || !dot.IsActive() {
			t.Fatalf("cast %d: no Penance channel on the target", i)
		}
		dot.Deactivate(sim)
		landBolts(sim)
	}
	if hitsDealt != 0 {
		t.Errorf("the casts dealt %d hits of their own, want none", hitsDealt)
	}
	if got := metrics.Hits + metrics.Crits + metrics.Misses; got != 40 || metrics.Misses == 0 {
		t.Errorf("40 casts' first ticks: %d rolled, %d missed", got, metrics.Misses)
	}

	// a landed tick without a crit, at no spell power and at 1000
	add := sim.GetTargetUnit(1)
	metrics, dot = &spell.SpellMetrics[add.UnitIndex], spell.Dot(add)
	tickDamage := func() float64 {
		for i := 0; i < 200; i++ {
			before := *metrics
			tickAndLand(sim, dot)
			if metrics.Hits > before.Hits {
				return metrics.TotalDamage - before.TotalDamage
			}
		}
		t.Fatal("no tick landed without a crit")
		return 0
	}
	priest.AddStatDynamic(sim, stats.SpellPower, -spell.SpellPower())
	if sp := spell.SpellPower(); sp != 0 {
		t.Fatalf("spell power %v, want 0", sp)
	}
	base := tickDamage()
	priest.AddStatDynamic(sim, stats.SpellPower, 1000)
	if got := (tickDamage() - base) / (base / 375) / 1000; math.Abs(got-0.1905) > 0.003 {
		t.Errorf("a tick takes %.4f of spell power, want 0.1905", got)
	}
}

// Searing Light and Twin Disciplines are separate percent mods on 53000, so a tick multiplies them. Live at
// 2290 spell power a tick did 974.
func TestDamagePenanceTalentsMultiply(t *testing.T) {
	sim, priest := newPenancePriest(t)
	talents := priest.Talents
	if priest.Penance == nil || talents.SearingLight == 0 || talents.TwinDisciplines == 0 || talents.FocusedPower == 0 {
		t.Fatalf("Penance %v, Searing Light %d, Twin Disciplines %d, Focused Power %d", priest.Penance != nil,
			talents.SearingLight, talents.TwinDisciplines, talents.FocusedPower)
	}
	priest.AddStatDynamic(sim, stats.SpellPower, -priest.Penance.SpellPower())
	if sp := priest.Penance.SpellPower(); sp != 0 {
		t.Fatalf("spell power %v, want 0", sp)
	}

	// the add takes no partial resists
	add := sim.GetTargetUnit(1)
	metrics := &priest.Penance.SpellMetrics[add.UnitIndex]
	dot := priest.Penance.Dot(add)
	want := 375 * (1 + .02*float64(talents.FocusedPower)) * (1 + .05*float64(talents.SearingLight)) *
		(1 + .01*float64(talents.TwinDisciplines))
	for landed, i := 0, 0; landed < 20; i++ {
		if i == 200 {
			t.Fatalf("%d of 200 ticks landed without a crit", landed)
		}
		before := *metrics
		tickAndLand(sim, dot)
		if metrics.Hits == before.Hits {
			continue
		}
		landed++
		if got := metrics.TotalDamage - before.TotalDamage; math.Abs(got-want) > 1 {
			t.Fatalf("a tick deals %.1f, want %.1f", got, want)
		}
	}
}

// Each damage tick is a 53000 that 53003's periodic trigger casts at the channel's target, a missile at 40 yd a
// second: from 30 yd, less the priest's 1.5 reach, 712 ms.
func TestDamagePenanceTicksFly(t *testing.T) {
	sim, priest := newPenancePriest(t)
	priest.DistanceFromTarget = 30
	spell, add := priest.Penance, sim.GetTargetUnit(1)
	if got := spell.TravelTime(); got != 712*time.Millisecond {
		t.Fatalf("a tick flies %v, want 712ms", got)
	}
	metrics := &spell.SpellMetrics[add.UnitIndex]
	dot := spell.Dot(add)
	for i := 0; ; i++ {
		if i == 20 {
			t.Fatal("20 ticks missed")
		}
		dot.Apply(sim)
		start, misses := sim.CurrentTime, metrics.Misses
		dot.TickOnce(sim)
		dot.Deactivate(sim)
		if metrics.Misses > misses {
			landBolts(sim)
			continue
		}
		if metrics.TotalDamage != 0 {
			t.Fatal("the tick dealt its damage as it went off")
		}
		landed := time.Duration(-1)
		for sim.CurrentTime < start+2*time.Second && !sim.Step() {
			if landed < 0 && metrics.TotalDamage != 0 {
				landed = sim.CurrentTime
			}
		}
		// it lands on a world tick at or after the flight
		if flight := landed - start; flight < 712*time.Millisecond || flight > 812*time.Millisecond {
			t.Errorf("the tick landed %v after it went off, want 712ms and up to a world tick or two", flight)
		}
		return
	}
}

// tickAndLand ticks the channel once, ends it, and runs the sim until the tick's bolt has landed.
func tickAndLand(sim *core.Simulation, dot *core.Dot) {
	dot.Apply(sim)
	dot.TickOnce(sim)
	dot.Deactivate(sim)
	landBolts(sim)
}

// landBolts runs the sim long enough for a bolt from 5 yd, the floor, to land.
func landBolts(sim *core.Simulation) {
	for until := sim.CurrentTime + 500*time.Millisecond; sim.CurrentTime < until && !sim.Step(); {
	}
}
