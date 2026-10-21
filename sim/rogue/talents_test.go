package rogue

import (
	"math"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// aura 138 divides both hands' attack time (Unit::ApplyAttackTimePercentMod), so 9/16/23% multiplies with any
// other melee speed
func TestLightningReflexesMeleeSpeed(t *testing.T) {
	// the 12th Combat talent
	at := strings.Index(CombatTalents, "-") + 1 + 11
	swingSpeed := func(rank int) float64 {
		talents := []byte(CombatTalents)
		talents[at] = byte('0' + rank)
		sim := core.NewSim(talentsAverageRequest(&proto.Player{
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassRogue,
			Equipment:     core.GetGearSet("../../ui/rogue/gear_sets", "p1_combat").GearSet,
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsCombatDI,
			Rotation:      core.GetAplRotation("../../ui/rogue/apls", "combat_expose").Rotation,
			TalentsString: string(talents),
		}))
		rogue := sim.Raid.Parties[0].Players[0].(RogueAgent).GetRogue()
		if rogue.Talents.LightningReflexes != int32(rank) {
			t.Fatalf("talents %s give Lightning Reflexes %d, want %d", talents, rogue.Talents.LightningReflexes, rank)
		}
		return rogue.SwingSpeed()
	}

	base := swingSpeed(0)
	for rank, want := range []float64{1.09, 1.16, 1.23} {
		if got := swingSpeed(rank+1) / base; math.Abs(got-want) > 1e-9 {
			t.Errorf("rank %d: swing speed x%.4f, want x%.2f", rank+1, got, want)
		}
	}
}
