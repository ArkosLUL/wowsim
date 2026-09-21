package dps

import (
	"os"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// A rotation saved before Icy Touch and Blood Presence moved to 49909 and 48266 still names 59131 and
// 50689. It has to find the same spells and sim exactly like the current one.
func TestLegacyAPLSpellIDs(t *testing.T) {
	for _, file := range []string{"unholy_dw_ss", "frost_bl_pesti"} {
		data, err := os.ReadFile("../../../ui/deathknight/apls/" + file + ".apl.json")
		if err != nil {
			t.Fatal(err)
		}
		current := string(data)
		legacy := strings.NewReplacer(`"spellId":49909`, `"spellId":59131`, `"spellId":48266`, `"spellId":50689`).Replace(current)
		if legacy == current {
			t.Fatalf("%s names neither id", file)
		}

		currentDps, currentWarnings := simLegacyAPL(t, current)
		legacyDps, legacyWarnings := simLegacyAPL(t, legacy)
		if strings.Join(legacyWarnings, "\n") != strings.Join(currentWarnings, "\n") {
			t.Errorf("%s warns %v with the old ids, %v with the current ones", file, legacyWarnings, currentWarnings)
		}
		if legacyDps != currentDps {
			t.Errorf("%s: %.3f DPS with the old ids, %.3f with the current ones", file, legacyDps, currentDps)
		}
	}
}

func simLegacyAPL(t *testing.T, apl string) (float64, []string) {
	t.Helper()
	player := parityPlayer(UnholyTalents, "p3_uh_dw", apl)
	raid := core.SinglePlayerRaidProto(player, nil, nil, nil)
	encounter := core.MakeSingleTargetEncounter(0)

	var warnings []string
	stats := core.ComputeStats(&proto.ComputeStatsRequest{Raid: raid, Encounter: encounter})
	rotation := stats.RaidStats.Parties[0].Players[0].RotationStats
	for _, action := range append(rotation.PrepullActions, rotation.PriorityList...) {
		warnings = append(warnings, action.Warnings...)
	}

	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 1, IsTest: true},
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	return result.RaidMetrics.Dps.Avg, warnings
}
