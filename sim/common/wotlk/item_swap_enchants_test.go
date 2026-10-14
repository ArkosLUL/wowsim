package wotlk_test

import (
	"math"
	"testing"

	_ "github.com/wowsims/wotlk/sim/common"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/deathknight/dps"
)

func init() {
	dps.RegisterDpsDeathknight()
}

// swapDK is a death knight wearing worn in the main hand with bag to swap to, against an undead target. A night
// elf, whose racials read no weapon.
func swapDK(t *testing.T, worn, bag *proto.ItemSpec) (*core.Simulation, *core.Character) {
	t.Helper()
	items := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotMainHand] = worn
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Targets[0].MobType = proto.MobType_MobTypeUndead
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:           proto.Race_RaceNightElf,
			Class:          proto.Class_ClassDeathknight,
			Equipment:      &proto.EquipmentSpec{Items: items},
			Consumes:       &proto.Consumes{},
			Spec:           &proto.Player_Deathknight{Deathknight: &proto.Deathknight{Options: &proto.Deathknight_Options{}}},
			Rotation:       core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			EnableItemSwap: true,
			ItemSwap:       &proto.ItemSwap{MhItem: bag},
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].GetCharacter()
}

// swapState is what the static enchant effects of a weapon change.
type swapState struct {
	stats        stats.Stats
	defenseSkill int32
	mobTypeAP    float64
	weaponMin    float64
}

func swapStateOf(c *core.Character) swapState {
	return swapState{c.GetStats(), c.PseudoStats.BonusDefenseSkill, c.PseudoStats.MobTypeAttackPower, c.AutoAttacks.MH().BaseDamageMin}
}

func (s swapState) differs(o swapState) string {
	for i := range s.stats {
		if math.Abs(s.stats[i]-o.stats[i]) > 1e-6*max(1, math.Abs(o.stats[i])) {
			return stats.Stat(i).StatName()
		}
	}
	switch {
	case s.defenseSkill != o.defenseSkill:
		return "defense skill"
	case s.mobTypeAP != o.mobTypeAP:
		return "attack power against undead"
	case s.weaponMin != o.weaponMin:
		return "weapon damage"
	}
	return ""
}

// Player::SwapItem takes the leaving weapon's enchants off and puts the new one's on (_ApplyItemMods), static
// effects too: a swap leaves what wearing the other weapon from the start gives, swapping back what wearing
// the first one does, and an iteration that ends swapped starts the next on the first weapon.
func TestItemSwapMovesStaticEnchants(t *testing.T) {
	for _, c := range []struct {
		name      string
		worn, bag *proto.ItemSpec
	}{
		// Stoneskin Gargoyle's defense skill and Nerubian Carapace's defense rating, each with its stamina percent
		{"runes", &proto.ItemSpec{Id: 40384, Enchant: 3847}, &proto.ItemSpec{Id: 40343, Enchant: 3883}},
		// Major Striking's weapon damage and Scourgebane's attack power against undead
		{"weapon enchants", &proto.ItemSpec{Id: 40384, Enchant: 963}, &proto.ItemSpec{Id: 40343, Enchant: 3247}},
		{"to a bare weapon", &proto.ItemSpec{Id: 40384, Enchant: 3883}, &proto.ItemSpec{Id: 40343}},
	} {
		t.Run(c.name, func(t *testing.T) {
			sim, dk := swapDK(t, c.worn, c.bag)
			_, other := swapDK(t, c.bag, c.worn)
			first, second := swapStateOf(dk), swapStateOf(other)
			if first.differs(second) == "" {
				t.Fatal("the two weapons give the same, nothing to tell")
			}
			mainHand := []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand}

			dk.ItemSwap.SwapItems(sim, mainHand)
			if what := swapStateOf(dk).differs(second); what != "" {
				t.Errorf("after the swap: %s isn't what the other weapon gives", what)
			}
			dk.ItemSwap.SwapItems(sim, mainHand)
			if what := swapStateOf(dk).differs(first); what != "" {
				t.Errorf("after swapping back: %s isn't what the first weapon gives", what)
			}
			dk.ItemSwap.SwapItems(sim, mainHand)
			sim.Cleanup()
			sim.Reset()
			if what := swapStateOf(dk).differs(first); what != "" {
				t.Errorf("after an iteration that ended swapped: %s isn't what the first weapon gives", what)
			}
		})
	}
}
