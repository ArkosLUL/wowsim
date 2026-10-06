package mage

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Torment the Weak needs an attack speed slow on the target; the boss takes no snares.
func TestTormentTheWeakNeedsAnAttackSpeedSlow(t *testing.T) {
	bonus := func(debuffs *proto.Debuffs) float64 {
		raid := core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTroll,
			Class:         proto.Class_ClassMage,
			Equipment:     core.GetGearSet("../../ui/mage/gear_sets", "p3_fire_alliance").GearSet,
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsFire,
			Rotation:      core.GetAplRotation("../../ui/mage/apls", "fire").Rotation,
			TalentsString: FireTalents,
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, debuffs)
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid:       raid,
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		mage := sim.Raid.Parties[0].Players[0].(*Mage)
		if mage.Talents.TormentTheWeak != 3 {
			t.Fatalf("FireTalents has %d points of Torment the Weak, want 3", mage.Talents.TormentTheWeak)
		}
		return mage.tormentTheWeak(sim.Encounter.TargetUnits[0])
	}

	if got := bonus(&proto.Debuffs{}); got != 1 {
		t.Errorf("no debuffs: x%v, want x1", got)
	}
	for name, debuffs := range map[string]*proto.Debuffs{
		"Thunder Clap":           {ThunderClap: proto.TristateEffect_TristateEffectRegular},
		"Frost Fever":            {FrostFever: proto.TristateEffect_TristateEffectRegular},
		"Infected Wounds":        {InfectedWounds: true},
		"Judgements of the Just": {JudgementsOfTheJust: true},
	} {
		if got := bonus(debuffs); got < 1.12-1e-9 || got > 1.12+1e-9 {
			t.Errorf("%s: x%v, want x1.12", name, got)
		}
	}
}
