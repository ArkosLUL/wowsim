package wotlk

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// SpellInfo::CheckTarget keeps the dead out of Echoes of Light's area search, lowest health percent or not, and
// with no one left it heals no one.
func TestAbacusSkipsTheDead(t *testing.T) {
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

	for _, c := range []struct {
		name                  string
		ownerDown, valkyrDown float64 // share of max health taken off, 1 kills
		summoned              bool
		want                  *core.Unit // nil: no heal
	}{
		{"a dead Val'kyr", 0.5, 1, true, &owner.Unit},
		{"a dead owner", 1, 0.1, true, &valkyr.Unit},
		{"no one alive", 1, 1, true, nil},
		{"the owner dead, every Val'kyr away", 1, 0, false, nil},
	} {
		healed = healed[:0]
		// each proc on a fresh iteration, so the 45 s cooldown never holds one back
		for seed := int64(0); seed < 50; seed++ {
			sim.Reseed(300 + seed)
			sim.Reset()
			sim.PrePull()
			if c.summoned {
				valkyr.registerSmite(false)
				valkyr.EnableWithTimeout(sim, valkyr, 30*time.Second)
				valkyr.RemoveHealth(sim, c.valkyrDown*valkyr.MaxHealth())
			}
			owner.RemoveHealth(sim, c.ownerDown*owner.MaxHealth())
			owner.OnHealDealt(sim, heal, heal.NewResult(&owner.Unit))
			sim.Cleanup()
		}
		switch {
		case c.want == nil && len(healed) > 0:
			t.Errorf("%s: %d heals, want none", c.name, len(healed))
		case c.want == nil:
		case len(healed) == 0:
			t.Errorf("%s: no heals, want them on %s", c.name, c.want.Label)
		default:
			for _, unit := range healed {
				if unit != c.want {
					t.Errorf("%s: healed %s, want %s", c.name, unit.Label, c.want.Label)
					break
				}
			}
		}
	}
}
