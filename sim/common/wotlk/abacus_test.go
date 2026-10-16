package wotlk

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Althor's Abacus heals one ally Echoes of Light (71610) finds around the healed unit, the one with the lowest
// health percent, and a pet that's away isn't in the world to be found: here the Val'kyrs of the Nibelung the
// same player holds.
func TestAbacusHealsTheLowestHealthPercentAndNoPetThatIsAway(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceTauren,
			Class:     proto.Class_ClassDruid,
			Equipment: &proto.EquipmentSpec{Items: []*proto.ItemSpec{{Id: nibelungID}, {Id: 50359}}},
			Consumes:  &proto.Consumes{},
			Spec:      &proto.Player_BalanceDruid{},
			Rotation:  &proto.APLRotation{},
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	owner := sim.Raid.Parties[0].Players[0].(*valkyrTestAgent)
	heal := owner.GetSpell(core.ActionID{SpellID: 71610})
	if heal == nil {
		t.Fatal("no Echoes of Light for a player holding the Abacus")
	}
	var valkyr *ValkyrPet
	for _, petAgent := range owner.PetAgents {
		if v, ok := petAgent.(*ValkyrPet); ok {
			valkyr = v
			break
		}
	}
	var healed []*core.Unit
	heal.ApplyEffects = func(_ *core.Simulation, target *core.Unit, _ *core.Spell) {
		healed = append(healed, target)
	}

	// each proc on a fresh iteration, so the 45 s cooldown never holds one back
	picks := func(summoned bool, hurt map[*core.Unit]float64) map[*core.Unit]int {
		healed = healed[:0]
		for seed := int64(0); seed < 200; seed++ {
			sim.Reseed(300 + seed)
			sim.Reset()
			sim.PrePull()
			if summoned {
				valkyr.registerSmite(false)
				valkyr.EnableWithTimeout(sim, valkyr, 30*time.Second)
			}
			for unit, pct := range hurt {
				unit.RemoveHealth(sim, pct*unit.MaxHealth())
			}
			owner.OnHealDealt(sim, heal, heal.NewResult(&owner.Unit))
			sim.Cleanup()
		}
		counts := map[*core.Unit]int{}
		for _, unit := range healed {
			counts[unit]++
		}
		return counts
	}

	away := picks(false, nil)
	if away[&owner.Unit] == 0 || len(away) != 1 {
		t.Errorf("%d heals with every Val'kyr away, %d of them on the owner: want all", len(healed), away[&owner.Unit])
	}
	out := picks(true, nil)
	if out[&valkyr.Unit] == 0 || len(out) != 2 {
		t.Errorf("one Val'kyr out: %v heals on it, %d on the owner, %d units healed in all: want it and the owner only",
			out[&valkyr.Unit], out[&owner.Unit], len(out))
	}

	// spell_item_echoes_of_light keeps the lowest health percent of the units found, whoever was healed
	if valkyr.MaxHealth() <= 0 {
		t.Fatal("the Val'kyr has no health to take off")
	}
	for _, c := range []struct {
		ownerDown, valkyrDown float64
		want                  *core.Unit
	}{
		{0.1, 0, &owner.Unit},
		{0, 0.1, &valkyr.Unit},
		{0.2, 0.3, &valkyr.Unit},
		{0.3, 0.2, &owner.Unit},
	} {
		got := picks(true, map[*core.Unit]float64{&owner.Unit: c.ownerDown, &valkyr.Unit: c.valkyrDown})
		if got[c.want] == 0 || got[c.want] != len(healed) {
			t.Errorf("owner down %v, Val'kyr down %v: %d of %d heals on %s, want all", c.ownerDown, c.valkyrDown,
				got[c.want], len(healed), c.want.Label)
		}
	}
}
