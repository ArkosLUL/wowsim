package rogue

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

type ruptureStep struct {
	at time.Duration
	do func(sim *core.Simulation, rogue *Rogue, target *core.Unit)
}

type ruptureRun struct {
	// when Rupture ticked, and when its aura went off
	ticks, faded []time.Duration
	// its tick interval right after each cast
	periods []time.Duration
	// landed Backstabs
	backstabs int
}

func secs(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// castRupture puts a 5 point Rupture on the target.
func castRupture(at time.Duration) ruptureStep {
	return ruptureStep{at, func(sim *core.Simulation, rogue *Rogue, target *core.Unit) {
		rogue.AddComboPoints(sim, 5, rogue.Rupture.ComboPointMetrics())
		rogue.GCD.Reset()
		if !rogue.Rupture.Cast(sim, target) {
			panic("Rupture didn't cast")
		}
	}}
}

func castBackstab(at time.Duration) ruptureStep {
	return ruptureStep{at, func(sim *core.Simulation, rogue *Rogue, target *core.Unit) {
		rogue.GCD.Reset()
		if !rogue.Backstab.Cast(sim, target) {
			panic("Backstab didn't cast")
		}
	}}
}

// hasteAt gives the rogue 40% melee haste from then on.
func hasteAt(at time.Duration) ruptureStep {
	return ruptureStep{at, func(sim *core.Simulation, rogue *Rogue, _ *core.Unit) {
		rogue.MultiplyMeleeSpeed(sim, 1.4)
	}}
}

// runRupture runs one iteration of a two dagger rogue who only swings, on exact timing, with each step
// done at its time. Every Rupture and Backstab lands.
func runRupture(t *testing.T, talents string, glyph bool, tweaks *proto.SpellTweaksSettings, steps ...ruptureStep) ruptureRun {
	t.Helper()
	gear := &proto.EquipmentSpec{}
	for range proto.ItemSlot_ItemSlotRanged + 1 {
		gear.Items = append(gear.Items, &proto.ItemSpec{})
	}
	gear.Items[proto.ItemSlot_ItemSlotMainHand].Id = testDaggerMH
	gear.Items[proto.ItemSlot_ItemSlotOffHand].Id = testDaggerOH
	player := &proto.Player{
		Race:          proto.Race_RaceHuman,
		Class:         proto.Class_ClassRogue,
		TalentsString: talents,
		Equipment:     gear,
		Glyphs:        &proto.Glyphs{},
		Spec:          PlayerOptionsCombatDI,
		Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
	}
	if glyph {
		player.Glyphs.Major1 = int32(proto.RogueMajorGlyph_GlyphOfBackstab)
	}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 60
	encounter.ServerSettings = &proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(0), SpellTweaks: tweaks}

	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	rogue := sim.Raid.Parties[0].Players[0].(*Rogue)
	target := sim.Encounter.TargetUnits[0]

	var run ruptureRun
	rogue.RegisterAura(core.Aura{
		Label:    "Rupture test log",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == rogue.Backstab && result.Landed() {
				run.backstabs++
			}
		},
		OnPeriodicDamageDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, _ *core.SpellResult) {
			if spell == rogue.Rupture {
				run.ticks = append(run.ticks, sim.CurrentTime)
			}
		},
	})
	dot := rogue.Rupture.Dot(target)
	dot.ApplyOnGain(func(_ *core.Aura, _ *core.Simulation) {
		run.periods = append(run.periods, dot.TickPeriod())
	})
	dot.ApplyOnExpire(func(_ *core.Aura, sim *core.Simulation) {
		run.faded = append(run.faded, sim.CurrentTime)
	})

	sim.Reset()
	sim.PrePull()
	// no miss, no dodge
	rogue.AddStatsDynamic(sim, stats.Stats{
		stats.MeleeHit:  20 * core.MeleeHitRatingPerHitChance,
		stats.Expertise: 100 * core.ExpertisePerQuarterPercentReduction,
	})
	for _, step := range steps {
		sim.AddPendingAction(&core.PendingAction{
			NextActionAt: step.at,
			Priority:     core.ActionPriorityLow,
			OnAction:     func(sim *core.Simulation) { step.do(sim, rogue, target) },
		})
	}
	for !sim.Step() {
	}
	sim.Cleanup()
	return run
}

func timesFrom(first, last, step float64) []time.Duration {
	var out []time.Duration
	for s := first; s <= last; s += step {
		out = append(out, secs(s))
	}
	return out
}

// mod-spell-tweaks' spell_tweaks_glyph_of_backstab: a Backstab adds 2 s to Rupture's time left and to its max
// duration, 6 s a cast, so the added seconds tick. With its switch or SpellTweaks.Enable off it's core's
// rule: the time left grows, three times for a fresh Rupture, and the ticks stay max duration / amplitude.
// Live before the fix: 8 ticks with or without the glyph, the aura off at 22 s.
func TestGlyphOfBackstabExtendsRupture(t *testing.T) {
	off := googleProto.Bool(false)
	// Rupture at 1 s, then four Backstabs
	steps := []ruptureStep{castRupture(secs(1)), castBackstab(secs(3)), castBackstab(secs(8)), castBackstab(secs(13)), castBackstab(secs(18))}

	for _, tc := range []struct {
		name   string
		glyph  bool
		tweaks *proto.SpellTweaksSettings
		ticks  []time.Duration
		faded  time.Duration
	}{
		{"no glyph", false, nil, timesFrom(3, 17, 2), secs(17)},
		{"glyph", true, nil, timesFrom(3, 23, 2), secs(23)},
		{"glyph, its switch off", true, &proto.SpellTweaksSettings{GlyphOfBackstabTicks: off}, timesFrom(3, 17, 2), secs(23)},
		{"glyph, SpellTweaks.Enable off", true, &proto.SpellTweaksSettings{Enable: off}, timesFrom(3, 17, 2), secs(23)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runRupture(t, AssassinationTalents, tc.glyph, tc.tweaks, steps...)
			if run.backstabs != 4 {
				t.Fatalf("%d Backstabs landed, want 4", run.backstabs)
			}
			if !slices.Equal(run.ticks, tc.ticks) {
				t.Errorf("Rupture ticked at %v, want %v", run.ticks, tc.ticks)
			}
			if !slices.Equal(run.faded, []time.Duration{tc.faded}) {
				t.Errorf("Rupture went off at %v, want %v", run.faded, tc.faded)
			}
		})
	}
}

// HandleCalcPeriodic zeroes the script's counter on every cast, so a recast gets its own 6 s.
func TestGlyphOfBackstabStartsOverOnARecast(t *testing.T) {
	run := runRupture(t, AssassinationTalents, true, nil,
		castRupture(secs(1)), castBackstab(secs(3)), castBackstab(secs(8)), castBackstab(secs(13)),
		castRupture(secs(20)), castBackstab(secs(21)))
	if run.backstabs != 4 {
		t.Fatalf("%d Backstabs landed, want 4", run.backstabs)
	}
	// the first Rupture's ticks up to the recast, then the second's 16 s and one Backstab's 2
	want := append(timesFrom(3, 19, 2), timesFrom(22, 38, 2)...)
	if !slices.Equal(run.ticks, want) {
		t.Errorf("Rupture ticked at %v, want %v", run.ticks, want)
	}
	if want := []time.Duration{secs(20), secs(38)}; !slices.Equal(run.faded, want) {
		t.Errorf("Rupture went off at %v, want %v", run.faded, want)
	}
}

// The script leaves the amplitude alone: the added seconds tick at the interval Rupture went on with, whatever
// the rogue's haste is by the Backstab. With Weapon Expertise that interval is hasted (spell_tweaks_rupture_haste).
func TestGlyphOfBackstabKeepsRupturesTickInterval(t *testing.T) {
	off := googleProto.Bool(false)
	casts := []ruptureStep{castRupture(secs(1)), castBackstab(secs(3)), castBackstab(secs(8)), castBackstab(secs(13))}

	for _, tc := range []struct {
		name   string
		haste  time.Duration
		tweaks *proto.SpellTweaksSettings
		// what the ticks have to fit in
		ticking time.Duration
	}{
		{"haste after the cast", secs(2), nil, secs(22)},
		{"haste before the cast", secs(0.5), nil, secs(22)},
		{"haste before the cast, the glyph's switch off", secs(0.5), &proto.SpellTweaksSettings{GlyphOfBackstabTicks: off}, secs(16)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runRupture(t, CombatTalents, true, tc.tweaks, append([]ruptureStep{hasteAt(tc.haste)}, casts...)...)
			if run.backstabs != 3 || len(run.periods) != 1 {
				t.Fatalf("%d Backstabs landed and Rupture went on %d times, want 3 and 1", run.backstabs, len(run.periods))
			}
			period := run.periods[0]
			if hasted := tc.haste < secs(1); hasted != (period < 1500*time.Millisecond) {
				t.Fatalf("Rupture went on ticking every %v, hasted should be %v", period, hasted)
			}
			var want []time.Duration
			for at := secs(1) + period; at <= secs(1)+tc.ticking; at += period {
				want = append(want, at)
			}
			if !slices.Equal(run.ticks, want) {
				t.Errorf("Rupture ticked at %v, want every %v: %v", run.ticks, period, want)
			}
			if want := []time.Duration{secs(23)}; !slices.Equal(run.faded, want) {
				t.Errorf("Rupture went off at %v, want %v", run.faded, want)
			}
		})
	}
}
