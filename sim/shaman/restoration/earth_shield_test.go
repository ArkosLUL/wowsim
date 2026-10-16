package restoration

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// spell_sha_earth_shield's charge is 49284's SpellHealingBonusDone, which takes Purification's healing done
// percent, and 379 heals for it with no caster mods of its own: 10% more at 5/5 than at 0/5.
func TestEarthShieldChargeTakesPurification(t *testing.T) {
	// StandardTalents without Improved Shields, with Purification as given
	charge := func(talents string, purification int32) float64 {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceTroll,
				Class:         proto.Class_ClassShaman,
				Equipment:     &proto.EquipmentSpec{},
				Consumes:      &proto.Consumes{},
				Spec:          PlayerOptionsStandard,
				Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
				TalentsString: talents,
			}, nil, nil, nil),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		shaman := sim.Raid.Parties[0].Players[0].(*RestorationShaman)
		if shaman.Talents.Purification != purification || shaman.Talents.ImprovedShields != 0 || !shaman.Talents.EarthShield {
			t.Fatalf("Purification %d, Improved Shields %d, Earth Shield %v", shaman.Talents.Purification,
				shaman.Talents.ImprovedShields, shaman.Talents.EarthShield)
		}
		target := &shaman.Unit
		hot := shaman.EarthShield.Hot(target)
		hot.Apply(sim)
		metrics := &shaman.EarthShield.SpellMetrics[target.UnitIndex]
		before := metrics.TotalHealing
		hot.TickOnce(sim)
		return metrics.TotalHealing - before
	}

	with := charge("-3020500-50005331335310501122331251", 5)
	without := charge("-3020500-50005331335310001122331251", 0)
	if without <= 0 || math.Abs(with-1.1*without) > 1.1 {
		t.Errorf("a charge healed %v with Purification 5/5 and %v without, want 10%% more", with, without)
	}
}
