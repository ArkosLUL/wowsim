package optimizer

import (
	"errors"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func rtGems(ids ...int32) [MaxGems]int32 {
	var out [MaxGems]int32
	copy(out[:], ids)
	return out
}

// rtLoadout passes every rule of rtOptimizeRequest's pool. Its meta needs a red, a yellow and a
// blue gem; the buckle's prismatic gem counts as all three.
func rtLoadout() Loadout {
	l := Loadout{RacialTraits: proto.Race_RaceOrc}
	l.Items[proto.ItemSlot_ItemSlotHead] = ItemChoice{ItemID: rtHead, Enchant: rtEnchHead, Gems: rtGems(rtMetaRYB, rtRed, rtBlue)}
	l.Items[proto.ItemSlot_ItemSlotNeck] = ItemChoice{ItemID: rtReforge, ReforgeFrom: 32, ReforgeTo: 36}
	l.Items[proto.ItemSlot_ItemSlotWrist] = ItemChoice{ItemID: rtWrist, Gems: rtGems(rtYellow)}
	l.Items[proto.ItemSlot_ItemSlotWaist] = ItemChoice{ItemID: rtWaist, Gems: rtGems(rtRed, rtPrismatic)}
	l.Items[proto.ItemSlot_ItemSlotFinger1] = ItemChoice{ItemID: rtRing, Enchant: rtEnchRing}
	l.Items[proto.ItemSlot_ItemSlotFinger2] = ItemChoice{ItemID: rtRingMax1}
	l.Items[proto.ItemSlot_ItemSlotTrinket1] = ItemChoice{ItemID: rtTrinket}
	l.Items[proto.ItemSlot_ItemSlotTrinket2] = ItemChoice{ItemID: rtTrinket2}
	l.Items[proto.ItemSlot_ItemSlotMainHand] = ItemChoice{ItemID: rtSword, Enchant: rtEnchWeapon}
	l.Items[proto.ItemSlot_ItemSlotOffHand] = ItemChoice{ItemID: rtAxeUnique}
	return l
}

func withBlacksmithing(req *proto.OptimizeGearRequest) {
	rtTarget(req).Professions = append(rtTarget(req).Professions, proto.Profession_Blacksmithing)
}

func TestCheck(t *testing.T) {
	const (
		head, neck, back, wrist, hands, waist = proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotNeck, proto.ItemSlot_ItemSlotBack,
			proto.ItemSlot_ItemSlotWrist, proto.ItemSlot_ItemSlotHands, proto.ItemSlot_ItemSlotWaist
		legs, finger1, finger2, trinket1, trinket2 = proto.ItemSlot_ItemSlotLegs, proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2,
			proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2
		mainHand, offHand = proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	)
	weapons := func(mh, oh int32) func(*Loadout) {
		return func(l *Loadout) {
			l.Items[mainHand] = ItemChoice{ItemID: mh}
			l.Items[offHand] = ItemChoice{ItemID: oh}
		}
	}
	gemsIn := func(slot proto.ItemSlot, ids ...int32) func(*Loadout) {
		return func(l *Loadout) { l.Items[slot].Gems = rtGems(ids...) }
	}
	both := func(changes ...func(*Loadout)) func(*Loadout) {
		return func(l *Loadout) {
			for _, change := range changes {
				change(l)
			}
		}
	}
	talents := func(s string) func(*proto.OptimizeGearRequest) {
		return func(req *proto.OptimizeGearRequest) { rtTarget(req).TalentsString = s }
	}
	paladin := func(req *proto.OptimizeGearRequest) {
		rtTarget(req).Class, rtTarget(req).TalentsString = proto.Class_ClassPaladin, ""
	}

	tests := []struct {
		name   string
		pool   func(*proto.OptimizeGearRequest)
		change func(*Loadout)
		want   Rule
		slot   proto.ItemSlot
	}{
		{name: "valid"},

		// weapon combos
		{name: "two-hander and one-hander without Titan's Grip", pool: talents(rtFuryNoTG), change: weapons(rtGreatswd, rtSword), want: RuleWeapons, slot: offHand},
		{name: "two-hander alone without Titan's Grip", pool: talents(rtFuryNoTG), change: weapons(rtGreatswd, 0)},
		{name: "two two-handers with Titan's Grip", change: weapons(rtGreatswd, rtGreatswd)},
		{name: "two-hander and shield with Titan's Grip", change: weapons(rtGreatswd, rtShield)},
		{name: "two-hander in the off hand without Titan's Grip", pool: talents(rtFuryNoTG), change: weapons(rtSword, rtGreatswd), want: RuleWeapons, slot: offHand},
		{name: "polearm with an off hand", change: weapons(rtPolearm, rtSword), want: RuleWeapons, slot: offHand},
		{name: "staff with a shield", change: weapons(rtStaff, rtShield), want: RuleWeapons, slot: offHand},
		{name: "polearm in the off hand", change: weapons(rtGreatswd, rtPolearm), want: RuleWeapons, slot: offHand},
		{name: "one-hander in the off hand, main hand empty", change: weapons(0, rtSword), want: RuleSlot, slot: offHand},
		{name: "shield with the main hand empty", change: weapons(0, rtShield)},
		{name: "off-hand dagger with the main hand empty", change: weapons(0, rtDagger)},
		{name: "unique one-hander in both hands", change: weapons(rtAxeUnique, rtAxeUnique), want: RuleUnique, slot: offHand},
		{name: "same one-hander in both hands", change: weapons(rtSword, rtSword)},
		{name: "paladin dual wielding", pool: paladin, change: weapons(rtSword, rtAxeUnique), want: RuleWeapons, slot: offHand},
		{name: "paladin with a shield", pool: paladin, change: weapons(rtSword, rtShield)},
		{name: "main-hand weapon in the off hand", change: weapons(rtSword, rtMace), want: RuleSlot, slot: offHand},
		{name: "fishing pole alone", change: weapons(rtPole, 0)},
		{name: "fishing pole with a shield", change: weapons(rtPole, rtShield), want: RuleWeapons, slot: offHand},
		{name: "fishing pole in the off hand", change: weapons(rtGreatswd, rtPole), want: RuleWeapons, slot: offHand},
		{name: "shield without a hand type", change: weapons(rtGreatswd, rtOddShield), want: RuleSlot, slot: offHand},
		{name: "shield in the main hand", change: weapons(rtShield, 0), want: RuleSlot, slot: mainHand},
		{name: "ring in the second slot only", change: func(l *Loadout) { l.Items[finger1] = ItemChoice{} }, want: RuleSlot, slot: finger2},
		{name: "trinket in the second slot only", change: func(l *Loadout) { l.Items[trinket1] = ItemChoice{} }, want: RuleSlot, slot: trinket2},
		{name: "ring in a trinket slot", change: func(l *Loadout) { l.Items[trinket1] = ItemChoice{ItemID: rtRing} }, want: RuleSlot, slot: trinket1},

		// catalog limits
		{name: "maxcount 1 ring twice", change: func(l *Loadout) { l.Items[finger1] = ItemChoice{ItemID: rtRingMax1} }, want: RuleUnique, slot: finger2},
		{name: "two rings of a limit category", change: func(l *Loadout) {
			l.Items[finger1] = ItemChoice{ItemID: rtRingCatA}
			l.Items[finger2] = ItemChoice{ItemID: rtRingCatB}
		}, want: RuleLimitGroup, slot: finger2},
		{name: "item needing a profession", change: func(l *Loadout) { l.Items[head].ItemID = rtEngiHelm }, want: RuleProfession, slot: head},
		{name: "excluded item", pool: func(req *proto.OptimizeGearRequest) {
			req.Settings.ExcludedItemIds = append(req.Settings.ExcludedItemIds, rtTrinket2)
		},
			want: RuleExcluded, slot: trinket2},
		{name: "item the slot's pool doesn't list", pool: func(req *proto.OptimizeGearRequest) {
			req.Pool.Slots[10].ItemIds = []int32{rtTrinket}
		}, want: RulePool, slot: trinket2},
		{name: "empty slot with a gem", change: gemsIn(legs, rtRed), want: RuleSlot, slot: legs},

		// jewelcrafter gems and unique gems
		{name: "3 jewelcrafter gems", change: both(gemsIn(head, rtMetaRYB, rtJCRed, rtBlue), gemsIn(wrist, rtJCYellow), gemsIn(waist, rtJCRed, rtPrismatic))},
		{name: "4 jewelcrafter gems", change: both(gemsIn(head, rtMetaRYB, rtJCRed, rtBlue), gemsIn(wrist, rtJCYellow), gemsIn(waist, rtJCRed, rtJCRed)),
			want: RuleLimitGroup, slot: waist},
		{name: "jewelcrafter gem without jewelcrafting", pool: func(req *proto.OptimizeGearRequest) {
			rtTarget(req).Professions = []proto.Profession{proto.Profession_Mining}
		}, change: gemsIn(wrist, rtJCYellow), want: RuleProfession, slot: wrist},
		{name: "unique gem twice", change: both(gemsIn(wrist, rtUniqueGem), gemsIn(waist, rtRed, rtUniqueGem)), want: RuleUnique, slot: waist},
		{name: "excluded gem", change: gemsIn(wrist, rtBadGem), want: RuleExcluded, slot: wrist},

		// metas
		{name: "2-blue meta on two prismatic gems", change: both(gemsIn(head, rtMeta2Blue, rtRed, rtRed), gemsIn(wrist, rtPrismatic))},
		{name: "2-blue meta on one prismatic gem", change: both(gemsIn(head, rtMeta2Blue, rtRed, rtRed), gemsIn(wrist, rtYellow)), want: RuleMeta, slot: head},
		{name: "red, yellow and blue meta on orange and blue", change: both(gemsIn(head, rtMetaRYB, rtOrange, rtBlue), gemsIn(wrist, rtOrange), gemsIn(waist, rtOrange))},
		{name: "red, yellow and blue meta on orange only", change: both(gemsIn(head, rtMetaRYB, rtOrange, rtOrange), gemsIn(wrist, rtOrange), gemsIn(waist, rtRed)),
			want: RuleMeta, slot: head},
		{name: "3-red meta on purple, orange and red", change: both(gemsIn(head, rtMeta3Red, rtPurple, rtOrange), gemsIn(waist, rtRed))},
		{name: "3-red meta on two oranges", change: both(gemsIn(head, rtMeta3Red, rtOrange, rtBlue), gemsIn(wrist, rtOrange), gemsIn(waist, rtBlue)),
			want: RuleMeta, slot: head},
		{name: "more red than blue, tied", change: both(gemsIn(head, rtMetaRedGtB, rtRed, rtBlue), gemsIn(wrist, rtGreen), gemsIn(waist, rtRed)),
			want: RuleMeta, slot: head},
		{name: "more red than blue", change: both(gemsIn(head, rtMetaRedGtB, rtRed, rtBlue), gemsIn(wrist, rtGreen), gemsIn(waist, rtRed, rtRed))},
		{name: "empty meta socket", change: gemsIn(head, 0, rtRed, rtBlue)},

		// sockets
		{name: "bracer socket without blacksmithing", change: gemsIn(wrist, rtYellow, rtRed), want: RuleSocket, slot: wrist},
		{name: "bracer socket counts for the meta", pool: withBlacksmithing,
			change: both(gemsIn(head, rtMeta2Blue, rtRed, rtBlue), gemsIn(wrist, rtYellow, rtBlue), gemsIn(waist, rtRed))},
		{name: "glove socket with blacksmithing", pool: withBlacksmithing, change: func(l *Loadout) { l.Items[hands] = ItemChoice{ItemID: rtHands, Gems: rtGems(rtRed)} }},
		{name: "glove socket without blacksmithing", change: func(l *Loadout) { l.Items[hands] = ItemChoice{ItemID: rtHands, Gems: rtGems(rtRed)} },
			want: RuleSocket, slot: hands},
		{name: "gem past the buckle", change: gemsIn(waist, rtRed, rtRed, rtRed), want: RuleSocket, slot: waist},
		{name: "belt with 3 sockets", change: func(l *Loadout) { l.Items[waist] = ItemChoice{ItemID: rtWaist3, Gems: rtGems(rtRed, rtYellow, rtBlue)} }},
		{name: "no buckle socket past 3", change: func(l *Loadout) {
			l.Items[waist] = ItemChoice{ItemID: rtWaist3, Gems: rtGems(rtRed, rtYellow, rtBlue, rtRed)}
		}, want: RuleSocket, slot: waist},
		{name: "meta gem in a red socket", change: gemsIn(head, rtMetaRYB, rtMeta2Blue, rtBlue), want: RuleSocket, slot: head},
		{name: "red gem in the meta socket", change: gemsIn(head, rtRed, rtRed, rtBlue), want: RuleSocket, slot: head},

		// enchants and reforges
		{name: "ring enchant in the other ring slot", change: func(l *Loadout) {
			l.Items[finger1] = ItemChoice{ItemID: rtRingMax1}
			l.Items[finger2] = ItemChoice{ItemID: rtRing, Enchant: rtEnchRing}
		}, want: RuleEnchant, slot: finger2},
		{name: "head enchant on a weapon", change: func(l *Loadout) { l.Items[mainHand].Enchant = rtEnchHead }, want: RuleEnchant, slot: mainHand},
		{name: "reforge into a stat the item has", change: func(l *Loadout) { l.Items[neck].ReforgeTo = 31 }, want: RuleReforge, slot: neck},
		{name: "reforge on StatsCount 10", change: func(l *Loadout) { l.Items[back] = ItemChoice{ItemID: rtNoReforge, ReforgeFrom: 32, ReforgeTo: 36} },
			want: RuleReforge, slot: back},

		// locks and floors
		{name: "locked slot kept", pool: func(req *proto.OptimizeGearRequest) {
			rtTarget(req).Equipment.Items[neck] = &proto.ItemSpec{Id: rtReforge, Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36}}
			req.Settings.LockedSlots = []proto.ItemSlot{neck}
		}},
		{name: "locked slot changed", pool: func(req *proto.OptimizeGearRequest) {
			rtTarget(req).Equipment.Items[neck] = &proto.ItemSpec{Id: rtReforge}
			req.Settings.LockedSlots = []proto.ItemSlot{neck}
		}, want: RuleLocked, slot: neck},
		{name: "floor met", pool: func(req *proto.OptimizeGearRequest) {
			req.Settings.StatMinimums = []*proto.StatMinimum{{Stat: proto.Stat_StatStrength, MinValue: 100}}
		}},
		{name: "floor missed", pool: func(req *proto.OptimizeGearRequest) {
			req.Settings.StatMinimums = []*proto.StatMinimum{{Stat: proto.Stat_StatStrength, MinValue: 100000}}
		}, want: RuleFloor, slot: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := rtPool(t, tt.pool)
			l := rtLoadout()
			if tt.change != nil {
				tt.change(&l)
			}
			err := p.Check(l)
			if tt.want == 0 {
				if err != nil {
					t.Errorf("Check = %v, want nil", err)
				}
				return
			}
			var ruleErr *RuleError
			if !errors.As(err, &ruleErr) || ruleErr.Rule != tt.want || ruleErr.Slot != tt.slot {
				t.Errorf("Check = %v, want %s in %s", err, tt.want, tt.slot)
			}
		})
	}
}

// A floor reads the character sheet: buffs and the target's own stats count, not just gear.
func TestCheckFloorsUseTheSheet(t *testing.T) {
	p := rtPool(t, nil)
	l := rtLoadout()
	sheet, err := p.finalStats(l)
	if err != nil {
		t.Fatal(err)
	}
	gear := gearStats(l)
	if sheet[stats.Strength] <= gear[stats.Strength] {
		t.Errorf("sheet strength %.0f isn't above the gear's %.0f", sheet[stats.Strength], gear[stats.Strength])
	}

	var empty Loadout
	empty.RacialTraits = proto.Race_RaceOrc
	bare, err := p.finalStats(empty)
	if err != nil {
		t.Fatal(err)
	}
	p.floors = []*proto.StatMinimum{{Stat: proto.Stat_StatStrength, MinValue: (bare[stats.Strength] + sheet[stats.Strength]) / 2}}
	if err := p.Check(l); err != nil {
		t.Errorf("geared loadout: %v", err)
	}
	if err := p.Check(empty); err == nil {
		t.Error("an empty loadout met a floor between it and the geared one")
	}
}

// Whatever passes CheckGear, core equips exactly as the loadout says: NewEquipmentSet never moves
// a ring, trinket or weapon to another slot.
func TestCheckedLoadoutsStayPut(t *testing.T) {
	weapons := []int32{0, rtSword, rtAxeUnique, rtGreatswd, rtPolearm, rtStaff, rtShield, rtHeld, rtMace, rtDagger, rtPole, rtOddShield}
	pairs := [][2]int32{{0, 0}, {rtRing, 0}, {0, rtRing}, {rtRing, rtRingMax1}, {rtRingMax1, rtRing}}
	trinkets := [][2]int32{{0, 0}, {rtTrinket, 0}, {0, rtTrinket}, {rtTrinket, rtTrinket2}}
	players := map[string]func(*proto.OptimizeGearRequest){
		"Titan's Grip":    nil,
		"no Titan's Grip": func(req *proto.OptimizeGearRequest) { rtTarget(req).TalentsString = rtFuryNoTG },
		"paladin": func(req *proto.OptimizeGearRequest) {
			rtTarget(req).Class, rtTarget(req).TalentsString = proto.Class_ClassPaladin, ""
		},
	}
	for name, change := range players {
		p := rtPool(t, change)
		passed := 0
		for _, mh := range weapons {
			for _, oh := range weapons {
				for _, rings := range pairs {
					for _, trinket := range trinkets {
						var l Loadout
						l.Items[proto.ItemSlot_ItemSlotMainHand].ItemID = mh
						l.Items[proto.ItemSlot_ItemSlotOffHand].ItemID = oh
						l.Items[proto.ItemSlot_ItemSlotFinger1].ItemID, l.Items[proto.ItemSlot_ItemSlotFinger2].ItemID = rings[0], rings[1]
						l.Items[proto.ItemSlot_ItemSlotTrinket1].ItemID, l.Items[proto.ItemSlot_ItemSlotTrinket2].ItemID = trinket[0], trinket[1]
						if p.CheckGear(l) != nil {
							continue
						}
						passed++
						equipment := core.ProtoToEquipment(l.Equipment(), nil)
						for slot, c := range l.Items {
							if got := equipment[slot].ID; got != c.ItemID {
								t.Errorf("%s: %s holds %d, want %d (loadout %v)", name, proto.ItemSlot(slot), got, c.ItemID, l.Items)
							}
						}
					}
				}
			}
		}
		if passed == 0 {
			t.Errorf("%s: no loadout passed", name)
		}
	}
}
