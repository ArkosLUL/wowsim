package healing

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Discipline to Penance without Grace, which would raise the later ticks, then Spiritual Healing 5/5 and
// Blessed Resilience 3/3
const penanceHealTalents = "0533203101300502331000100001-0055510020201525003"

// Spiritual Healing and Twin Disciplines are percent mods on 52985, Blessed Resilience and Focused Power healing
// done auras, so a heal tick multiplies all four. Live at 2290 spell power, with the last two only, a tick
// healed 2960 to 3170.
func TestHealPenanceTalentsMultiply(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsDisc,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: penanceHealTalents,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*HealingPriest)
	talents := priest.Talents
	if priest.PenanceHeal == nil || talents.Grace != 0 || talents.SpiritualHealing != 5 || talents.BlessedResilience != 3 ||
		talents.FocusedPower == 0 || talents.TwinDisciplines == 0 {
		t.Fatalf("Penance %v, Grace %d, Spiritual Healing %d, Blessed Resilience %d, Focused Power %d, Twin Disciplines %d",
			priest.PenanceHeal != nil, talents.Grace, talents.SpiritualHealing, talents.BlessedResilience,
			talents.FocusedPower, talents.TwinDisciplines)
	}
	spell := priest.PenanceHeal
	priest.AddStatDynamic(sim, stats.SpellPower, -spell.SpellPower())
	if power := spell.HealingPower(&priest.Unit); power != 0 {
		t.Fatalf("healing power %v, want 0", power)
	}

	multiplier := (1 + .02*float64(talents.SpiritualHealing)) * (1 + .01*float64(talents.BlessedResilience)) *
		(1 + .02*float64(talents.FocusedPower)) * (1 + .01*float64(talents.TwinDisciplines))
	metrics := &spell.SpellMetrics[priest.UnitIndex]
	hot := spell.Hot(&priest.Unit)
	for landed, i := 0, 0; landed < 20; i++ {
		if i == 2000 {
			t.Fatalf("%d of 2000 ticks went without a crit", landed)
		}
		before := *metrics
		hot.Apply(sim)
		hot.TickOnce(sim)
		hot.Deactivate(sim)
		runFor(sim, 100*time.Millisecond)
		if metrics.Hits == before.Hits {
			continue
		}
		landed++
		if got := metrics.TotalHealing - before.TotalHealing; got < 1484*multiplier-1 || got > 1676*multiplier+1 {
			t.Fatalf("a tick heals %.1f, want %.1f to %.1f", got, 1484*multiplier, 1676*multiplier)
		}
	}
}

// Each heal tick is a 52985 that 52988's periodic trigger casts at the channel's target, a missile at 40 yd a second
// like the damage one, starting its flight on the tick after its tick. At another unit it flies the gap between their
// distances from the target, less the priest's 1.5 reach: from the priest at 30 yd to the target dummy at the boss,
// 712 ms. At the priest himself it has no flight (Spell::AddUnitTarget), so it lands on that next tick. The channel
// starts the tick after the press's, as the damage one does.
func TestHealPenanceTicksFly(t *testing.T) {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:               proto.Race_RaceUndead,
			Class:              proto.Class_ClassPriest,
			Equipment:          &proto.EquipmentSpec{},
			Consumes:           &proto.Consumes{},
			Spec:               PlayerOptionsDisc,
			Rotation:           core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString:      penanceHealTalents,
			DistanceFromTarget: 30,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	}
	rsr.Raid.TargetDummies = 1
	sim := core.NewSim(rsr)
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*HealingPriest)
	spell, dummy := priest.PenanceHeal, priest.CurrentTarget
	if dummy == &priest.Unit || dummy.DistanceFromTarget != 0 {
		t.Fatalf("the priest's target is himself or stands %v yd off the boss", dummy.DistanceFromTarget)
	}
	if got := spell.TravelTimeTo(dummy); got != 712*time.Millisecond {
		t.Fatalf("a tick flies %v to the dummy, want 712ms", got)
	}

	for _, tc := range []struct {
		name   string
		target *core.Unit
		flight time.Duration
	}{
		{"the dummy", dummy, 712 * time.Millisecond},
		{"the priest", &priest.Unit, 0},
	} {
		metrics := &spell.SpellMetrics[tc.target.UnitIndex]
		press := sim.CurrentTime + 1003*time.Millisecond
		spell.CD.Reset()
		sim.AddPendingAction(&core.PendingAction{NextActionAt: press, OnAction: func(sim *core.Simulation) {
			priest.GCD.Reset()
			if !spell.Cast(sim, tc.target) {
				t.Fatalf("%s: Penance failed", tc.name)
			}
		}})
		hot := spell.Hot(tc.target)
		var landed []time.Duration
		period := time.Duration(0)
		for sim.CurrentTime < press+4*time.Second {
			healing := metrics.TotalHealing
			if sim.Step() {
				break
			}
			if period == 0 && hot.IsActive() {
				period = hot.TickPeriod()
			}
			if metrics.TotalHealing != healing {
				landed = append(landed, sim.CurrentTime)
			}
		}

		pressTick := sim.NextWorldTick(press)
		lands := func(tick time.Duration) time.Duration {
			return sim.NextWorldTick(sim.NextWorldTick(tick+1) + tc.flight)
		}
		want := []time.Duration{lands(sim.NextWorldTick(pressTick + 1)), lands(sim.NextWorldTick(pressTick + period)),
			lands(sim.NextWorldTick(pressTick + 2*period))}
		if !slices.Equal(landed, want) {
			t.Errorf("%s: the heals landed at %v, want %v", tc.name, landed, want)
		}
	}
}

// runFor runs the sim for d.
func runFor(sim *core.Simulation, d time.Duration) {
	for until := sim.CurrentTime + d; sim.CurrentTime < until && !sim.Step(); {
	}
}
