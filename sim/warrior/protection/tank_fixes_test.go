package protection

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func newTankFixesWarrior(glyphs *proto.Glyphs) (*core.Simulation, *ProtectionWarrior) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassWarrior,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsBasic,
			TalentsString: DefaultTalents,
			Glyphs:        glyphs,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	return sim, sim.Raid.Parties[0].Players[0].(*ProtectionWarrior)
}

// Last Stand's 15,000 (30% of 50,000) comes off on expiry as AuraEffect::HandleAuraModIncreaseHealth takes it off.
func TestLastStandBonusHealthOnExpiry(t *testing.T) {
	sim, warrior := newTankFixesWarrior(DefaultGlyphs)
	aura := warrior.GetAura("Last Stand")
	if aura == nil {
		t.Fatal("no Last Stand")
	}
	metrics := warrior.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel})
	// health with the bonus up
	for _, tc := range []struct{ health, want float64 }{
		{65000, 50000},
		{20000, 5000},
		{15000, 1},
		{0, 0},
	} {
		if tc.health != 65000 {
			sim.Cleanup()
		}
		sim.Reset()
		sim.PrePull()
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		warrior.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - warrior.MaxHealth()})
		warrior.GainHealth(sim, warrior.MaxHealth(), metrics)

		aura.Activate(sim)
		if warrior.MaxHealth() != 65000 || warrior.CurrentHealth() != 65000 {
			t.Fatalf("%v of %v health with Last Stand up, want 65000 of 65000", warrior.CurrentHealth(), warrior.MaxHealth())
		}
		if tc.health < 65000 {
			warrior.RemoveHealth(sim, 65000-tc.health)
		}
		aura.Deactivate(sim)
		if warrior.CurrentHealth() != tc.want || warrior.MaxHealth() != 50000 {
			t.Errorf("at %v: %v of %v health after it, want %v of 50000", tc.health, warrior.CurrentHealth(), warrior.MaxHealth(), tc.want)
		}
	}
}

// 12975's category cooldown is 180 s, and Glyph of Last Stand (58376) takes 60 s off.
func TestLastStandCooldown(t *testing.T) {
	for _, tc := range []struct {
		glyphs *proto.Glyphs
		want   time.Duration
	}{
		{DefaultGlyphs, 3 * time.Minute},
		{&proto.Glyphs{Major1: int32(proto.WarriorMajorGlyph_GlyphOfLastStand)}, 2 * time.Minute},
	} {
		_, warrior := newTankFixesWarrior(tc.glyphs)
		if got := warrior.GetSpell(core.ActionID{SpellID: 12975}).CD.Duration; got != tc.want {
			t.Errorf("glyphs %v: cooldown %v, want %v", tc.glyphs, got, tc.want)
		}
	}
}
