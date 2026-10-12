package sim

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/druid/balance"
	"github.com/wowsims/wotlk/sim/mage"
)

// A druid's Typhoon snares the add it lands on, and so does its Hurricane, at 0% without the glyph. Torment the
// Weak counts both whoever cast them, but not on the snare-immune raid boss: Hurricane's rank 5 slows the boss's
// attacks without MECHANIC_SLOW_ATTACK, which only ranks 1 to 4 have. The boss takes the storm's aura without
// its snare, so the storm drops it at every other look at its targets: live, it was on the boss dummy in about
// half the probes of a channel.
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

	newSimWith := func(debuffs *proto.Debuffs) (*core.Simulation, *mage.Mage, *balance.BalanceDruid) {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{firemage, moonkin}, Buffs: &proto.PartyBuffs{}}},
				Buffs: &proto.RaidBuffs{}, Debuffs: debuffs},
			Encounter:  encounter,
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		return sim, sim.Raid.Parties[0].Players[0].(*mage.Mage), sim.Raid.Parties[0].Players[1].(*balance.BalanceDruid)
	}
	newSim := func() (*core.Simulation, *mage.Mage, *balance.BalanceDruid) { return newSimWith(&proto.Debuffs{}) }
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
	wantSwingSpeed := func(when string, unit *core.Unit, want float64) {
		t.Helper()
		if got := unit.SwingSpeed(); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: swing speed x%v, want x%v", when, got, want)
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
	bossSpeed, addSpeed := boss.SwingSpeed(), add.SwingSpeed()
	// the storm looks at its targets every 500 ms, counted on its map updates
	firstUpdate := sim.NextServerTick(sim.CurrentTime)
	mapUpdate := sim.NextServerTick(firstUpdate+1) - firstUpdate
	pass := (500*time.Millisecond + mapUpdate - 1) / mapUpdate * mapUpdate
	if mapUpdate < 50*time.Millisecond || pass < 500*time.Millisecond {
		t.Fatalf("map updates %v apart and a pass every %v", mapUpdate, pass)
	}
	cast := sim.CurrentTime
	if !moon.Hurricane.Cast(sim, boss) {
		t.Fatal("Hurricane didn't cast")
	}
	wantSwingSpeed("the boss as Hurricane goes off", boss, bossSpeed/1.2)
	slowed := true
	var flips []time.Duration
	for sim.CurrentTime < cast+7*time.Second && !sim.Step() {
		wantTorment("the boss in Hurricane", m, boss, 1)
		wantTorment("the add in Hurricane", m, add, 1.12)
		wantSwingSpeed("the add in Hurricane", add, addSpeed/1.2)
		now := boss.SwingSpeed() < bossSpeed*0.99
		if now != slowed {
			slowed = now
			flips = append(flips, sim.CurrentTime-cast)
		}
		want := bossSpeed
		if slowed {
			want = bossSpeed / 1.2
		}
		wantSwingSpeed("the boss in Hurricane", boss, want)
	}
	if len(flips) < 13 {
		t.Fatalf("the boss's slow came or went %d times in 7 s, want every %v: %v", len(flips), pass, flips)
	}
	for i, flip := range flips {
		if want := firstUpdate - cast + pass - mapUpdate + time.Duration(i)*pass; flip != want {
			t.Errorf("the boss's slow changed %v into the channel, want %v (change %d)", flip, want, i)
		}
	}
	advance(sim, 3*time.Second)
	wantTorment("the add after Hurricane ended", m, add, 1)
	wantSwingSpeed("the boss after Hurricane ended", boss, bossSpeed)

	// under Hurricane a weaker Thunder Clap still counts, and only the stronger of the two slows the swing
	// (spell_group 1059)
	sim, m, moon = newSimWith(&proto.Debuffs{ThunderClap: proto.TristateEffect_TristateEffectRegular})
	boss = sim.GetTargetUnit(0)
	clapped := boss.SwingSpeed()
	wantTorment("the boss under Thunder Clap", m, boss, 1.12)
	if !moon.Hurricane.Cast(sim, boss) {
		t.Fatal("Hurricane didn't cast")
	}
	wantTorment("the boss under Thunder Clap in Hurricane", m, boss, 1.12)
	wantSwingSpeed("the boss under Thunder Clap in Hurricane", boss, clapped*1.1/1.2)
	// past the storm's first pass, which takes its aura off the boss
	advance(sim, 600*time.Millisecond)
	wantTorment("the boss under Thunder Clap with the storm's aura off", m, boss, 1.12)
	wantSwingSpeed("the boss under Thunder Clap with the storm's aura off", boss, clapped)
	advance(sim, 12*time.Second)
	wantSwingSpeed("the boss under Thunder Clap after Hurricane ended", boss, clapped)
}
