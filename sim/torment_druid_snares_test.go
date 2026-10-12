package sim

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/druid/balance"
	"github.com/wowsims/wotlk/sim/mage"
)

// A druid's Typhoon snares the add it lands on, and its Hurricane slows the attacks of every creature in the
// storm. Torment the Weak counts both whoever cast them, but the raid boss is snare immune and not attack-slow
// immune.
func TestTormentTheWeakCountsADruidsTyphoonAndHurricane(t *testing.T) {
	idle := core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`)
	moonkin := &proto.Player{
		Race:               proto.Race_RaceTauren,
		Class:              proto.Class_ClassDruid,
		Equipment:          core.GetGearSet("../ui/balance_druid/gear_sets", "p1").GearSet,
		Consumes:           &proto.Consumes{},
		Spec:               &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{OkfUptime: 0.2}}},
		Rotation:           idle,
		TalentsString:      "5012203105331303213315311231--205003012",
		DistanceFromTarget: 30,
	}
	firemage := &proto.Player{
		Race:               proto.Race_RaceTroll,
		Class:              proto.Class_ClassMage,
		Equipment:          core.GetGearSet("../ui/mage/gear_sets", "p3_fire_alliance").GearSet,
		Consumes:           &proto.Consumes{},
		Spec:               &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{Armor: proto.Mage_Options_MoltenArmor}}},
		Rotation:           idle,
		TalentsString:      "23000503110003-0055030012303331053120301351",
		DistanceFromTarget: 30,
	}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Targets = append(encounter.Targets, &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid})

	newSim := func() (*core.Simulation, *mage.Mage, *balance.BalanceDruid) {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{firemage, moonkin}, Buffs: &proto.PartyBuffs{}}},
				Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
			Encounter:  encounter,
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		return sim, sim.Raid.Parties[0].Players[0].(*mage.Mage), sim.Raid.Parties[0].Players[1].(*balance.BalanceDruid)
	}
	wantTorment := func(when string, m *mage.Mage, target *core.Unit, want float64) {
		t.Helper()
		if got := m.TormentTheWeakBonus(target); got < want-1e-9 || got > want+1e-9 {
			t.Errorf("%s: x%v, want x%v", when, got, want)
		}
	}
	advance := func(sim *core.Simulation, by time.Duration) {
		for until := sim.CurrentTime + by; sim.CurrentTime < until && !sim.Step(); {
		}
	}

	sim, m, moon := newSim()
	boss, add := sim.GetTargetUnit(0), sim.GetTargetUnit(1)
	if boss.ImmuneToMechanic(core.MechanicSnare) == add.ImmuneToMechanic(core.MechanicSnare) {
		t.Fatalf("snare immune: boss %v, add %v", boss.ImmuneToMechanic(core.MechanicSnare), add.ImmuneToMechanic(core.MechanicSnare))
	}
	wantTorment("the boss before any slow", m, boss, 1)
	wantTorment("the add before any slow", m, add, 1)

	// the hit can miss, so cast it until the add is snared
	for try := 0; try < 8 && m.TormentTheWeakBonus(add) == 1; try++ {
		moon.Typhoon.CD.Reset()
		moon.Typhoon.Cast(sim, boss)
		advance(sim, 2*time.Second)
	}
	wantTorment("the add after Typhoon", m, add, 1.12)
	wantTorment("the boss after Typhoon", m, boss, 1)
	advance(sim, 7*time.Second)
	wantTorment("the add after Typhoon's snare ran out", m, add, 1)

	sim, m, moon = newSim()
	boss, add = sim.GetTargetUnit(0), sim.GetTargetUnit(1)
	if !moon.Hurricane.Cast(sim, boss) {
		t.Fatal("Hurricane didn't cast")
	}
	advance(sim, 2*time.Second)
	wantTorment("the boss in Hurricane", m, boss, 1.12)
	wantTorment("the add in Hurricane", m, add, 1.12)
	advance(sim, 11*time.Second)
	wantTorment("the boss after Hurricane ended", m, boss, 1)
}
