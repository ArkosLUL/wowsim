package tank

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Glyphed Survival Instincts' 22,500 (45% of 50,000) comes off on expiry as AuraEffect::HandleAuraModIncreaseHealth
// takes it off.
func TestSurvivalInstinctsBonusHealthOnExpiry(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTauren,
			Class:         proto.Class_ClassDruid,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsDefault,
			TalentsString: StandardTalents,
			Glyphs:        StandardGlyphs,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	druid := sim.Raid.Parties[0].Players[0].(*FeralTankDruid)
	if druid.SurvivalInstinctsAura == nil {
		t.Fatal("no Survival Instincts")
	}
	metrics := druid.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel})
	// health with the bonus up
	for _, tc := range []struct{ health, want float64 }{
		{72500, 50000},
		{30000, 7500},
		{22500, 1},
		{0, 0},
	} {
		if tc.health != 72500 {
			sim.Cleanup()
		}
		sim.Reset()
		sim.PrePull()
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		druid.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - druid.MaxHealth()})
		druid.GainHealth(sim, druid.MaxHealth(), metrics)

		druid.SurvivalInstinctsAura.Activate(sim)
		if druid.MaxHealth() != 72500 || druid.CurrentHealth() != 72500 {
			t.Fatalf("%v of %v health with Survival Instincts up, want 72500 of 72500", druid.CurrentHealth(), druid.MaxHealth())
		}
		if tc.health < 72500 {
			druid.RemoveHealth(sim, 72500-tc.health)
		}
		druid.SurvivalInstinctsAura.Deactivate(sim)
		if druid.CurrentHealth() != tc.want || druid.MaxHealth() != 50000 {
			t.Errorf("at %v: %v of %v health after it, want %v of 50000", tc.health, druid.CurrentHealth(), druid.MaxHealth(), tc.want)
		}
	}
}
