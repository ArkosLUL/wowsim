package healing

import (
	"os"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// HEAL_MEASURE=1 prints the page's default incoming damage for each phase: the effective HPS of the page request
// when the damage is high enough that nothing overheals. Paste the rounded numbers into the page's
// incoming_damage.json.
func TestMeasureDefaultIncomingDamage(t *testing.T) {
	if os.Getenv("HEAL_MEASURE") == "" {
		t.Skip("set HEAL_MEASURE=1 to measure")
	}
	for _, phase := range []string{"p1", "p2", "p3", "p4", "p5"} {
		for _, tree := range []string{"disc", "holy"} {
			rsr := pageRequest(tree, phase)
			rsr.Raid.IncomingDamagePerSecond = 1e6
			rsr.SimOptions = &proto.SimOptions{Iterations: 3000, IsTest: true, RandomSeed: 424242}
			result := core.RunRaidSim(rsr)
			if result.ErrorResult != "" {
				t.Fatal(result.ErrorResult)
			}
			t.Logf("MEASURE %s %s %.0f", tree, phase, result.RaidMetrics.Parties[0].Players[0].Hps.Avg)
		}
	}
}
