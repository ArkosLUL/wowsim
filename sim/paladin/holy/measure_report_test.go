package holy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/statslopes"
)

// HEAL_REPORT=1 prints, per phase, what the page request heals at the page's default damage, and its "Measure at
// my gear" slopes in effective HPS a point.
func TestMeasureReport(t *testing.T) {
	if os.Getenv("HEAL_REPORT") == "" {
		t.Skip("set HEAL_REPORT=1 to measure")
	}
	for _, phase := range []string{"preraid", "p1", "p2", "p3", "p4", "p5"} {
		rsr := pageRequest(phase)
		measureReport(t, "holy", phase, rsr)
	}
}

func reportSeed() int64 {
	if v, err := strconv.ParseInt(os.Getenv("HEAL_SEED"), 10, 64); err == nil {
		return v
	}
	return 424242
}

func measureReport(t *testing.T, name, phase string, rsr *proto.RaidSimRequest) {
	if v, err := strconv.ParseFloat(os.Getenv("HEAL_DAMAGE"), 64); err == nil {
		rsr.Raid.IncomingDamagePerSecond = v
	}
	rsr.SimOptions = &proto.SimOptions{Iterations: 3000, IsTest: true, RandomSeed: reportSeed()}
	result := core.RunRaidSim(rsr)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	u := result.RaidMetrics.Parties[0].Players[0]
	t.Logf("REPORT %s %s dmg %.0f eff %.0f raw %.0f overheal%% %.1f unhealed %.0f tto %.0f oom %.0f", name, phase,
		rsr.Raid.IncomingDamagePerSecond, u.Hps.Avg, u.RawHps.Avg, 100*u.Overheal.Avg/u.RawHps.Avg, u.UnhealedDps.Avg, u.Tto.Avg, u.SecondsOomAvg)

	slopes := statslopes.Measure(&proto.StatSlopesRequest{Raid: rsr.Raid, Encounter: rsr.Encounter,
		SimOptions: &proto.SimOptions{Iterations: 3000, RandomSeed: reportSeed()}}, nil)
	if slopes.ErrorResult != "" {
		t.Fatal(slopes.ErrorResult)
	}
	var row []string
	for _, s := range slopes.Slopes {
		if s.Pseudo {
			continue
		}
		row = append(row, fmt.Sprintf("%s %.2f/%.2f", strings.TrimPrefix(s.Stat.String(), "Stat"), s.Gain.Hps, s.Loss.GetHps()))
	}
	t.Logf("SLOPES %s %s %s", name, phase, strings.Join(row, " | "))
}
