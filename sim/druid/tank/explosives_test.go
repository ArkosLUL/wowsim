package tank

import (
	"os"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

const (
	testSaroniteBomb  = 56350
	testThermalSapper = 56488
)

func simExplosives(t *testing.T, apl string, iterations int32) *proto.UnitMetrics {
	t.Helper()
	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTauren,
			Class:         proto.Class_ClassDruid,
			Equipment:     core.GetGearSet("../../../ui/feral_tank_druid/gear_sets", "p1").GearSet,
			Consumes:      FullConsumes,
			Spec:          PlayerOptionsDefault,
			Glyphs:        StandardGlyphs,
			TalentsString: StandardTalents,
			Profession1:   proto.Profession_Engineering,
			Rotation:      core.APLRotationFromJsonString(apl),
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: 101, IsTest: true},
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	return result.RaidMetrics.Parties[0].Players[0]
}

func explosiveTargets(t *testing.T, player *proto.UnitMetrics, spellID int32) *proto.TargetedActionMetrics {
	t.Helper()
	for _, action := range player.Actions {
		if action.Id.GetSpellId() != spellID {
			continue
		}
		for _, target := range action.Targets {
			// the encounter's target, not the druid the sapper hits too
			if target.UnitIndex == 0 {
				return target
			}
		}
	}
	t.Fatalf("no hit of %d on the target", spellID)
	return nil
}

// A rotation saved while the explosives went by their items still names 42641. It has to find the sapper's
// spell and sim like the current one.
func TestRotationNamingTheSappersItemStillCastsIt(t *testing.T) {
	data, err := os.ReadFile("../../../ui/feral_tank_druid/apls/default.apl.json")
	if err != nil {
		t.Fatal(err)
	}
	current := string(data)
	legacy := strings.Replace(current, `"spellId":56488`, `"itemId":42641`, 1)
	if legacy == current {
		t.Fatal("the default rotation doesn't name the sapper's spell")
	}

	now, old := simExplosives(t, current, 20), simExplosives(t, legacy, 20)
	if explosiveTargets(t, old, testThermalSapper).Casts == 0 {
		t.Error("the rotation naming the item never cast the sapper")
	}
	if now.Dps.Avg != old.Dps.Avg {
		t.Errorf("%.3f DPS naming the item, %.3f naming the spell", old.Dps.Avg, now.Dps.Avg)
	}
}

// Saronite Bomb (56350) has no damage class, so it always lands and never crits. The sapper (56488) is a
// magic spell: it misses and crits like one.
func TestSaroniteBombAlwaysLandsAndNeverCrits(t *testing.T) {
	apl := `{"type": "TypeAPL", "priorityList": [{"action": {"autocastOtherCooldowns": {}}}]}`
	player := simExplosives(t, apl, 2000)

	bomb := explosiveTargets(t, player, testSaroniteBomb)
	if bomb.Casts == 0 || bomb.Misses != 0 || bomb.Crits != 0 {
		t.Errorf("Saronite Bomb: %d casts, %d misses, %d crits, want every one a plain hit", bomb.Casts, bomb.Misses, bomb.Crits)
	}
	sapper := explosiveTargets(t, player, testThermalSapper)
	if sapper.Misses == 0 || sapper.Crits == 0 {
		t.Errorf("the sapper: %d misses and %d crits in %d casts, want some of each", sapper.Misses, sapper.Crits, sapper.Casts)
	}
}
