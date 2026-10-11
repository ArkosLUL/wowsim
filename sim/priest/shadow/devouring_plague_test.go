package shadow

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

// DefaultTalents without Shadow Weaving, whose stacks would grow between two casts
const noShadowWeavingTalents = "05032031--325023051220010323151301351"

// runPlagueBursts casts Devouring Plague at each time on a naked Shadowform priest with this much spell haste,
// and returns what Improved Devouring Plague's burst did each time, before crit and resist.
func runPlagueBursts(t *testing.T, haste float64, tweaks *proto.SpellTweaksSettings, casts ...time.Duration) []float64 {
	t.Helper()
	// no miss
	run := runPlagueCasts(t, haste, 20, tweaks, casts...)
	if len(run.bursts) != len(casts) {
		t.Fatalf("%d bursts from %d casts", len(run.bursts), len(casts))
	}
	return run.bursts
}

// plagueRun is what a run of Devouring Plague casts did: how many put the dot on, how many bursts that
// cast, and what each burst that landed did.
type plagueRun struct {
	dots, burstCasts int
	bursts           []float64
}

// runPlagueCasts is runPlagueBursts with this much spell hit in percent on top of the talents'.
func runPlagueCasts(t *testing.T, haste, hitPct float64, tweaks *proto.SpellTweaksSettings, casts ...time.Duration) plagueRun {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 60
	encounter.ServerSettings = &proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(0), SpellTweaks: tweaks}

	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Spec:          PlayerOptionsBasic,
			Glyphs:        &proto.Glyphs{},
			TalentsString: noShadowWeavingTalents,
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	priest := sim.Raid.Parties[0].Players[0].(*ShadowPriest)
	target := sim.Encounter.TargetUnits[0]
	if talents := priest.Talents; !talents.Shadowform || talents.ImprovedDevouringPlague != 3 || talents.ShadowWeaving != 0 {
		t.Fatalf("talents: Shadowform %v, Improved Devouring Plague %d, Shadow Weaving %d, want true, 3 and 0",
			talents.Shadowform, talents.ImprovedDevouringPlague, talents.ShadowWeaving)
	}

	var run plagueRun
	priest.RegisterAura(core.Aura{
		Label:    "Devouring Plague test log",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			switch {
			case spell == priest.DevouringPlague && result.Landed():
				run.dots++
			case spell.ActionID.SpellID == 63675:
				run.burstCasts++
				if result.Landed() {
					run.bursts = append(run.bursts, result.PreOutcomeDamage)
				}
			}
		},
	})

	sim.Reset()
	sim.PrePull()
	priest.MultiplyCastSpeed(haste)
	priest.AddStatsDynamic(sim, stats.Stats{stats.SpellHit: hitPct * core.SpellHitRatingPerHitChance})
	// free, so a long run of casts doesn't run dry
	priest.DevouringPlague.CostMultiplier -= 1
	for _, at := range casts {
		sim.AddPendingAction(&core.PendingAction{
			NextActionAt: at,
			Priority:     core.ActionPriorityLow,
			OnAction: func(sim *core.Simulation) {
				priest.GCD.Reset()
				if !priest.DevouringPlague.Cast(sim, target) {
					t.Fatal("Devouring Plague didn't cast")
				}
			},
		})
	}
	for !sim.Step() {
	}
	sim.Cleanup()
	return run
}

// The burst comes off Devouring Plague's aura as it goes on (Aura::HandleAuraSpecificMods), so a cast the
// target resists has none.
func TestImprovedDevouringPlagueNeedsTheDotToLand(t *testing.T) {
	var casts []time.Duration
	for at := time.Second; at < 59*time.Second; at += 1500 * time.Millisecond {
		casts = append(casts, at)
	}
	// with no hit from gear, some of these miss the boss
	run := runPlagueCasts(t, 1, 0, nil, casts...)
	if run.dots == 0 || run.dots == len(casts) {
		t.Fatalf("%d of %d Devouring Plagues landed, want some to miss", run.dots, len(casts))
	}
	if run.burstCasts != run.dots {
		t.Errorf("%d bursts from %d casts, %d of which landed", run.burstCasts, len(casts), run.dots)
	}
}

// Improved Devouring Plague's burst is a share of the dot's whole damage, the tick times
// AuraEffect::GetTotalTicks. With the ShadowDotHaste tweak haste adds ticks. Stock, Shadowform's passive
// 71167 shortens the duration with the interval, so a new dot has its 8. A recast over a running dot counts
// more: Aura::RefreshTimers puts the whole 24 s back over the hasted interval, the burst goes off, and only
// then the cast shortens the duration again.
func TestImprovedDevouringPlagueTickCount(t *testing.T) {
	off := googleProto.Bool(false)
	// 25% haste: a 2.4 s interval, so 10 of them in 24 s. The dot of the cast at 4 s is long over by 40 s.
	casts := []time.Duration{time.Second, 4 * time.Second, 40 * time.Second}
	tweaked := runPlagueBursts(t, 1.25, nil, casts...)
	perTick := tweaked[0] / 10

	for _, tc := range []struct {
		name   string
		haste  float64
		tweaks *proto.SpellTweaksSettings
		ticks  []float64
	}{
		{"the tweak", 1.25, nil, []float64{10, 10, 10}},
		{"its switch off", 1.25, &proto.SpellTweaksSettings{ShadowDotHaste: off}, []float64{8, 10, 8}},
		{"SpellTweaks.Enable off", 1.25, &proto.SpellTweaksSettings{Enable: off}, []float64{8, 10, 8}},
		{"its switch off, no haste", 1, &proto.SpellTweaksSettings{ShadowDotHaste: off}, []float64{8, 8, 8}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bursts := runPlagueBursts(t, tc.haste, tc.tweaks, casts...)
			for i, burst := range bursts {
				if got := burst / perTick; math.Abs(got-tc.ticks[i]) > 0.02 {
					t.Errorf("cast %d: the burst is %.3f ticks' worth, want %v", i+1, got, tc.ticks[i])
				}
			}
		})
	}
}
