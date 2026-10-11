package balance

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// spell_proc -16880 takes a crit in the cast phase, where a missile's crit is already rolled, and again in the
// hit phase. So a crit Wrath gives Nature's Grace at its launch, in time for the next cast, and the hit starts
// its 3 s over: live, 171 of 171 crit Wraths gave 16886 at both.
func TestNaturesGraceComesAtWrathsLaunch(t *testing.T) {
	player := nakedDruidWith(0, `{"type": "TypeAPL", "priorityList": []}`)
	player.DistanceFromTarget = 30
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	druid := sim.Raid.Parties[0].Players[0].(*BalanceDruid)
	grace := druid.NaturesGraceProcAura
	if grace == nil {
		t.Fatal("the druid has no Nature's Grace")
	}

	at := func(when time.Duration, do func(sim *core.Simulation)) {
		sim.AddPendingAction(&core.PendingAction{NextActionAt: when, Priority: core.ActionPriorityLow, OnAction: do})
	}
	var landed time.Duration
	var upLate, downAfter bool
	druid.RegisterAura(core.Aura{
		Label:    "wrath test log",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if druid.Wrath.IsEqual(spell) {
				if !result.DidCrit() {
					t.Fatalf("Wrath landed as %s, not a crit", result.Outcome)
				}
				landed = sim.CurrentTime
				// a player's aura counts from the world tick before the hit; the launch's own 3 s are over by then
				tick := sim.NextWorldTick(landed)
				ends := 2*tick - sim.NextWorldTick(tick+time.Nanosecond) + 3*time.Second
				at(ends-time.Millisecond, func(*core.Simulation) { upLate = grace.IsActive() })
				at(ends+time.Millisecond, func(*core.Simulation) { downAfter = !grace.IsActive() })
			}
		},
	})

	sim.Reset()
	sim.PrePull()
	druid.AddStatsDynamic(sim, stats.Stats{
		stats.SpellCrit: 200 * core.CritRatingPerCritChance,
		stats.SpellHit:  20 * core.SpellHitRatingPerHitChance,
	})
	var launch time.Duration
	at(0, func(sim *core.Simulation) {
		druid.GCD.Reset()
		if !druid.Wrath.Cast(sim, druid.CurrentTarget) {
			t.Fatal("Wrath didn't cast")
		}
		launch = druid.Wrath.CurCast.CastTime
		at(launch-time.Millisecond, func(sim *core.Simulation) {
			if grace.IsActive() {
				t.Error("Nature's Grace is up before Wrath's launch")
			}
		})
		at(launch+time.Millisecond, func(sim *core.Simulation) {
			if landed != 0 {
				t.Fatalf("Wrath landed at %s, by its launch at %s", landed, launch)
			}
			if !grace.IsActive() {
				t.Error("no Nature's Grace at the launch of a crit Wrath")
			}
		})
	})
	for !sim.Step() {
	}
	sim.Cleanup()

	if landed < launch+time.Second {
		t.Fatalf("Wrath launched at %s and landed at %s, want over a second of flight at 30 yards", launch, landed)
	}
	if !upLate || !downAfter {
		t.Errorf("Nature's Grace up just before 3 s past the hit: %v, down just after: %v; want both", upLate, downAfter)
	}
}

// Only a spell the druid casts has the cast phase (Spell::_cast skips it for a triggered one), so at one
// point a crit Moonfire rolls the 33% twice at its hit and a crit star of Starfall once.
func TestNaturesGraceRollsOncePerPhase(t *testing.T) {
	player := nakedDruidWith(0, `{"type": "TypeAPL", "priorityList": []}`)
	// Nature's Grace is the balance tree's 7th talent
	player.TalentsString = StandardTalents[:6] + "1" + StandardTalents[7:]
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	druid := sim.Raid.Parties[0].Players[0].(*BalanceDruid)
	if druid.Talents.NaturesGrace != 1 {
		t.Fatalf("%d points in Nature's Grace, want 1", druid.Talents.NaturesGrace)
	}
	talent, grace := druid.GetAura("Natures Grace"), druid.NaturesGraceProcAura
	star := druid.GetSpell(core.ActionID{SpellID: 53195})
	if talent == nil || grace == nil || star == nil {
		t.Fatalf("the talent's aura %v, its proc %v, Starfall's star %v", talent != nil, grace != nil, star != nil)
	}
	sim.Reset()
	sim.PrePull()

	const crits = 3000
	rate := func(spell *core.Spell) float64 {
		procs := 0
		for range crits {
			grace.Deactivate(sim)
			result := spell.NewResult(druid.CurrentTarget)
			result.Outcome = core.OutcomeCrit
			talent.OnSpellHitDealt(talent, sim, spell, result)
			spell.DisposeResult(result)
			if grace.IsActive() {
				procs++
			}
		}
		return float64(procs) / crits
	}
	for _, c := range []struct {
		name  string
		spell *core.Spell
		want  float64
	}{
		{"Moonfire", druid.Moonfire.Spell, 1 - 0.67*0.67},
		{"a star of Starfall", star, 0.33},
	} {
		// 5 deviations of a rate over 3000 crits
		if got := rate(c.spell); math.Abs(got-c.want) > 0.046 {
			t.Errorf("%s gave Nature's Grace at %.3f of its crits, want %.3f", c.name, got, c.want)
		}
	}
	grace.Deactivate(sim)
	sim.Cleanup()
}
