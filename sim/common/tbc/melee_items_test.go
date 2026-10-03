package tbc_test

import (
	"math"
	"testing"

	simpkg "github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// 21992 is one cast: 300 nature damage on the first target and the debuff jumping to 4 more. The cast's
// level 63 is split between the targets it reached, and the slow it triggers (27648) makes no threat.
func TestThunderfuryThreat(t *testing.T) {
	simpkg.RegisterAll()

	items := make([]*proto.ItemSpec, len(proto.ItemSlot_name))
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotMainHand].Id = 19019
	targets := []*proto.Target{core.NewDefaultTarget(), core.NewDefaultTarget(), core.NewDefaultTarget()}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceHuman,
			Class:     proto.Class_ClassWarrior,
			Equipment: &proto.EquipmentSpec{Items: items},
			Consumes:  &proto.Consumes{},
			Spec:      &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  &proto.Encounter{Duration: 180, Targets: targets},
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 1},
	})
	char := sim.Raid.Parties[0].Players[0].GetCharacter()
	proc := char.GetSpell(core.ActionID{SpellID: 21992})
	if proc == nil {
		t.Fatal("no Thunderfury proc spell")
	}
	proc.Flags |= core.SpellFlagIgnoreResists
	proc.BonusHitRating += 200 * core.SpellHitRatingPerHitChance

	sim.Reset()
	proc.SkipCastAndApplyEffects(sim, sim.Encounter.TargetUnits[0])

	perPoint := char.ThreatSchoolMultiplier(core.SpellSchoolNature)
	for i, target := range sim.Encounter.TargetUnits {
		m := proc.SpellMetrics[target.UnitIndex]
		if i == 0 && m.TotalDamage == 0 || i > 0 && m.TotalDamage != 0 {
			t.Errorf("target %d took %v damage", i, m.TotalDamage)
		}
		if want := m.TotalDamage*perPoint + 63.0/3; math.Abs(m.TotalThreat-want) > 1e-6 {
			t.Errorf("target %d: threat %v, want %v", i, m.TotalThreat, want)
		}
	}
}
