package feral

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/druid"
	googleProto "google.golang.org/protobuf/proto"
)

type faerieFireRun struct {
	// landed Faerie Fires (Feral) and landed hits of the damage its Bear Form hit triggers
	casts, bearHits int
	// casts that left Clearcasting up, and Omen of Clarity's rolls that came through
	clearcasts, procs int
	// casts that left the Balance T10 2-piece proc up
	setProcs int
}

// two pieces of Lasherweave Regalia, the Balance T10 set
const testLasherweaveMantle, testLasherweaveCover = 50819, 50821

// runFaerieFires has a druid who never swings cast Faerie Fire (Feral) once a second, in Cat Form or Bear
// Form, with Clearcasting and the set's proc taken off before each cast. Nothing misses.
func runFaerieFires(t *testing.T, bear, lasherweave bool, tweaks *proto.SpellTweaksSettings) faerieFireRun {
	t.Helper()
	const casts = 2000
	gear := &proto.EquipmentSpec{}
	if lasherweave {
		for range proto.ItemSlot_ItemSlotRanged + 1 {
			gear.Items = append(gear.Items, &proto.ItemSpec{})
		}
		gear.Items[proto.ItemSlot_ItemSlotShoulder].Id = testLasherweaveMantle
		gear.Items[proto.ItemSlot_ItemSlotHead].Id = testLasherweaveCover
	}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = casts + 2
	encounter.ServerSettings = &proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(0), SpellTweaks: tweaks}

	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTauren,
			Class:         proto.Class_ClassDruid,
			Equipment:     gear,
			Spec:          PlayerOptionsMonoCat,
			Glyphs:        &proto.Glyphs{},
			TalentsString: StandardTalents,
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	cat := sim.Raid.Parties[0].Players[0].(*FeralDruid)
	target := sim.Encounter.TargetUnits[0]
	if !cat.Talents.OmenOfClarity {
		t.Fatal("StandardTalents has no Omen of Clarity")
	}

	var run faerieFireRun
	procOoc := cat.ProcOoc
	cat.ProcOoc = func(sim *core.Simulation) {
		run.procs++
		procOoc(sim)
	}
	cat.RegisterAura(core.Aura{
		Label:    "Faerie Fire test log",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}
			switch {
			case cat.FaerieFire.IsEqual(spell):
				run.casts++
			case spell.ActionID.SameAction(druid.FaerieFireFeralDamageActionID):
				run.bearHits++
			}
		},
	})

	sim.Reset()
	sim.PrePull()
	if bear {
		cat.BearFormAura.Activate(sim)
	}
	cat.AutoAttacks.CancelAutoSwing(sim)
	// or the pull starts the swings again
	cat.AutoAttacks.AutoSwingMelee = false
	// no miss
	cat.AddStatsDynamic(sim, stats.Stats{stats.SpellHit: 20 * core.SpellHitRatingPerHitChance})
	setProc := cat.GetAura("T10-2P proc")
	if lasherweave != (setProc != nil) {
		t.Fatalf("the Balance T10 2-piece proc is registered: %v, want %v", setProc != nil, lasherweave)
	}
	for i := range casts {
		sim.AddPendingAction(&core.PendingAction{
			NextActionAt: time.Duration(i+1) * time.Second,
			Priority:     core.ActionPriorityLow,
			OnAction: func(sim *core.Simulation) {
				cat.ClearcastingAura.Deactivate(sim)
				if setProc != nil {
					setProc.Deactivate(sim)
				}
				cat.GCD.Reset()
				cat.FaerieFire.CD.Reset()
				if !cat.FaerieFire.Cast(sim, target) {
					t.Fatal("Faerie Fire (Feral) didn't cast")
				}
				if cat.ClearcastingAura.IsActive() {
					run.clearcasts++
				}
				if setProc.IsActive() {
					run.setProcs++
				}
			},
		})
	}
	for !sim.Step() {
	}
	sim.Cleanup()

	if run.casts != casts {
		t.Fatalf("%d Faerie Fires landed, want %d", run.casts, casts)
	}
	if want := core.Ternary(bear, casts, 0); run.bearHits != want {
		t.Fatalf("%d Bear Form hits landed, want %d", run.bearHits, want)
	}
	return run
}

// mod-spell-tweaks' spell_tweaks_omen_faerie_fire gives Clearcasting on every Faerie Fire (Feral) that
// lands on a mob. With its switch or SpellTweaks.Enable off that's gone, and stock Omen of Clarity rolls
// its 3.5 PPM on the 1.5 s an instant counts as (Aura::CalcProcChance): 8.75% a landed spell. In Bear Form
// the damage the hit triggers (60089) is a second spell, and Omen procs from procs, so it rolls again.
func TestOmenOfClarityOnFaerieFireFeral(t *testing.T) {
	off := googleProto.Bool(false)

	for _, tc := range []struct {
		name   string
		bear   bool
		tweaks *proto.SpellTweaksSettings
		// 0 for Clearcasting on every cast
		rollsACast int
	}{
		{"cat, the tweak", false, nil, 0},
		{"bear, the tweak", true, nil, 0},
		{"cat, its switch off", false, &proto.SpellTweaksSettings{OmenClarityFaerieFire: off}, 1},
		{"bear, its switch off", true, &proto.SpellTweaksSettings{OmenClarityFaerieFire: off}, 2},
		{"cat, SpellTweaks.Enable off", false, &proto.SpellTweaksSettings{Enable: off}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runFaerieFires(t, tc.bear, false, tc.tweaks)
			if tc.rollsACast == 0 {
				if run.clearcasts != run.casts || run.procs != 0 {
					t.Errorf("Clearcasting after %d of %d Faerie Fires and %d Omen rolls came through, want every cast and no roll",
						run.clearcasts, run.casts, run.procs)
				}
				return
			}
			const chance = 0.0875
			rolls := float64(run.casts * tc.rollsACast)
			want := chance * rolls
			if tolerance := 5 * math.Sqrt(rolls*chance*(1-chance)); math.Abs(float64(run.procs)-want) > tolerance {
				t.Errorf("%d Clearcasting procs from %.0f rolls, want %.0f give or take %.0f", run.procs, rolls, want, tolerance)
			}
			t.Logf("%d Clearcasting procs from %.0f rolls (%.4f a roll)", run.procs, rolls, float64(run.procs)/rolls)
		})
	}
}

// The tweak's script casts Clearcasting itself, so the Balance T10 2-piece proc (70721) doesn't come with it:
// only spell_dru_omen_of_clarity::HandleProc casts that, off Omen's own 8.75% roll, which still runs.
func TestLasherweaveProcOnFaerieFireFeral(t *testing.T) {
	for _, tc := range []struct {
		name       string
		bear       bool
		rollsACast float64
	}{
		{"cat", false, 1},
		{"bear", true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runFaerieFires(t, tc.bear, true, nil)
			if run.clearcasts != run.casts {
				t.Errorf("Clearcasting after %d of %d Faerie Fires, want every cast", run.clearcasts, run.casts)
			}
			chance := 1 - math.Pow(1-0.0875, tc.rollsACast)
			casts := float64(run.casts)
			want := chance * casts
			if tolerance := 5 * math.Sqrt(casts*chance*(1-chance)); math.Abs(float64(run.setProcs)-want) > tolerance {
				t.Errorf("the set's proc after %d of %.0f casts, want %.0f give or take %.0f", run.setProcs, casts, want, tolerance)
			}
			t.Logf("the set's proc after %d of %.0f casts (%.4f a cast)", run.setProcs, casts, float64(run.setProcs)/casts)
		})
	}
}
