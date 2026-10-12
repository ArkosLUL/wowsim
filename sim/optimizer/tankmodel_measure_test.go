package optimizer

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"google.golang.org/protobuf/encoding/protojson"
	goproto "google.golang.org/protobuf/proto"
)

// rosterRaid reads a raid sim page's saved state (the raid.json the BiS driver imports, or its
// storage snapshot): the roster is never committed, so the path comes from the environment.
func rosterRaid(tb testing.TB) *proto.Raid {
	tb.Helper()
	path := os.Getenv("BIS_ROSTER")
	if path == "" {
		tb.Skip("BIS_ROSTER names a raid sim storage snapshot or a raid JSON")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err != nil {
		tb.Fatal(err)
	}
	if raw, ok := wrapper["__wotlk_raid____currentSettings__"]; ok {
		var inner string
		if json.Unmarshal(raw, &inner) == nil {
			raw = json.RawMessage(inner)
		}
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(raw, &settings); err != nil {
			tb.Fatal(err)
		}
		data = settings["raid"]
	}
	raid := &proto.Raid{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, raid); err != nil {
		tb.Fatal(err)
	}
	return raid
}

// tankFightRequest is the roster's raid with the named raider tanking the phase's fight, the way the
// raid batch builds it: the whole raid, so the server's dungeon scale is the full-raid one.
func tankFightRequest(tb testing.TB, name string, phase int32) *Request {
	tb.Helper()
	if !core.WITH_DB {
		tb.Skip("needs the with_db item data")
	}
	raid := goproto.Clone(rosterRaid(tb)).(*proto.Raid)
	// the page adds the raid's buffs from its composition when it builds a request; the saved state
	// has none, so give the raid the usual full set and the tank its blessings
	raid.Buffs, raid.Debuffs = core.FullRaidBuffs, core.FullDebuffs
	for _, party := range raid.Parties {
		party.Buffs = core.FullPartyBuffs
	}
	index := -1
	var player *proto.Player
	for i, party := range raid.Parties {
		for j, p := range party.Players {
			if p.GetName() == name {
				index, player = i*5+j, p
			}
		}
	}
	if player == nil {
		tb.Fatalf("no raider %q in the roster", name)
	}
	player.Buffs = core.FullIndividualBuffs
	raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: int32(index)}}
	req := &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid:       raid,
			Encounter:  tankEncounter(tb, phase),
			SimOptions: &proto.SimOptions{RandomSeed: 101},
		},
		TargetRaidIndex: int32(index),
		Settings: &proto.OptimizerSettings{
			ContentPhase:        phase,
			Effort:              proto.OptimizerEffort_OptimizerEffortNormal,
			TankSurvival:        0.7,
			RequireCritImmunity: true,
			RacialMode:          proto.OptimizerRacialMode_OptimizerRacialKeepCurrent,
		},
	}
	player.HealingModel = tankHealing(req.Base.Encounter.Targets[0])
	r, err := PrepareRequest(req)
	if err != nil {
		tb.Fatal(err)
	}
	if err := (&run{r: r}).pinHealingModel(); err != nil {
		tb.Fatal(err)
	}
	return r
}

// TestMeasureTankModel prints, per raider and phase, the tank fight's damage taken and chance of
// death, the objective's normalizers, and what one more point of stamina, agility, dodge and defense
// rating is worth. It asserts only that the objective keeps all three survival terms.
//
//	BIS_ROSTER=<storage-snapshot.json> go test -tags with_db -run TestMeasureTankModel -v ./sim/optimizer/
func TestMeasureTankModel(t *testing.T) {
	budget := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal)
	t.Logf("Normal budget: %+v", budget)
	iterations := budget.Iterations
	offsets := []struct {
		name string
		stat stats.Stat
	}{
		{"Stamina", stats.Stamina}, {"Agility", stats.Agility}, {"Dodge", stats.Dodge}, {"Defense", stats.Defense},
	}
	const bump = 100.0

	for _, name := range []string{"Bulwark", "Felesta"} {
		for _, phase := range []int32{1, 2, 3} {
			r := tankFightRequest(t, name, phase)
			eval := NewSimEvaluator(r, MetricDTPS, MetricTMI, MetricTPS, MetricPDeath)
			obj, err := NewObjective(context.Background(), eval, r, iterations)
			if err != nil {
				t.Fatalf("%s phase %d: %v", name, phase, err)
			}
			t.Logf("%s phase %d: pinned %.0f HPS, weights %v normalizers %+v warnings %v", name, phase,
				r.Target().GetHealingModel().GetHps(), obj.Weights, obj.Normalizers, obj.Warnings)
			for _, m := range []Metric{MetricDTPS, MetricTMI, MetricPDeath} {
				if obj.coef[m] == 0 {
					t.Errorf("%s phase %d: %s left out of the objective", name, phase, metricName(m))
				}
			}
			points := []Point{{Loadout: r.Seed}}
			for _, o := range offsets {
				var offset stats.Stats
				offset[o.stat] = bump
				points = append(points, Point{Loadout: r.Seed, Offset: offset})
			}
			evals, err := eval.Evaluate(context.Background(), points, iterations)
			if err != nil {
				t.Fatal(err)
			}
			seed := evals[0]
			t.Logf("%s phase %d seed: DTPS %.0f TMI %.0f TPS %.0f death %.4f", name, phase,
				seed.Metrics[MetricDTPS].Mean, seed.Metrics[MetricTMI].Mean, seed.Metrics[MetricTPS].Mean, seed.Metrics[MetricPDeath].Mean)
			for i, o := range offsets {
				d := obj.Delta(seed, evals[i+1])
				t.Logf("%s phase %d: dJ per %s point %+.4f (se %.4f); dDTPS %+.3f dTMI %+.3f dDeath %+.6f",
					name, phase, o.name, d.Mean/bump, d.SE/bump,
					Delta(seed, evals[i+1], MetricDTPS).Mean/bump, Delta(seed, evals[i+1], MetricTMI).Mean/bump,
					Delta(seed, evals[i+1], MetricPDeath).Mean/bump)
			}
		}
	}
}
