package shadow

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Mind Flay's channel (48156) carries a slow with MECHANIC_SNARE, which a mage's Torment the Weak counts on a
// target that takes snares. The sim marks the channel's aura on the target for it.
func TestMindFlayMarksItsTargetSnared(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 60
	encounter.Targets = []*proto.Target{{Level: 80, MobType: proto.MobType_MobTypeHumanoid}}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Spec:          PlayerOptionsBasic,
			Glyphs:        &proto.Glyphs{},
			TalentsString: DefaultTalents,
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*ShadowPriest)
	target := sim.Encounter.TargetUnits[0]
	// no miss
	priest.AddStatsDynamic(sim, stats.Stats{stats.SpellHit: 20 * core.SpellHitRatingPerHitChance})

	if target.HasActiveAuraWithTag(core.SnareAuraTag) {
		t.Fatal("the target is snared before any cast")
	}
	mindFlay := priest.MindFlay[3]
	mindFlay.Cast(sim, target)
	if !mindFlay.Dot(target).IsActive() {
		t.Fatal("Mind Flay's channel never started")
	}
	if !target.HasActiveAuraWithTag(core.SnareAuraTag) {
		t.Error("the target isn't marked snared while Mind Flay channels")
	}
	mindFlay.Dot(target).Cancel(sim)
	if target.HasActiveAuraWithTag(core.SnareAuraTag) {
		t.Error("the target stays marked snared after the channel")
	}
}
