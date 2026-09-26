package optimizer

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// presetPlayer is presetOptimizeRequest's player under another name, for a raid slot.
func presetPlayer(tb testing.TB, preset, name string) *proto.Player {
	tb.Helper()
	player := presetOptimizeRequest(tb, preset).Base.Raid.Parties[0].Players[0]
	player.Name = name
	return player
}

// demonologyWarlock is Demonology's own package test build (sim/warlock/warlock_test.go), with a
// pet summoned so its Demonic Pact talent actually gives the raid spell power.
func demonologyWarlock(name string) *proto.Player {
	return &proto.Player{
		Name:          name,
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarlock,
		Equipment:     core.GetGearSet("../../ui/warlock/gear_sets", "p1_demodestro").GearSet,
		Rotation:      core.GetAplRotation("../../ui/warlock/apls", "demo").Rotation,
		TalentsString: "-203203301035012530135201351-550000052",
		Glyphs: &proto.Glyphs{
			Major1: int32(proto.WarlockMajorGlyph_GlyphOfQuickDecay),
			Major2: int32(proto.WarlockMajorGlyph_GlyphOfLifeTap),
			Major3: int32(proto.WarlockMajorGlyph_GlyphOfFelguard),
		},
		Spec: &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.Warlock_Options{
			Armor:        proto.Warlock_Options_FelArmor,
			Summon:       proto.Warlock_Options_Felguard,
			WeaponImbue:  proto.Warlock_Options_GrandSpellstone,
			DetonateSeed: true,
		}}},
		Consumes: &proto.Consumes{
			Flask:         proto.Flask_FlaskOfTheFrostWyrm,
			Food:          proto.Food_FoodFirecrackerSalmon,
			DefaultPotion: proto.Potions_PotionOfSpeed,
		},
		Buffs: core.FullIndividualBuffs,
	}
}

// raidContribRequest puts players in one party against the UI's default encounter, for a raid-DPS
// optimizer run on the player at targetIndex.
func raidContribRequest(tb testing.TB, targetIndex int, players ...*proto.Player) *proto.OptimizeGearRequest {
	tb.Helper()
	if !core.WITH_DB {
		tb.Skip("needs the with_db item data")
	}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid: &proto.Raid{
				Parties: []*proto.Party{{Players: players, Buffs: core.FullPartyBuffs}},
				Buffs:   core.FullRaidBuffs,
				Debuffs: core.FullDebuffs,
			},
			Encounter: &proto.Encounter{
				Duration:             180,
				DurationVariation:    5,
				ExecuteProportion_20: 0.2,
				ExecuteProportion_25: 0.25,
				ExecuteProportion_35: 0.35,
				Targets:              []*proto.Target{core.NewDefaultTarget()},
			},
			SimOptions: &proto.SimOptions{RandomSeed: 55},
		},
		TargetRaidIndex: int32(targetIndex),
		Settings:        &proto.OptimizerSettings{ContentPhase: 1, Objective: proto.OptimizerObjective_OptimizerObjectiveRaidDps},
	}
}

// Reevaluating the seed loadout, with a fresh evaluator and no shared cache, reproduces the same
// paired random numbers for the whole raid: the delta between the two runs is exactly 0.
func TestRaidEvaluatorIdenticalLoadoutHasNoDelta(t *testing.T) {
	req := raidContribRequest(t, 0, presetPlayer(t, "fury_p1", "Target"), presetPlayer(t, "arcane_p3", "Mage"))
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	a := NewRaidEvaluator(r, MetricDPS)
	a.shardSize = 100
	b := NewRaidEvaluator(r, MetricDPS)
	b.shardSize = 100

	evalA := evaluate(t, a, 200, Point{Loadout: r.Seed})[0]
	evalB := evaluate(t, b, 200, Point{Loadout: r.Seed})[0]
	d := Delta(evalA, evalB, MetricDPS)
	if d.Mean != 0 || d.SE != 0 {
		t.Errorf("two independent raid evaluators scoring the same loadout differ by %+v, want exactly 0", d)
	}
	t.Logf("raid DPS %.1f ± %.1f", evalA.Metrics[MetricDPS].Mean, evalA.Metrics[MetricDPS].SE)
}

// A raid evaluator scores the raid's DPS but still knows the target's own, the same number an ordinary
// evaluator over the same raid reports.
func TestRaidEvaluatorKeepsTheTargetsOwnDPS(t *testing.T) {
	req := raidContribRequest(t, 0, presetPlayer(t, "fury_p1", "Target"), presetPlayer(t, "arcane_p3", "Mage"))
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	raid := NewRaidEvaluator(r, MetricDPS)
	raid.shardSize = 100
	own := NewSimEvaluator(r, MetricDPS)
	own.shardSize = 100
	raidEval := evaluate(t, raid, 200, Point{Loadout: r.Seed})[0]
	ownEval := evaluate(t, own, 200, Point{Loadout: r.Seed})[0]

	if got, want := raidEval.ownDPS(), ownEval.Metrics[MetricDPS].Mean; got != want {
		t.Errorf("raid evaluator's own DPS = %.3f, an ordinary evaluator's = %.3f", got, want)
	}
	if got := ownEval.ownDPS(); got != ownEval.Metrics[MetricDPS].Mean {
		t.Errorf("ordinary evaluator's own DPS = %.3f, its DPS = %.3f", got, ownEval.Metrics[MetricDPS].Mean)
	}
	if raidEval.ownDPS() >= raidEval.Metrics[MetricDPS].Mean {
		t.Errorf("own DPS %.1f isn't below the raid's %.1f", raidEval.ownDPS(), raidEval.Metrics[MetricDPS].Mean)
	}
	t.Logf("own DPS %.1f of the raid's %.1f over %d iterations", raidEval.ownDPS(), raidEval.Metrics[MetricDPS].Mean, raidEval.Iterations)
}

// Set bonuses only move the target's own damage, so a raid-mode run screens sets against its own
// share of J: the rest of the raid's DPS doesn't loosen the screen.
func TestRaidModeScreensSetsAgainstTheTargetsShare(t *testing.T) {
	screen := func(others float64) float64 {
		req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
		if others > 0 {
			req.Settings.Objective = proto.OptimizerObjective_OptimizerObjectiveRaidDps
		}
		r, err := PrepareRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		got := -1.0
		traceHook = func(format string, args ...any) {
			fmt.Sscanf(fmt.Sprintf(format, args...), "set screen: %f of J", &got)
		}
		defer func() { traceHook = nil }()
		fake := newKnownEvaluator(kaMetrics)
		fake.others = others
		if result := optimize(context.Background(), r, r, fake, nil, time.Now()); result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		if got < 0 {
			t.Fatal("the run never screened sets")
		}
		return got
	}
	solo, raid := screen(0), screen(80000)
	if !near(raid, solo, 0.01*solo) {
		t.Errorf("raid mode screens sets at %.1f of J, a solo run at %.1f; want the same", raid, solo)
	}
	t.Logf("set screen: %.1f solo, %.1f in raid mode", solo, raid)
}

// A gear change moves the target's own damage and whatever it buffs elsewhere, but every other
// raider replays identically under IsTest, so a paired raid-DPS delta stays as quiet as a solo
// evaluator's, not louder for carrying the whole raid's output.
func TestRaidEvaluatorPairsAcrossTheRaid(t *testing.T) {
	req := raidContribRequest(t, 0, presetPlayer(t, "fury_p1", "Target"), presetPlayer(t, "arcane_p3", "Mage"))
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	raid := NewRaidEvaluator(r, MetricDPS)
	raid.shardSize = 100
	raidEvals := evaluate(t, raid, 400, Point{Loadout: r.Seed}, withOffset(r.Seed, stats.AttackPower, 100))
	raidDelta := Delta(raidEvals[0], raidEvals[1], MetricDPS)

	solo := presetRequest(t, "fury_p1")
	soloEval := NewSimEvaluator(solo, MetricDPS)
	soloEval.shardSize = 100
	soloEvals := evaluate(t, soloEval, 400, Point{Loadout: solo.Seed}, withOffset(solo.Seed, stats.AttackPower, 100))
	soloDelta := Delta(soloEvals[0], soloEvals[1], MetricDPS)

	if raidDelta.Mean <= 2*raidDelta.SE {
		t.Fatalf("+100 AP moved raid DPS by %+v, want a clear paired gain", raidDelta)
	}
	if raidDelta.SE > 3*soloDelta.SE {
		t.Errorf("raid-wide se %.2f is more than 3x a single raider's own se %.2f: the rest of the raid isn't cancelling out of the pair", raidDelta.SE, soloDelta.SE)
	}
	t.Logf("+100 AP: %.1f ± %.1f raid DPS, vs %.1f ± %.1f solo", raidDelta.Mean, raidDelta.SE, soloDelta.Mean, soloDelta.SE)
}

// A raid-mode normalizer for a Demonology warlock's spell power counts the extra Demonic Pact it
// gives its raidmate, so it comes out higher than the individual context's, which only sees the
// warlock's own DPS.
func TestDemonologyValuesSpellPowerHigherInRaidMode(t *testing.T) {
	// Several pact recipients, so its raid-wide contribution clears the sims' noise: each gets the
	// same aura, so their combined gain scales with their count while the paired standard error
	// only grows with its square root.
	req := raidContribRequest(t, 0, demonologyWarlock("Warlock"), presetPlayer(t, "arcane_p3", "Mage1"), presetPlayer(t, "arcane_p3", "Mage2"), presetPlayer(t, "arcane_p3", "Mage3"))
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Target().GetWarlock().GetOptions().GetSummon(); got == proto.Warlock_Options_NoSummon {
		t.Fatal("the warlock needs a pet out for Demonic Pact to give anything")
	}

	const iterations = 8000
	raidEval := NewRaidEvaluator(r, MetricDPS)
	raidObj, err := NewObjective(context.Background(), raidEval, r, iterations)
	if err != nil {
		t.Fatal(err)
	}
	if raidObj.ReferenceStats[MetricDPS] != stats.SpellPower {
		t.Fatalf("reference stat = %s, want spell power", raidObj.ReferenceStats[MetricDPS].StatName())
	}

	simmed, err := simmedRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	soloEval := NewSimEvaluator(simmed, MetricDPS)
	soloObj, err := NewObjective(context.Background(), soloEval, simmed, iterations)
	if err != nil {
		t.Fatal(err)
	}

	raidNorm, soloNorm := raidObj.Normalizers[MetricDPS], soloObj.Normalizers[MetricDPS]
	if raidNorm.Mean <= soloNorm.Mean+2*(raidNorm.SE+soloNorm.SE) {
		t.Errorf("raid-mode DPS per spell power = %+v, solo = %+v; want raid mode clearly higher, the pact it feeds the mage", raidNorm, soloNorm)
	}
	t.Logf("DPS per spell power: %.3f ± %.3f raid mode, %.3f ± %.3f solo", raidNorm.Mean, raidNorm.SE, soloNorm.Mean, soloNorm.SE)
}

// calibrationPoolReq attaches a one-slot fake pool to req, so the run has something to change and
// runs its full pipeline instead of stopping at "nothing to change".
func calibrationPoolReq(req *proto.OptimizeGearRequest) *proto.OptimizeGearRequest {
	kaRegister()
	req.Pool = &proto.CandidatePool{Slots: []*proto.SlotPool{
		{Slot: proto.ItemSlot_ItemSlotHead, ItemIds: []int32{kaHeadPlain, kaHeadMeta}},
	}}
	return req
}

// A raid batch's own-metrics stage sims the target inside a derived, single-player context: the
// calibration gap says how far that context's own DPS sits from simming the seed in the real raid,
// which the derived context's approximate buffs and debuffs don't reproduce exactly.
func TestCalibrationGapComparesTheDerivedContextToTheRealRaid(t *testing.T) {
	req := calibrationPoolReq(raidContribRequest(t, 0, presetPlayer(t, "fury_p1", "Target"), presetPlayer(t, "arcane_p3", "Mage")))
	req.Settings.Objective = proto.OptimizerObjective_OptimizerObjectiveOwnMetrics
	req.Settings.Effort = proto.OptimizerEffort_OptimizerEffortQuick
	result := Optimize(context.Background(), req, nil)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	if result.CalibrationGap == 0 {
		t.Error("a real raid of two specced players reported no calibration gap at all")
	}
	if math.IsNaN(result.CalibrationGap) || math.Abs(result.CalibrationGap) > 1 {
		t.Errorf("calibration gap %v isn't a sane fraction of the real raid's DPS", result.CalibrationGap)
	}
	t.Logf("calibration gap: %+.2f%%", result.CalibrationGap*100)
}

// A lone target has no real raid to differ from, raid mode already scores against the real raid
// directly, and a run with no DPS term in its objective has nothing to calibrate.
func TestCalibrationGapSkipsWhereItDoesntApply(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  func(tb testing.TB) *proto.OptimizeGearRequest
	}{
		{"a lone target", func(tb testing.TB) *proto.OptimizeGearRequest {
			return calibrationPoolReq(raidContribRequest(tb, 0, presetPlayer(tb, "fury_p1", "Target")))
		}},
		{"raid mode", func(tb testing.TB) *proto.OptimizeGearRequest {
			req := calibrationPoolReq(raidContribRequest(tb, 0, presetPlayer(tb, "fury_p1", "Target"), presetPlayer(tb, "arcane_p3", "Mage")))
			req.Settings.Objective = proto.OptimizerObjective_OptimizerObjectiveRaidDps
			return req
		}},
		{"no DPS term in the objective", func(tb testing.TB) *proto.OptimizeGearRequest {
			req := calibrationPoolReq(raidContribRequest(tb, 0, presetPlayer(tb, "fury_p1", "Target"), presetPlayer(tb, "arcane_p3", "Mage")))
			req.Settings.MetricWeights = &proto.OptimizerMetrics{Tps: 1}
			return req
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req(t)
			req.Settings.Effort = proto.OptimizerEffort_OptimizerEffortQuick
			result := Optimize(context.Background(), req, nil)
			if result.ErrorResult != "" {
				t.Fatal(result.ErrorResult)
			}
			if result.CalibrationGap != 0 {
				t.Errorf("calibration gap = %v, want exactly 0", result.CalibrationGap)
			}
		})
	}
}

// The acceptance bar sizes a raid-mode gain against the target's own share of J (ownJ), not the
// whole raid's: a gain worth clearing 0.05% of the target's own points must clear that bar even
// where the rest of the raid's DPS would otherwise swamp it.
func TestRaidModeAcceptanceBarUsesTheTargetsShare(t *testing.T) {
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	req.Settings.Objective = proto.OptimizerObjective_OptimizerObjectiveRaidDps
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	fake := newKnownEvaluator(kaMetrics)
	fake.others = 1e6
	obj, err := NewObjective(context.Background(), fake, r, 200)
	if err != nil {
		t.Fatal(err)
	}
	seedEvals, err := fake.Evaluate(context.Background(), []Point{{Loadout: r.Seed}}, 200)
	if err != nil {
		t.Fatal(err)
	}
	rn := &run{asked: r, r: r, obj: obj, seedEval: seedEvals[0]}
	rn.seedJ = obj.Score(rn.seedEval)
	own := rn.ownJ()
	if own >= abs(rn.seedJ.Mean) {
		t.Fatalf("own share %.1f isn't smaller than the raid's J %.1f; the fixture needs a bigger others", own, rn.seedJ.Mean)
	}
	if gain := (Estimate{Mean: acceptFraction * own * 1.5}); !rn.accepts(gain) {
		t.Errorf("a %.4f gain, over 0.05%% of the target's own %.1f, wasn't accepted", gain.Mean, own)
	}
	if gain := (Estimate{Mean: acceptFraction * own * 0.5}); rn.accepts(gain) {
		t.Errorf("a %.4f gain, under 0.05%% of the target's own %.1f, was accepted; the bar must not shrink against the rest of the raid", gain.Mean, own)
	}
}

// The raid-mode racial screen sims the whole party, so a Draenei's Heroic Presence lifts a
// hit-starved partymate's own DPS and the race survives the cut; the individual context has no
// partymate to give it to.
func TestRaidRacialScreenCountsPartyRacials(t *testing.T) {
	needsHit := presetPlayer(t, "fury_p1", "Needs Hit")
	needsHit.BonusStats = &proto.UnitStats{Stats: stats.Stats{stats.MeleeHit: -300}.ToFloatArray()}
	req := raidContribRequest(t, 0, presetPlayer(t, "fury_p1", "Target"), needsHit)
	req.Settings.RacialMode = proto.OptimizerRacialMode_OptimizerRacialSearch
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	rn := &run{
		ctx:    context.Background(),
		asked:  r,
		r:      r,
		budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortQuick),
		screen: 250,
	}
	raidEval := NewRaidEvaluator(r, MetricDPS)
	rn.eval = &countingEvaluator{inner: raidEval, have: map[Point]int{}}
	obj, err := NewObjective(rn.ctx, rn.eval, rn.r, rn.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	rn.obj = obj

	finalists, err := rn.screenRacials()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, race := range finalists {
		if race == proto.Race_RaceDraenei {
			found = true
		}
	}
	if !found {
		t.Errorf("Draenei isn't a finalist with a hit-starved partymate (finalists: %v, screen: %v)", finalists, rn.racialScreen)
	}
	t.Logf("screen: %v", rn.racialScreen)
}
