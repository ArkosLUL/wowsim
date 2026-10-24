package wotlk_test

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/priest/healing"
)

const talismanOfTrollDivinity = 37734

// Touched by a Troll (60518) goes on the heal's target a stack a direct heal, 58 healing taken a stack, up to 5.
// A hot going on or ticking is no direct heal.
func TestTalismanOfTrollDivinityStacksOnDirectHeals(t *testing.T) {
	items := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotTrinket1] = &proto.ItemSpec{Id: talismanOfTrollDivinity}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceUndead,
			Class:     proto.Class_ClassPriest,
			Equipment: &proto.EquipmentSpec{Items: items},
			Consumes:  &proto.Consumes{},
			Spec:      &proto.Player_HealingPriest{HealingPriest: &proto.HealingPriest{Options: &proto.HealingPriest_Options{}}},
			Rotation:  core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*healing.HealingPriest)
	target := &priest.Unit
	mana := priest.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen})
	cast := func(spell *core.Spell) {
		t.Helper()
		priest.AddMana(sim, priest.MaxMana()-priest.CurrentMana(), mana)
		priest.GCD.Reset()
		if !spell.Cast(sim, target) {
			t.Fatalf("%s didn't cast", spell.ActionID)
		}
		for until := sim.CurrentTime + 2*time.Second; sim.CurrentTime < until && !sim.Step(); {
		}
	}
	stacks := func() int32 { return target.GetAura("Touched by a Troll").GetStacks() }
	taken := func() float64 { return target.PseudoStats.BonusHealingTaken }

	talisman := priest.GetSpell(core.ActionID{ItemID: talismanOfTrollDivinity})
	if !talisman.Cast(sim, nil) {
		t.Fatal("the Talisman didn't cast")
	}

	// the hot's landing deals no heal, and its first tick is 3 s away
	cast(priest.Renew)
	if got := stacks(); got != 0 {
		t.Errorf("%d stacks after Renew went on, want 0", got)
	}

	for want := int32(1); want <= 5; want++ {
		cast(priest.FlashHeal)
		if got := stacks(); got != want {
			t.Fatalf("%d stacks after %d Flash Heals, want %d", got, want, want)
		}
		if got := taken(); got != 58*float64(want) {
			t.Errorf("%v flat healing taken at %d stacks, want %v", got, want, 58*float64(want))
		}
	}

	cast(priest.FlashHeal)
	if got, want := stacks(), int32(5); got != want {
		t.Errorf("%d stacks after a sixth heal, want the cap of %d", got, want)
	}

	for until := sim.CurrentTime + 11*time.Second; sim.CurrentTime < until && !sim.Step(); {
	}
	if got := stacks(); got != 0 {
		t.Errorf("%d stacks left 11 s on, want 0", got)
	}
	if got := taken(); got != 0 {
		t.Errorf("%v flat healing taken after the stacks ended, want 0", got)
	}
}
