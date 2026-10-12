package elemental

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Frost Shock (49236) slows its target for 8 s with MECHANIC_SNARE, which a mage's Torment the Weak counts.
// A target immune to snares takes the damage alone.
func TestFrostShockMarksItsTargetSnared(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 60
	encounter.Targets = []*proto.Target{
		{Level: 80, MobType: proto.MobType_MobTypeHumanoid},
		{Level: 83, MobType: proto.MobType_MobTypeHumanoid},
	}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTroll,
			Class:         proto.Class_ClassShaman,
			Equipment:     &proto.EquipmentSpec{},
			Spec:          PlayerOptionsAdaptive,
			Glyphs:        &proto.Glyphs{},
			TalentsString: StandardTalents,
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	shaman := sim.Raid.Parties[0].Players[0].(*ElementalShaman)
	add, boss := sim.Encounter.TargetUnits[0], sim.Encounter.TargetUnits[1]
	// no miss
	shaman.AddStatsDynamic(sim, stats.Stats{stats.SpellHit: 20 * core.SpellHitRatingPerHitChance})

	for _, target := range []*core.Unit{add, boss} {
		shaman.GCD.Reset()
		shaman.FrostShock.CD.Reset()
		if !shaman.FrostShock.Cast(sim, target) {
			t.Fatal("Frost Shock didn't cast")
		}
	}
	if boss.HasActiveAuraWithTag(core.SnareAuraTag) {
		t.Error("the boss is marked snared, and it's immune to snares")
	}
	snares := add.GetAurasWithTag(core.SnareAuraTag)
	if len(snares) != 1 || !snares[0].IsActive() {
		t.Fatalf("the add isn't marked snared after Frost Shock: %d snare auras", len(snares))
	}
	if left := snares[0].RemainingDuration(sim); left > 8*time.Second+11*time.Millisecond || left < 7900*time.Millisecond {
		t.Errorf("the snare has %v left, want the shock's 8 s", left)
	}
}
