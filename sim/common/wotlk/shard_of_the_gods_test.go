package wotlk_test

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/priest/healing"
)

func init() {
	healing.RegisterHealingPriest()
}

// A Renew deals nothing as it goes on, and still rolls the Purified set's bonus (69755), as any landed magic-class
// cast does: Cauterizing Heal (69733) comes off Renews alone, 50 s apart at least.
func TestShardOfTheGodsProcsOffAHotAlone(t *testing.T) {
	items := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotTrinket1] = &proto.ItemSpec{Id: 49310}
	items[proto.ItemSlot_ItemSlotTrinket2] = &proto.ItemSpec{Id: 49463}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 600
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceUndead,
			Class:     proto.Class_ClassPriest,
			Equipment: &proto.EquipmentSpec{Items: items},
			Consumes:  &proto.Consumes{},
			Spec:      &proto.Player_HealingPriest{HealingPriest: &proto.HealingPriest{Options: &proto.HealingPriest_Options{}}},
			Rotation:  core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*healing.HealingPriest)

	var procs []time.Duration
	priest.RegisterAura(core.Aura{
		Label:    "shard test",
		Duration: core.NeverExpires,
		OnHealDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, _ *core.SpellResult) {
			if spell.ActionID.SpellID == 69733 {
				procs = append(procs, sim.CurrentTime)
			}
		},
	}).Activate(sim)

	mana := priest.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen})
	for range 150 {
		for until := sim.CurrentTime + 3*time.Second; sim.CurrentTime < until && !sim.Step(); {
		}
		priest.AddMana(sim, priest.MaxMana()-priest.CurrentMana(), mana)
		priest.GCD.Reset()
		if !priest.Renew.Cast(sim, &priest.Unit) {
			t.Fatal("Renew didn't cast")
		}
	}

	if len(procs) < 4 {
		t.Fatalf("%d Cauterizing Heals off 150 Renews over 450 s, want one about every 50 to 60 s", len(procs))
	}
	for i := 1; i < len(procs); i++ {
		if gap := procs[i] - procs[i-1]; gap < 50*time.Second {
			t.Errorf("Cauterizing Heal at %s and %s, inside the 50 s cooldown", procs[i-1], procs[i])
		}
	}
}

// Inner Focus (14751) is magic class and lands on the priest with nothing dealt, so it rolls the Purified set's
// bonus too, and the heal goes on the priest.
func TestShardOfTheGodsProcsOffASelfBuff(t *testing.T) {
	items := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotTrinket1] = &proto.ItemSpec{Id: 49310}
	items[proto.ItemSlot_ItemSlotTrinket2] = &proto.ItemSpec{Id: 49463}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{Items: items},
			Consumes:      &proto.Consumes{},
			Spec:          &proto.Player_HealingPriest{HealingPriest: &proto.HealingPriest{Options: &proto.HealingPriest_Options{}}},
			TalentsString: "0503203130300512301313231251-2351010303",
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	priest := sim.Raid.Parties[0].Players[0].(*healing.HealingPriest)
	if priest.InnerFocus == nil {
		t.Fatal("no Inner Focus in the talents")
	}

	var healed []*core.Unit
	watch := priest.RegisterAura(core.Aura{
		Label:    "shard test",
		Duration: core.NeverExpires,
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell.ActionID.SpellID == 69733 {
				healed = append(healed, result.Target)
			}
		},
	})

	// one cast a fresh iteration, so the 50 s cooldown never holds a roll back
	const casts = 80
	for seed := int64(0); seed < casts; seed++ {
		sim.Reseed(500 + seed)
		sim.Reset()
		watch.Activate(sim)
		if !priest.InnerFocus.Cast(sim, priest.CurrentTarget) {
			t.Fatal("Inner Focus didn't cast")
		}
		sim.Cleanup()
	}

	if len(healed) < casts/10 || len(healed) > casts/2 {
		t.Errorf("%d Cauterizing Heals off %d Inner Focus casts, want about a quarter", len(healed), casts)
	}
	for _, target := range healed {
		if target != &priest.Unit {
			t.Errorf("Cauterizing Heal on %s, want the priest", target.Label)
		}
	}
}
