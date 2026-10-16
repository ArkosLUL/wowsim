package optimizer

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// A tank's gear resists nothing: its pool prices items, enchants and gems without their resistance,
// and every sim and sheet takes the gear's back off, while Gift of the Wild's stays. Anyone else's
// gear keeps its resistance.
func TestTankGearResistsNothing(t *testing.T) {
	const (
		head, chest = proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotChest
		icebane     = 43586 // Icebane Chestguard: 169 Stamina, 115 Frost Resistance
		frostySoul  = 3812  // Arcanum of the Frosty Soul: 30 Stamina, 25 Frost Resistance
		voidSphere  = 22459 // 4 of every resistance
	)
	for _, tank := range []bool{true, false} {
		player := protPaladin(t, "p3")
		if !tank {
			player.Spec = &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.RetributionPaladin_Options{}}}
		}
		req := tankOptimizeRequest(t, player, 3)
		heaume := player.Equipment.Items[head].Id
		req.Pool = &proto.CandidatePool{
			Slots: []*proto.SlotPool{
				{Slot: head, ItemIds: []int32{heaume}, EnchantOptions: []*proto.ItemEnchantOptions{{ItemId: heaume, EnchantIds: []int32{frostySoul}}}},
				{Slot: chest, ItemIds: []int32{icebane}},
			},
			GemIds: []int32{voidSphere},
		}
		r, err := PrepareRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		pool, err := CompilePool(r)
		if err != nil {
			t.Fatal(err)
		}

		priced := func(want float64) float64 { return map[bool]float64{true: 0, false: want}[tank] }
		item := pool.candidate(chest, icebane).Item
		if item.Stats[stats.FrostResistance] != priced(115) || item.Stats[stats.Stamina] != 169 {
			t.Errorf("tank %v: Icebane prices %v Frost Resistance and %v Stamina", tank, item.Stats[stats.FrostResistance], item.Stats[stats.Stamina])
		}
		enchant := pool.candidate(head, heaume).Enchants[0].Stats
		if enchant[stats.FrostResistance] != priced(25) || enchant[stats.Stamina] != 30 {
			t.Errorf("tank %v: the arcanum prices %v Frost Resistance and %v Stamina", tank, enchant[stats.FrostResistance], enchant[stats.Stamina])
		}
		if gem := pool.gem(voidSphere).Stats; gem[stats.ShadowResistance] != priced(4) {
			t.Errorf("tank %v: Void Sphere prices %v Shadow Resistance", tank, gem[stats.ShadowResistance])
		}

		wearing := r.Seed
		wearing.Items[chest] = ItemChoice{ItemID: icebane}
		wearing.Items[head].Enchant = frostySoul
		wearing.Items[head].Gems[1] = voidSphere
		bare := r.Seed
		bare.Items[chest] = ItemChoice{}
		bare.Items[head].Enchant = 0
		bare.Items[head].Gems[1] = 0
		frost := func(l Loadout) float64 {
			rsr := NewSimEvaluator(r).request(Point{Loadout: l}, 0, 1)
			sheet, err := core.ComputePlayerSheet(rsr.Raid, rsr.Encounter, r.TargetIndex)
			if err != nil {
				t.Fatal(err)
			}
			if fromPool, err := pool.sheet(l); err != nil || fromPool.FinalStats[stats.FrostResistance] != sheet.FinalStats[stats.FrostResistance] {
				t.Errorf("tank %v: the pool's sheet resists %v frost, the sim's %v (%v)", tank, fromPool.FinalStats[stats.FrostResistance], sheet.FinalStats[stats.FrostResistance], err)
			}
			return sheet.FinalStats[stats.FrostResistance]
		}
		auras, worn := frost(bare), frost(wearing)
		if want := auras + priced(115+25+4); math.Abs(worn-want) > 1e-9 || auras <= 0 {
			t.Errorf("tank %v: %v Frost Resistance wearing it all, %v bare; want %v, and the raid's above 0", tank, worn, auras, want)
		}
		if tank {
			if heaumeFrost := frost(r.Seed) - frost(Loadout{RacialTraits: r.Seed.RacialTraits}); heaumeFrost != 0 {
				t.Errorf("the seed's heaume still resists %v frost", heaumeFrost)
			}
		}
		if got := gearResistance(wearing)[stats.FrostResistance] - gearResistance(bare)[stats.FrostResistance]; got != 115+25+4 {
			t.Errorf("tank %v: gearResistance counts %v frost for the three", tank, got)
		}
	}
}
