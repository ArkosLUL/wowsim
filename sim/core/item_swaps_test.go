package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const (
	itemTestSwapSocketed = 9100021
	itemTestSwapPlain    = 9100022
	gemTestSwap          = 9100023
)

func init() {
	AddToDatabase(&proto.SimDatabase{
		Items: []*proto.SimItem{
			{
				Id:          itemTestSwapSocketed,
				Type:        proto.ItemType_ItemTypeWeapon,
				HandType:    proto.HandType_HandTypeOneHand,
				Stats:       stats.Stats{stats.Stamina: 30, stats.MeleeCrit: 50, stats.SpellCrit: 50}.ToFloatArray(),
				GemSockets:  []proto.GemColor{proto.GemColor_GemColorRed},
				SocketBonus: stats.Stats{stats.Stamina: 6}.ToFloatArray(),
				ServerStats: []*proto.ItemStat{{StatType: 7, Value: 30}, {StatType: 32, Value: 50}},
			},
			{
				Id:       itemTestSwapPlain,
				Type:     proto.ItemType_ItemTypeWeapon,
				HandType: proto.HandType_HandTypeOneHand,
				Stats:    stats.Stats{stats.Stamina: 40}.ToFloatArray(),
			},
		},
		Gems: []*proto.SimGem{{Id: gemTestSwap, Color: proto.GemColor_GemColorRed, Stats: stats.Stats{stats.Strength: 20}.ToFloatArray()}},
	})
}

func TestItemSwapStatChangesIncludeSocketBonusAndReforge(t *testing.T) {
	character := &Character{}
	character.Equipment[proto.ItemSlot_ItemSlotMainHand] = NewItem(ItemSpec{
		ID:      itemTestSwapSocketed,
		Gems:    []int32{gemTestSwap},
		Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36},
	}, nil)
	swap := ItemSwap{character: character}
	*swap.GetItem(proto.ItemSlot_ItemSlotMainHand) = NewItem(ItemSpec{ID: itemTestSwapPlain}, nil)

	got := swap.CalcStatChanges([]proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand})

	// 40 stamina on the new weapon vs 30 + 6 socket bonus on the old one
	want := stats.Stats{stats.Stamina: 4, stats.Strength: -20, stats.MeleeCrit: -30, stats.SpellCrit: -30, stats.MeleeHaste: -20, stats.SpellHaste: -20}
	if got != want {
		t.Errorf("stat changes = %v, want %v", got, want)
	}
}

const (
	itemTestMetaHead = 9100024
	gemTestBlue      = 9100025
	gemTestChaotic   = 41285
)

func init() {
	AddToDatabase(&proto.SimDatabase{
		Items: []*proto.SimItem{{
			Id:         itemTestMetaHead,
			Type:       proto.ItemType_ItemTypeHead,
			GemSockets: []proto.GemColor{proto.GemColor_GemColorMeta, proto.GemColor_GemColorBlue},
		}},
		Gems: []*proto.SimGem{
			{Id: gemTestBlue, Color: proto.GemColor_GemColorBlue, Stats: stats.Stats{stats.Stamina: 10}.ToFloatArray()},
			{Id: gemTestChaotic, Color: proto.GemColor_GemColorMeta, Stats: stats.Stats{stats.MeleeCrit: 21}.ToFloatArray()},
		},
	})
}

// metaSwapTestSim is a player with Chaotic Skyflare and a blue gem in the head, whose second blue gem
// sits in one of its two main hand weapons: the worn one, or the one it swaps to.
func metaSwapTestSim(t *testing.T, startsOn bool, setup func(*timingTestAgent)) (*Simulation, *timingTestAgent) {
	t.Helper()
	timingTestSetup = setup
	defer func() { timingTestSetup = nil }()

	withGem := &proto.ItemSpec{Id: itemTestSwapSocketed, Gems: []int32{gemTestBlue}}
	without := &proto.ItemSpec{Id: itemTestSwapPlain}
	worn, other := withGem, without
	if !startsOn {
		worn, other = without, withGem
	}
	items := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotHead] = &proto.ItemSpec{Id: itemTestMetaHead, Gems: []int32{gemTestChaotic, gemTestBlue}}
	items[proto.ItemSlot_ItemSlotMainHand] = worn
	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true},
		Raid: &proto.Raid{
			Parties: []*proto.Party{{
				Players: []*proto.Player{{
					Name:           "Meta",
					Class:          proto.Class_ClassShaman,
					Consumes:       &proto.Consumes{},
					Buffs:          &proto.IndividualBuffs{},
					Spec:           &proto.Player_EnhancementShaman{},
					Equipment:      &proto.EquipmentSpec{Items: items},
					EnableItemSwap: true,
					ItemSwap:       &proto.ItemSwap{MhItem: other},
				}},
				Buffs: &proto.PartyBuffs{},
			}},
		},
		Encounter: &proto.Encounter{
			Targets:  []*proto.Target{{Name: "target", Level: 83, MobType: proto.MobType_MobTypeDemon}},
			Duration: 60,
		},
	})
	sim.reset()
	return sim, sim.Raid.Parties[0].Players[0].(*timingTestAgent)
}

// Player::CorrectMetaGemEnchants takes the whole enchant off as a swap breaks the gem's condition and puts
// it back as one meets it: the 3% crit damage and the gem's effect, not only its stats.
func TestItemSwapMovesTheMetaGemsCritDamageAndEffect(t *testing.T) {
	const with, without = 1 + (1.5*1.03 - 1), 1.5
	for _, startsOn := range []bool{true, false} {
		var spell, heal *Spell
		effectOn := false
		toggles := 0
		sim, a := metaSwapTestSim(t, startsOn, func(a *timingTestAgent) {
			spell = a.RegisterSpell(SpellConfig{
				ActionID:       ActionID{SpellID: testSpellNoServerData},
				CritMultiplier: a.DefaultSpellCritMultiplier(),
			})
			heal = a.RegisterSpell(SpellConfig{
				ActionID:       ActionID{SpellID: testSpellNoServerData, Tag: 1},
				Flags:          SpellFlagHelpful,
				CritMultiplier: a.DefaultHealingCritMultiplier(),
			})
			if !a.MetaGemSwaps() {
				t.Fatal("the swap changes the blue gems from two to one, which Chaotic Skyflare needs")
			}
			a.OnMetaGemSwap(func(_ *Simulation, on bool) {
				effectOn = on
				toggles++
			})
		})
		check := func(when string, on bool) {
			t.Helper()
			want := without
			if on {
				want = with
			}
			if spell.CritMultiplier != want || effectOn != on || a.HasMetaGemEquipped(gemTestChaotic) != on {
				t.Errorf("starts on %v, %s: crit multiplier %v, effect on %v, gem met %v; want %v and %v",
					startsOn, when, spell.CritMultiplier, effectOn, a.HasMetaGemEquipped(gemTestChaotic), want, on)
			}
			if got, want := a.ItemSwap.mhCritMultiplier, 2*TernaryFloat64(on, 1.03, 1); got != want {
				t.Errorf("starts on %v, %s: the swapped weapon's crit multiplier %v, want %v", startsOn, when, got, want)
			}
			// the gem's 3% is on damage, though a heal's multiplier is a damage spell's without it
			if heal.CritMultiplier != without {
				t.Errorf("starts on %v, %s: a heal's crit multiplier %v, want %v", startsOn, when, heal.CritMultiplier, without)
			}
		}

		check("at the pull", startsOn)
		crit := a.GetStat(stats.MeleeCrit)
		a.ItemSwap.SwapItems(sim, a.ItemSwap.slots)
		check("swapped", !startsOn)
		// the weapons' own 50 crit aside, the gem's 21 goes with it
		if got, want := a.GetStat(stats.MeleeCrit)-crit, TernaryFloat64(startsOn, -50-21, 50+21); got != want {
			t.Errorf("starts on %v: the swap moves melee crit by %v, want %v", startsOn, got, want)
		}
		a.ItemSwap.SwapItems(sim, a.ItemSwap.slots)
		check("swapped back", startsOn)

		// a fight that ends swapped starts the next one as built
		a.ItemSwap.SwapItems(sim, a.ItemSwap.slots)
		sim.Cleanup()
		sim.reset()
		check("at the next pull", startsOn)
		if effectOn != startsOn || toggles == 0 {
			t.Errorf("starts on %v: effect on %v after %d toggles", startsOn, effectOn, toggles)
		}
	}
}

// A swap that leaves the gem's condition alone leaves its effect to apply for good, as before.
func TestItemSwapThatKeepsTheMetaGemSetsNothingUp(t *testing.T) {
	character := &Character{}
	character.Equipment = metaTestEquipment(gemTestChaotic, proto.GemColor_GemColorBlue, proto.GemColor_GemColorBlue)
	character.Equipment[proto.ItemSlot_ItemSlotMainHand] = NewItem(ItemSpec{ID: itemTestSwapSocketed}, nil)
	character.enableItemSwap(&proto.ItemSwap{MhItem: &proto.ItemSpec{Id: itemTestSwapPlain}}, 0, 0, 0)
	character.initMetaGemSwap()

	if character.MetaGemSwaps() {
		t.Error("both weapons leave two blue gems on, so the gem never changes")
	}
}

func TestItemSwapSkipsSameItemWithDifferentReforge(t *testing.T) {
	character := &Character{}
	character.Equipment[proto.ItemSlot_ItemSlotMainHand] = NewItem(ItemSpec{
		ID:      itemTestSwapSocketed,
		Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36},
	}, nil)

	character.enableItemSwap(&proto.ItemSwap{MhItem: &proto.ItemSpec{Id: itemTestSwapSocketed}}, 1, 1, 1)

	if len(character.ItemSwap.slots) != 0 {
		t.Errorf("swap slots = %v, want none", character.ItemSwap.slots)
	}
}
