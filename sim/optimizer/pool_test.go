package optimizer

import (
	"slices"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Made-up items, gems and enchants for the pool, rules and gem tests; ids stay clear of the with_db
// data and of the other tests' ids, since AddToDatabase never replaces an entry.
const (
	rtHead      = 9600001 // meta, red and blue sockets
	rtWrist     = 9600003 // yellow socket
	rtHands     = 9600004 // no sockets
	rtWaist     = 9600005 // red socket
	rtLegs      = 9600006 // red, yellow, blue
	rtRingMax1  = 9600010 // maxcount 1
	rtRing      = 9600011
	rtRingCatA  = 9600012 // limit category 60, one equipped
	rtRingCatB  = 9600013
	rtTrinket   = 9600020
	rtTrinket2  = 9600021
	rtSword     = 9600030 // one-hander
	rtAxeUnique = 9600031 // unique-equipped one-hander
	rtGreatswd  = 9600032 // two-hander
	rtPolearm   = 9600033
	rtStaff     = 9600034
	rtShield    = 9600035
	rtHeld      = 9600036 // held in the off hand
	rtMace      = 9600037 // main hand only
	rtDagger    = 9600038 // off hand only
	rtEngiHelm  = 9600039 // needs engineering
	rtRanged    = 9600040
	rtReforge   = 9600041 // crit and hit, StatsCount 2
	rtNoReforge = 9600042 // crit, StatsCount 10
	rtPole      = 9600043 // fishing pole: two-handed, no weapon type
	rtOddShield = 9600044 // shield without a hand type
	rtWaist3    = 9600045 // three sockets of its own

	rtMeta2Blue  = 9600100
	rtMetaRYB    = 9600101
	rtMeta3Red   = 9600102
	rtMetaRedGtB = 9600103 // more red than blue
	rtRed        = 9600110
	rtYellow     = 9600111
	rtBlue       = 9600112
	rtOrange     = 9600113
	rtPrismatic  = 9600114
	rtPurple     = 9600115
	rtGreen      = 9600116
	rtJCRed      = 9600120 // jewelcrafter only, category 2
	rtJCYellow   = 9600121
	rtUniqueGem  = 9600130 // prismatic, unique-equipped
	rtBadGem     = 9600131 // excluded by the settings

	rtEnchHead   = 9600200
	rtEnchRing   = 9600201 // offered on rtRing in Finger1 only
	rtEnchWeapon = 9600202
)

var (
	rtFuryTG   = "-000000000000000000000000001"    // Titan's Grip
	rtFuryNoTG = "-00000000000000000000000000"     // no Titan's Grip
	rtEnhDW    = "-00000000000000000001"           // shaman Dual Wield
	rtArms     = "3022032123335100202012013031251" // arms only
)

func rtItem(id int32, t proto.ItemType, s stats.Stats, sockets ...proto.GemColor) *proto.SimItem {
	return &proto.SimItem{Id: id, Type: t, Stats: s.ToFloatArray(), GemSockets: sockets, SocketBonus: stats.Stats{stats.Stamina: 6}.ToFloatArray()}
}

func rtWeapon(id int32, hand proto.HandType, wt proto.WeaponType) *proto.SimItem {
	return &proto.SimItem{Id: id, Type: proto.ItemType_ItemTypeWeapon, HandType: hand, WeaponType: wt, WeaponDamageMin: 100, WeaponDamageMax: 200,
		WeaponSpeed: 2.6, Stats: stats.Stats{stats.Strength: 20}.ToFloatArray()}
}

func rtGem(id int32, color proto.GemColor, s stats.Stats) *proto.SimGem {
	return &proto.SimGem{Id: id, Color: color, Stats: s.ToFloatArray()}
}

var rtDatabase = &proto.SimDatabase{
	Items: []*proto.SimItem{
		rtItem(rtHead, proto.ItemType_ItemTypeHead, stats.Stats{stats.Strength: 50},
			proto.GemColor_GemColorMeta, proto.GemColor_GemColorRed, proto.GemColor_GemColorBlue),
		rtItem(rtWrist, proto.ItemType_ItemTypeWrist, stats.Stats{stats.Strength: 30}, proto.GemColor_GemColorYellow),
		rtItem(rtHands, proto.ItemType_ItemTypeHands, stats.Stats{stats.Strength: 40}),
		rtItem(rtWaist, proto.ItemType_ItemTypeWaist, stats.Stats{stats.Strength: 40}, proto.GemColor_GemColorRed),
		rtItem(rtLegs, proto.ItemType_ItemTypeLegs, stats.Stats{stats.Strength: 60},
			proto.GemColor_GemColorRed, proto.GemColor_GemColorYellow, proto.GemColor_GemColorBlue),
		rtItem(rtRingMax1, proto.ItemType_ItemTypeFinger, stats.Stats{stats.Strength: 20}),
		rtItem(rtRing, proto.ItemType_ItemTypeFinger, stats.Stats{stats.Strength: 21}),
		rtItem(rtRingCatA, proto.ItemType_ItemTypeFinger, stats.Stats{stats.Strength: 22}),
		rtItem(rtRingCatB, proto.ItemType_ItemTypeFinger, stats.Stats{stats.Strength: 23}),
		rtItem(rtTrinket, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 60}),
		rtItem(rtTrinket2, proto.ItemType_ItemTypeTrinket, stats.Stats{stats.AttackPower: 70}),
		rtWeapon(rtSword, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword),
		rtWeapon(rtAxeUnique, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeAxe),
		rtWeapon(rtGreatswd, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeSword),
		rtWeapon(rtPolearm, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypePolearm),
		rtWeapon(rtStaff, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeStaff),
		{Id: rtShield, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeOffHand, WeaponType: proto.WeaponType_WeaponTypeShield},
		{Id: rtHeld, Type: proto.ItemType_ItemTypeWeapon, HandType: proto.HandType_HandTypeOffHand, WeaponType: proto.WeaponType_WeaponTypeOffHand},
		rtWeapon(rtMace, proto.HandType_HandTypeMainHand, proto.WeaponType_WeaponTypeMace),
		rtWeapon(rtDagger, proto.HandType_HandTypeOffHand, proto.WeaponType_WeaponTypeDagger),
		rtItem(rtEngiHelm, proto.ItemType_ItemTypeHead, stats.Stats{stats.Strength: 70}, proto.GemColor_GemColorMeta),
		{Id: rtRanged, Type: proto.ItemType_ItemTypeRanged, RangedWeaponType: proto.RangedWeaponType_RangedWeaponTypeThrown},
		rtItem(rtReforge, proto.ItemType_ItemTypeNeck, stats.Stats{stats.MeleeCrit: 40, stats.SpellCrit: 40, stats.MeleeHit: 30, stats.SpellHit: 30}),
		rtItem(rtNoReforge, proto.ItemType_ItemTypeBack, stats.Stats{stats.MeleeCrit: 40, stats.SpellCrit: 40}),
		rtWeapon(rtPole, proto.HandType_HandTypeTwoHand, proto.WeaponType_WeaponTypeUnknown),
		{Id: rtOddShield, Type: proto.ItemType_ItemTypeWeapon, WeaponType: proto.WeaponType_WeaponTypeShield},
		rtItem(rtWaist3, proto.ItemType_ItemTypeWaist, stats.Stats{stats.Strength: 40},
			proto.GemColor_GemColorRed, proto.GemColor_GemColorYellow, proto.GemColor_GemColorBlue),
	},
	Gems: []*proto.SimGem{
		rtGem(rtMeta2Blue, proto.GemColor_GemColorMeta, stats.Stats{stats.Strength: 21}),
		rtGem(rtMetaRYB, proto.GemColor_GemColorMeta, stats.Stats{stats.Strength: 21}),
		rtGem(rtMeta3Red, proto.GemColor_GemColorMeta, stats.Stats{stats.Strength: 21}),
		rtGem(rtMetaRedGtB, proto.GemColor_GemColorMeta, stats.Stats{stats.Strength: 21}),
		rtGem(rtRed, proto.GemColor_GemColorRed, stats.Stats{stats.Strength: 20}),
		rtGem(rtYellow, proto.GemColor_GemColorYellow, stats.Stats{stats.MeleeCrit: 20}),
		rtGem(rtBlue, proto.GemColor_GemColorBlue, stats.Stats{stats.Stamina: 30}),
		rtGem(rtOrange, proto.GemColor_GemColorOrange, stats.Stats{stats.Strength: 10, stats.MeleeCrit: 10}),
		rtGem(rtPrismatic, proto.GemColor_GemColorPrismatic, stats.Stats{stats.Strength: 5, stats.Stamina: 5}),
		rtGem(rtPurple, proto.GemColor_GemColorPurple, stats.Stats{stats.Strength: 10, stats.Stamina: 15}),
		rtGem(rtGreen, proto.GemColor_GemColorGreen, stats.Stats{stats.MeleeCrit: 10, stats.Stamina: 15}),
		rtGem(rtJCRed, proto.GemColor_GemColorRed, stats.Stats{stats.Strength: 34}),
		rtGem(rtJCYellow, proto.GemColor_GemColorYellow, stats.Stats{stats.MeleeCrit: 34}),
		rtGem(rtUniqueGem, proto.GemColor_GemColorPrismatic, stats.Stats{stats.Strength: 10, stats.Agility: 10, stats.Stamina: 10}),
		rtGem(rtBadGem, proto.GemColor_GemColorRed, stats.Stats{stats.Strength: 100}),
	},
	Enchants: []*proto.SimEnchant{
		{EffectId: rtEnchHead, Stats: stats.Stats{stats.AttackPower: 50}.ToFloatArray()},
		{EffectId: rtEnchRing, Stats: stats.Stats{stats.AttackPower: 40}.ToFloatArray()},
		{EffectId: rtEnchWeapon, Stats: stats.Stats{stats.AttackPower: 30}.ToFloatArray()},
	},
}

func rtSlotPool(slot proto.ItemSlot, ids ...int32) *proto.SlotPool {
	return &proto.SlotPool{Slot: slot, ItemIds: ids}
}

// rtOptimizeRequest is a warrior with Titan's Grip, jewelcrafting and mining, and a pool over the
// made-up items.
func rtOptimizeRequest() *proto.OptimizeGearRequest {
	weapons := []int32{rtSword, rtAxeUnique, rtGreatswd, rtPolearm, rtStaff, rtShield, rtHeld, rtMace, rtDagger, rtPole, rtOddShield}
	head := rtSlotPool(proto.ItemSlot_ItemSlotHead, rtHead, rtEngiHelm)
	head.EnchantOptions = []*proto.ItemEnchantOptions{{ItemId: rtHead, EnchantIds: []int32{rtEnchHead}}}
	finger1 := rtSlotPool(proto.ItemSlot_ItemSlotFinger1, rtRingMax1, rtRing, rtRingCatA, rtRingCatB)
	finger1.EnchantOptions = []*proto.ItemEnchantOptions{{ItemId: rtRing, EnchantIds: []int32{rtEnchRing}}}
	mainHand := rtSlotPool(proto.ItemSlot_ItemSlotMainHand, weapons...)
	mainHand.EnchantOptions = []*proto.ItemEnchantOptions{{ItemId: rtSword, EnchantIds: []int32{rtEnchWeapon}}}
	offHand := rtSlotPool(proto.ItemSlot_ItemSlotOffHand, weapons...)
	offHand.EnchantOptions = []*proto.ItemEnchantOptions{{ItemId: rtSword, EnchantIds: []int32{rtEnchWeapon}}}

	target := &proto.Player{
		Name:          "Target",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		TalentsString: rtFuryTG,
		Professions:   []proto.Profession{proto.Profession_Jewelcrafting, proto.Profession_Mining},
		Equipment:     testEquipment(nil),
		Spec:          &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
		Rotation:      &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
		Database:      rtDatabase,
	}
	return &proto.OptimizeGearRequest{
		Base: &proto.RaidSimRequest{
			Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{target}}}},
			Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{{}}},
			SimOptions: &proto.SimOptions{Iterations: 10, RandomSeed: 1},
		},
		Settings: &proto.OptimizerSettings{ContentPhase: 3, ExcludedItemIds: []int32{rtBadGem}},
		Pool: &proto.CandidatePool{
			Slots: []*proto.SlotPool{
				head,
				rtSlotPool(proto.ItemSlot_ItemSlotNeck, rtReforge),
				rtSlotPool(proto.ItemSlot_ItemSlotBack, rtNoReforge),
				rtSlotPool(proto.ItemSlot_ItemSlotWrist, rtWrist),
				rtSlotPool(proto.ItemSlot_ItemSlotHands, rtHands),
				rtSlotPool(proto.ItemSlot_ItemSlotWaist, rtWaist, rtWaist3),
				rtSlotPool(proto.ItemSlot_ItemSlotLegs, rtLegs),
				finger1,
				rtSlotPool(proto.ItemSlot_ItemSlotFinger2, rtRingMax1, rtRing, rtRingCatA, rtRingCatB),
				rtSlotPool(proto.ItemSlot_ItemSlotTrinket1, rtTrinket, rtTrinket2),
				rtSlotPool(proto.ItemSlot_ItemSlotTrinket2, rtTrinket, rtTrinket2),
				mainHand,
				offHand,
				rtSlotPool(proto.ItemSlot_ItemSlotRanged, rtRanged),
			},
			GemIds: []int32{rtMeta2Blue, rtMetaRYB, rtMeta3Red, rtMetaRedGtB, rtRed, rtYellow, rtBlue, rtOrange, rtPrismatic, rtPurple,
				rtGreen, rtJCRed, rtJCYellow, rtUniqueGem, rtBadGem},
			MetaConditions: []*proto.MetaGemCondition{
				{GemId: rtMeta2Blue, Constraints: []*proto.MetaColorConstraint{{Blue: 1, MinTotal: 2}}},
				{GemId: rtMetaRYB, Constraints: []*proto.MetaColorConstraint{{Red: 1, MinTotal: 1}, {Yellow: 1, MinTotal: 1}, {Blue: 1, MinTotal: 1}}},
				{GemId: rtMeta3Red, Constraints: []*proto.MetaColorConstraint{{Red: 1, MinTotal: 3}}},
				{GemId: rtMetaRedGtB, Constraints: []*proto.MetaColorConstraint{{Red: 1, Blue: -1, MinTotal: 1}}},
			},
			LimitGroups: []*proto.LimitGroup{
				{Id: 2, Name: "Jeweler's Gems", MaxEquipped: 3},
				{Id: 60, Name: "Test Rings", MaxEquipped: 1},
			},
			CatalogItems: []*proto.CatalogItem{
				{Id: rtRingMax1, MaxCount: 1},
				{Id: rtRingCatA, LimitCategory: 60},
				{Id: rtRingCatB, LimitCategory: 60},
				{Id: rtAxeUnique, UniqueEquipped: true},
				{Id: rtEngiHelm, RequiredProfession: proto.Profession_Engineering},
				{Id: rtReforge, StatsCount: 2},
				{Id: rtNoReforge, StatsCount: 10},
				{Id: rtJCRed, IsGem: true, LimitCategory: 2, RequiredProfession: proto.Profession_Jewelcrafting},
				{Id: rtJCYellow, IsGem: true, LimitCategory: 2, RequiredProfession: proto.Profession_Jewelcrafting},
				{Id: rtUniqueGem, IsGem: true, UniqueEquipped: true},
			},
		},
	}
}

func rtPool(t *testing.T, change func(*proto.OptimizeGearRequest)) *Pool {
	t.Helper()
	req := rtOptimizeRequest()
	if change != nil {
		change(req)
	}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	p, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func rtTarget(req *proto.OptimizeGearRequest) *proto.Player {
	return req.Base.Raid.Parties[0].Players[0]
}

func candidateIDs(p *Pool, slot proto.ItemSlot) []int32 {
	var ids []int32
	for _, c := range p.Slots[slot] {
		ids = append(ids, c.Item.ID)
	}
	return ids
}

// Fishing poles and shields without a hand type never make the off hand's list.
func TestCompilePoolSlots(t *testing.T) {
	oneHanders := []int32{rtSword, rtAxeUnique}
	mainHand := []int32{rtSword, rtAxeUnique, rtGreatswd, rtPolearm, rtStaff, rtMace, rtPole}
	tests := []struct {
		name     string
		change   func(*proto.Player)
		mainHand []int32
		offHand  []int32
	}{
		{"Titan's Grip", nil,
			mainHand, append(slices.Clone(oneHanders), rtGreatswd, rtShield, rtHeld, rtDagger)},
		{"fury without Titan's Grip", func(p *proto.Player) { p.TalentsString = rtFuryNoTG },
			mainHand, append(slices.Clone(oneHanders), rtShield, rtHeld, rtDagger)},
		{"arms", func(p *proto.Player) { p.TalentsString = rtArms },
			mainHand, append(slices.Clone(oneHanders), rtShield, rtHeld, rtDagger)},
		{"paladin can't dual wield", func(p *proto.Player) {
			p.Class, p.TalentsString = proto.Class_ClassPaladin, ""
		}, mainHand, []int32{rtShield, rtHeld}},
		{"shaman without Dual Wield", func(p *proto.Player) {
			p.Class, p.TalentsString = proto.Class_ClassShaman, ""
		}, mainHand, []int32{rtShield, rtHeld}},
		{"shaman with Dual Wield", func(p *proto.Player) {
			p.Class, p.TalentsString = proto.Class_ClassShaman, rtEnhDW
		}, mainHand, append(slices.Clone(oneHanders), rtShield, rtHeld, rtDagger)},
		{"rogue", func(p *proto.Player) {
			p.Class, p.TalentsString = proto.Class_ClassRogue, ""
		}, mainHand, append(slices.Clone(oneHanders), rtShield, rtHeld, rtDagger)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := rtPool(t, func(req *proto.OptimizeGearRequest) {
				if tt.change != nil {
					tt.change(rtTarget(req))
				}
			})
			if got := candidateIDs(p, proto.ItemSlot_ItemSlotMainHand); !slices.Equal(got, tt.mainHand) {
				t.Errorf("main hand = %v, want %v", got, tt.mainHand)
			}
			if got := candidateIDs(p, proto.ItemSlot_ItemSlotOffHand); !slices.Equal(got, tt.offHand) {
				t.Errorf("off hand = %v, want %v", got, tt.offHand)
			}
		})
	}
}

func TestCompilePoolCandidates(t *testing.T) {
	p := rtPool(t, nil)

	if got := candidateIDs(p, proto.ItemSlot_ItemSlotHead); !slices.Equal(got, []int32{rtHead}) {
		t.Errorf("head = %v, want the engineering helm left out", got)
	}
	var gems []int32
	for _, g := range p.Gems {
		gems = append(gems, g.Gem.ID)
	}
	if slices.Contains(gems, rtBadGem) || !slices.Contains(gems, rtJCRed) || len(gems) != 14 {
		t.Errorf("gems = %v, want all but the excluded one", gems)
	}

	head := p.Slots[proto.ItemSlot_ItemSlotHead][0]
	if len(head.Enchants) != 1 || head.Enchants[0].ID != rtEnchHead || head.Enchants[0].Stats[stats.AttackPower] != 50 {
		t.Errorf("head enchants = %+v", head.Enchants)
	}
	if head.NeedsSim || !p.Slots[proto.ItemSlot_ItemSlotMainHand][0].NeedsSim {
		t.Error("weapons need sims for their damage, plain armor doesn't")
	}
	if len(p.candidate(proto.ItemSlot_ItemSlotFinger1, rtRing).Enchants) != 1 || len(p.candidate(proto.ItemSlot_ItemSlotFinger2, rtRing).Enchants) != 0 {
		t.Error("enchant options are per slot")
	}

	// jewelcrafting only: no blacksmith sockets
	sockets := func(p *Pool, slot proto.ItemSlot) []proto.GemColor { return p.Slots[slot][0].Sockets }
	if got := sockets(p, proto.ItemSlot_ItemSlotWaist); !slices.Equal(got, []proto.GemColor{proto.GemColor_GemColorRed, proto.GemColor_GemColorPrismatic}) {
		t.Errorf("belt sockets = %v, want its own plus the buckle", got)
	}
	if got := p.candidate(proto.ItemSlot_ItemSlotWaist, rtWaist3).Sockets; len(got) != 3 {
		t.Errorf("sockets of a belt with 3 = %v, want no buckle socket", got)
	}
	if got := sockets(p, proto.ItemSlot_ItemSlotWrist); len(got) != 1 {
		t.Errorf("bracer sockets without blacksmithing = %v", got)
	}
	bs := rtPool(t, func(req *proto.OptimizeGearRequest) {
		rtTarget(req).Professions = append(rtTarget(req).Professions, proto.Profession_Blacksmithing)
	})
	for _, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotWrist, proto.ItemSlot_ItemSlotHands} {
		if got := sockets(bs, slot); len(got) == 0 || got[len(got)-1] != proto.GemColor_GemColorPrismatic {
			t.Errorf("%s sockets with blacksmithing = %v, want a prismatic one last", slot, got)
		}
	}
}

func TestCompilePoolLockedSlots(t *testing.T) {
	p := rtPool(t, func(req *proto.OptimizeGearRequest) {
		rtTarget(req).Equipment.Items[proto.ItemSlot_ItemSlotNeck] = &proto.ItemSpec{Id: rtReforge, Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36}}
		req.Settings.LockedSlots = []proto.ItemSlot{proto.ItemSlot_ItemSlotNeck, proto.ItemSlot_ItemSlotBack}
	})
	neck := p.Slots[proto.ItemSlot_ItemSlotNeck]
	if !p.Locked[proto.ItemSlot_ItemSlotNeck] || len(neck) != 1 || len(neck[0].Reforges) != 1 || neck[0].Reforges[0].To != 36 {
		t.Errorf("locked neck = %+v", neck)
	}
	if len(p.Slots[proto.ItemSlot_ItemSlotBack]) != 0 {
		t.Error("a locked empty slot has candidates")
	}
}

// Reforges are exactly the pairs core.CanReforge allows, on items with StatsCount 1 to 9, except
// from a rating the item has for melee or spells only.
func TestReforgeOptions(t *testing.T) {
	const spirit, dodge, parry, hit, crit, haste, expertise = 6, 13, 14, 31, 32, 36, 37
	ratings := stats.Stats{stats.MeleeCrit: 40, stats.SpellCrit: 40, stats.MeleeHit: 30, stats.SpellHit: 30, stats.Strength: 50}
	spellHitOnly := stats.Stats{stats.MeleeCrit: 40, stats.SpellCrit: 40, stats.SpellHit: 30, stats.Strength: 50}
	meleeCritOnly := stats.Stats{stats.MeleeCrit: 40, stats.Spirit: 20}
	tests := []struct {
		name  string
		stats stats.Stats
		row   *proto.CatalogItem
		allow bool
		// pairs core.CanReforge allows that the server doesn't
		refused [][2]int32
		want    int
	}{
		// crit or hit into spirit, dodge, parry, haste or expertise
		{"no catalog row", ratings, nil, true, nil, 10},
		{"StatsCount 1", ratings, &proto.CatalogItem{StatsCount: 1}, true, nil, 10},
		{"StatsCount 9", ratings, &proto.CatalogItem{StatsCount: 9}, true, nil, 10},
		{"StatsCount 0", ratings, &proto.CatalogItem{}, false, nil, 0},
		{"StatsCount 10", ratings, &proto.CatalogItem{StatsCount: 10}, false, nil, 0},
		// only the crit moves
		{"spell hit only", spellHitOnly, &proto.CatalogItem{StatsCount: 3}, true,
			[][2]int32{{hit, spirit}, {hit, dodge}, {hit, parry}, {hit, haste}, {hit, expertise}}, 5},
		// only the spirit moves
		{"melee crit only", meleeCritOnly, &proto.CatalogItem{StatsCount: 2}, true,
			[][2]int32{{crit, dodge}, {crit, parry}, {crit, hit}, {crit, haste}, {crit, expertise}}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := core.Item{ID: 1, Stats: tt.stats}
			got := map[[2]int32]stats.Stats{}
			for _, o := range reforgeOptions(item, tt.row) {
				got[[2]int32{o.From, o.To}] = o.Stats
			}
			for _, from := range core.ReforgeableStatTypes {
				for _, to := range core.ReforgeableStatTypes {
					reforge := &proto.ItemReforge{FromStatType: from, ToStatType: to}
					allowed := tt.allow && core.CanReforge(item.Stats, reforge) && !slices.Contains(tt.refused, [2]int32{from, to})
					delta, listed := got[[2]int32{from, to}]
					if listed != allowed {
						t.Errorf("%d -> %d listed %v, want %v", from, to, listed, allowed)
					}
					if listed && delta != core.ReforgeStats(item.Stats, reforge) {
						t.Errorf("%d -> %d stats = %v", from, to, delta)
					}
				}
			}
			if len(got) != tt.want {
				t.Errorf("%d reforges listed, want %d", len(got), tt.want)
			}
		})
	}
}
