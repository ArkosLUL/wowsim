package optimizer

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	goproto "google.golang.org/protobuf/proto"
)

var update = flag.Bool("update", false, "rewrite "+searchTestdata)

// searchTestdata is the request for a CLI smoke run: the Fury P1 preset over its phase 1 pool, cut
// to item level 200 and up and without catalog sources so the file stays small. Rewrite it with
//
//	tools/acore/dock.sh exec go test --tags=with_db ./sim/optimizer -run TestSearchTestdata -update
const searchTestdata = "testdata/search/fury_p1.json"

// The CLI smoke request still loads, and its pool still compiles.
func TestSearchTestdata(t *testing.T) {
	if *update {
		req := presetOptimizeRequest(t, "fury_p1")
		player := req.Base.Raid.Parties[0].Players[0]
		player.Professions = []proto.Profession{proto.Profession_Jewelcrafting, proto.Profession_Engineering}
		req.Settings = &proto.OptimizerSettings{ContentPhase: 1, Effort: proto.OptimizerEffort_OptimizerEffortQuick,
			RacialMode: proto.OptimizerRacialMode_OptimizerRacialKeepCurrent}
		req.Pool = realisticPool(t, player, 1, func(item *proto.UIItem) bool { return item.Ilvl >= 200 })
		for i, row := range req.Pool.CatalogItems {
			// the search never reads sources, and they're most of the file
			row = goproto.Clone(row).(*proto.CatalogItem)
			row.Sources = nil
			req.Pool.CatalogItems[i] = row
		}
		data, err := protojson.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(searchTestdata), 0777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(searchTestdata, append(data, '\n'), 0666); err != nil {
			t.Fatal(err)
		}
	}
	if !core.WITH_DB {
		t.Skip("needs the with_db item data")
	}
	data, err := os.ReadFile(searchTestdata)
	if err != nil {
		t.Fatal(err)
	}
	req := &proto.OptimizeGearRequest{}
	if err := protojson.Unmarshal(data, req); err != nil {
		t.Fatal(err)
	}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotMainHand} {
		if len(pool.Slots[slot]) < 5 {
			t.Errorf("%s has %d candidates", slot, len(pool.Slots[slot]))
		}
	}
}

// The UI's item data and the server catalog, for pools like the UI's pool builder makes.
var (
	realDataOnce sync.Once
	realUIDB     *proto.UIDatabase
	realCatalog  *proto.ServerCatalog
	realDataErr  error
)

func loadRealData(tb testing.TB) (*proto.UIDatabase, *proto.ServerCatalog) {
	tb.Helper()
	if !core.WITH_DB {
		tb.Skip("needs the with_db item data")
	}
	realDataOnce.Do(func() {
		read := func(path string, m goproto.Message) error {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(data, m)
		}
		realUIDB, realCatalog = &proto.UIDatabase{}, &proto.ServerCatalog{}
		if realDataErr = read("../../assets/database/db.json", realUIDB); realDataErr == nil {
			realDataErr = read("../../assets/database/server_catalog.json", realCatalog)
		}
	})
	if realDataErr != nil {
		tb.Fatal(realDataErr)
	}
	return realUIDB, realCatalog
}

// Mirrors of the UI's equip tables in ui/core/proto_utils/utils.ts, for the test pools.
var (
	classMaxArmor = map[proto.Class]proto.ArmorType{
		proto.Class_ClassDruid:       proto.ArmorType_ArmorTypeLeather,
		proto.Class_ClassHunter:      proto.ArmorType_ArmorTypeMail,
		proto.Class_ClassMage:        proto.ArmorType_ArmorTypeCloth,
		proto.Class_ClassPaladin:     proto.ArmorType_ArmorTypePlate,
		proto.Class_ClassPriest:      proto.ArmorType_ArmorTypeCloth,
		proto.Class_ClassRogue:       proto.ArmorType_ArmorTypeLeather,
		proto.Class_ClassShaman:      proto.ArmorType_ArmorTypeMail,
		proto.Class_ClassWarlock:     proto.ArmorType_ArmorTypeCloth,
		proto.Class_ClassWarrior:     proto.ArmorType_ArmorTypePlate,
		proto.Class_ClassDeathknight: proto.ArmorType_ArmorTypePlate,
	}
	// weapon type -> can use a two-hander of it
	classWeapons = map[proto.Class]map[proto.WeaponType]bool{
		proto.Class_ClassDruid: {proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeFist: false,
			proto.WeaponType_WeaponTypeMace: true, proto.WeaponType_WeaponTypeOffHand: false,
			proto.WeaponType_WeaponTypePolearm: true, proto.WeaponType_WeaponTypeStaff: true},
		proto.Class_ClassMage: {proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeOffHand: false,
			proto.WeaponType_WeaponTypeStaff: true, proto.WeaponType_WeaponTypeSword: false},
		proto.Class_ClassPaladin: {proto.WeaponType_WeaponTypeAxe: true, proto.WeaponType_WeaponTypeMace: true, proto.WeaponType_WeaponTypeOffHand: false,
			proto.WeaponType_WeaponTypePolearm: true, proto.WeaponType_WeaponTypeShield: false, proto.WeaponType_WeaponTypeSword: true},
		proto.Class_ClassRogue: {proto.WeaponType_WeaponTypeAxe: false, proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeFist: false,
			proto.WeaponType_WeaponTypeMace: false, proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypeSword: false},
		proto.Class_ClassWarrior: {proto.WeaponType_WeaponTypeAxe: true, proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeFist: false,
			proto.WeaponType_WeaponTypeMace: true, proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypePolearm: true,
			proto.WeaponType_WeaponTypeShield: false, proto.WeaponType_WeaponTypeStaff: true, proto.WeaponType_WeaponTypeSword: true},
	}
	classRanged = map[proto.Class][]proto.RangedWeaponType{
		proto.Class_ClassDruid:   {proto.RangedWeaponType_RangedWeaponTypeIdol},
		proto.Class_ClassMage:    {proto.RangedWeaponType_RangedWeaponTypeWand},
		proto.Class_ClassPaladin: {proto.RangedWeaponType_RangedWeaponTypeLibram},
		proto.Class_ClassRogue: {proto.RangedWeaponType_RangedWeaponTypeBow, proto.RangedWeaponType_RangedWeaponTypeCrossbow,
			proto.RangedWeaponType_RangedWeaponTypeGun, proto.RangedWeaponType_RangedWeaponTypeThrown},
		proto.Class_ClassWarrior: {proto.RangedWeaponType_RangedWeaponTypeBow, proto.RangedWeaponType_RangedWeaponTypeCrossbow,
			proto.RangedWeaponType_RangedWeaponTypeGun, proto.RangedWeaponType_RangedWeaponTypeThrown},
	}
	dualWieldSpecs = []proto.Spec{proto.Spec_SpecEnhancementShaman, proto.Spec_SpecHunter, proto.Spec_SpecRogue, proto.Spec_SpecWarrior,
		proto.Spec_SpecProtectionWarrior, proto.Spec_SpecDeathknight, proto.Spec_SpecTankDeathknight}
	hordeRaces = []proto.Race{proto.Race_RaceOrc, proto.Race_RaceUndead, proto.Race_RaceTauren, proto.Race_RaceTroll, proto.Race_RaceBloodElf}
)

// metaConditions mirrors the WotLK metas' conditions in ui/core/proto_utils/gems.ts, as the pool
// builder's linear constraints: minimum red, yellow and blue.
var metaConditions = map[int32][3]int32{
	41285: {0, 0, 2}, 41307: {1, 1, 1}, 41333: {3, 0, 0}, 41335: {2, 1, 0}, 41377: {1, 0, 2}, 41339: {1, 2, 0},
	41375: {1, 1, 1}, 41376: {2, 0, 0}, 41378: {0, 2, 1}, 41379: {2, 0, 1}, 41380: {1, 0, 2}, 41381: {0, 2, 1},
	41382: {1, 1, 1}, 41385: {1, 0, 2}, 41389: {2, 1, 0}, 41395: {2, 0, 1}, 41396: {2, 0, 1}, 41397: {0, 0, 3},
	41398: {1, 1, 1}, 41400: {1, 1, 1}, 41401: {1, 1, 1}, 44076: {1, 2, 0}, 44078: {1, 1, 1}, 44081: {2, 0, 1},
	44082: {1, 0, 2}, 44084: {0, 2, 1}, 44087: {0, 0, 3}, 44088: {0, 1, 2}, 44089: {1, 1, 1},
}

func playerFaction(race proto.Race) proto.Faction {
	if slices.Contains(hordeRaces, race) {
		return proto.Faction_Horde
	}
	return proto.Faction_Alliance
}

// eligibleSlots mirrors getEligibleItemSlots: two-handers go in either hand, for Titan's Grip.
func eligibleSlots(item *proto.UIItem) []proto.ItemSlot {
	switch item.Type {
	case proto.ItemType_ItemTypeWeapon:
		switch item.HandType {
		case proto.HandType_HandTypeMainHand:
			return []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand}
		case proto.HandType_HandTypeOffHand:
			return []proto.ItemSlot{proto.ItemSlot_ItemSlotOffHand}
		}
		return []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand}
	case proto.ItemType_ItemTypeFinger:
		return []proto.ItemSlot{proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2}
	case proto.ItemType_ItemTypeTrinket:
		return []proto.ItemSlot{proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2}
	}
	return []proto.ItemSlot{core.ItemTypeToSlot(item.Type)}
}

// canEquipItem mirrors the UI's canEquipItem.
func canEquipItem(item *proto.UIItem, class proto.Class, spec proto.Spec, slot proto.ItemSlot) bool {
	if len(item.ClassAllowlist) > 0 && !slices.Contains(item.ClassAllowlist, class) {
		return false
	}
	switch item.Type {
	case proto.ItemType_ItemTypeFinger, proto.ItemType_ItemTypeTrinket:
		return true
	case proto.ItemType_ItemTypeWeapon:
		twoHand, ok := classWeapons[class][item.WeaponType]
		if !ok {
			return false
		}
		held := item.WeaponType == proto.WeaponType_WeaponTypeShield || item.WeaponType == proto.WeaponType_WeaponTypeOffHand
		if (item.HandType == proto.HandType_HandTypeOffHand || item.HandType == proto.HandType_HandTypeOneHand && slot == proto.ItemSlot_ItemSlotOffHand) &&
			!held && !slices.Contains(dualWieldSpecs, spec) {
			return false
		}
		if item.HandType == proto.HandType_HandTypeTwoHand && !twoHand {
			return false
		}
		return !(item.HandType == proto.HandType_HandTypeTwoHand && slot == proto.ItemSlot_ItemSlotOffHand && spec != proto.Spec_SpecWarrior)
	case proto.ItemType_ItemTypeRanged:
		return slices.Contains(classRanged[class], item.RangedWeaponType)
	}
	return classMaxArmor[class] >= item.ArmorType
}

// enchantFits mirrors enchantAppliesToItem and canEquipEnchant, and leaves out enchants a profession
// the player lacks gates.
func enchantFits(e *proto.UIEnchant, item *proto.UIItem, slot proto.ItemSlot, class proto.Class, professions []proto.Profession) bool {
	types := append([]proto.ItemType{e.Type}, e.ExtraTypes...)
	fits := false
	for _, t := range types {
		switch {
		case t == proto.ItemType_ItemTypeWeapon:
			fits = fits || slot == proto.ItemSlot_ItemSlotMainHand || slot == proto.ItemSlot_ItemSlotOffHand
		case t == proto.ItemType_ItemTypeFinger:
			fits = fits || slot == proto.ItemSlot_ItemSlotFinger1 || slot == proto.ItemSlot_ItemSlotFinger2
		default:
			fits = fits || core.ItemTypeToSlot(t) == slot && t != proto.ItemType_ItemTypeTrinket
		}
	}
	switch {
	case !fits:
		return false
	case e.EnchantType == proto.EnchantType_EnchantTypeTwoHand && item.HandType != proto.HandType_HandTypeTwoHand:
		return false
	case (e.EnchantType == proto.EnchantType_EnchantTypeShield) != (item.WeaponType == proto.WeaponType_WeaponTypeShield):
		return false
	case e.EnchantType == proto.EnchantType_EnchantTypeStaff && item.WeaponType != proto.WeaponType_WeaponTypeStaff:
		return false
	case item.WeaponType == proto.WeaponType_WeaponTypeOffHand:
		return false
	case slot == proto.ItemSlot_ItemSlotRanged && !slices.Contains([]proto.RangedWeaponType{proto.RangedWeaponType_RangedWeaponTypeBow,
		proto.RangedWeaponType_RangedWeaponTypeCrossbow, proto.RangedWeaponType_RangedWeaponTypeGun}, item.RangedWeaponType):
		return false
	case len(e.ClassAllowlist) > 0 && !slices.Contains(e.ClassAllowlist, class):
		return false
	}
	return e.RequiredProfession == proto.Profession_ProfessionUnknown || slices.Contains(professions, e.RequiredProfession)
}

// realisticPool builds the pool the UI's pool builder would for the player at a content phase: the
// class's usable items per slot at catalog tier <= 12 + phase, PvE only, for its faction or either,
// with the enchants and gems that fit the same way. Item data stays in core's database, so the pool
// carries none. keep, when set, narrows the items.
func realisticPool(tb testing.TB, player *proto.Player, phase int32, keep func(*proto.UIItem) bool) *proto.CandidatePool {
	tb.Helper()
	uidb, catalog := loadRealData(tb)
	rows := make(map[int32]*proto.CatalogItem, len(catalog.Items))
	for _, row := range catalog.Items {
		rows[row.Id] = row
	}
	faction := playerFaction(player.Race)
	usable := func(id int32) *proto.CatalogItem {
		row := rows[id]
		if row == nil || row.Pvp || row.ProgressionTier > 12+phase {
			return nil
		}
		if row.Faction != proto.Faction_Unknown && row.Faction != faction {
			return nil
		}
		return row
	}
	spec := core.PlayerProtoToSpec(player)
	professions := core.ProtoToProfessions(player)

	pool := &proto.CandidatePool{CatalogDate: catalog.Date}
	slots := map[proto.ItemSlot]*proto.SlotPool{}
	var catalogIDs []int32
	for _, item := range uidb.Items {
		row := usable(item.Id)
		if row == nil || keep != nil && !keep(item) {
			continue
		}
		if _, ok := core.LookupItem(item.Id); !ok {
			continue
		}
		added := false
		for _, slot := range eligibleSlots(item) {
			if !canEquipItem(item, player.Class, spec, slot) {
				continue
			}
			sp := slots[slot]
			if sp == nil {
				sp = &proto.SlotPool{Slot: slot}
				slots[slot] = sp
			}
			sp.ItemIds = append(sp.ItemIds, item.Id)
			var enchants []int32
			for _, e := range uidb.Enchants {
				if enchantFits(e, item, slot, player.Class, professions) && !slices.Contains(enchants, e.EffectId) {
					if _, ok := core.LookupEnchant(e.EffectId); ok {
						enchants = append(enchants, e.EffectId)
					}
				}
			}
			if len(enchants) > 0 {
				sp.EnchantOptions = append(sp.EnchantOptions, &proto.ItemEnchantOptions{ItemId: item.Id, EnchantIds: enchants})
			}
			added = true
		}
		if added {
			catalogIDs = append(catalogIDs, item.Id)
		}
	}
	for slot := proto.ItemSlot(0); int(slot) < NumSlots; slot++ {
		if sp := slots[slot]; sp != nil {
			pool.Slots = append(pool.Slots, sp)
		}
	}
	for _, gem := range uidb.Gems {
		if usable(gem.Id) == nil {
			continue
		}
		if _, ok := core.LookupGem(gem.Id); !ok {
			continue
		}
		if gem.Color == proto.GemColor_GemColorMeta {
			cond, ok := metaConditions[gem.Id]
			if !ok {
				continue
			}
			mc := &proto.MetaGemCondition{GemId: gem.Id}
			for i, n := range cond {
				if n > 0 {
					c := &proto.MetaColorConstraint{MinTotal: n}
					switch i {
					case 0:
						c.Red = 1
					case 1:
						c.Yellow = 1
					case 2:
						c.Blue = 1
					}
					mc.Constraints = append(mc.Constraints, c)
				}
			}
			pool.MetaConditions = append(pool.MetaConditions, mc)
		}
		pool.GemIds = append(pool.GemIds, gem.Id)
		catalogIDs = append(catalogIDs, gem.Id)
	}
	groups := map[int32]bool{}
	for _, id := range catalogIDs {
		row := rows[id]
		pool.CatalogItems = append(pool.CatalogItems, row)
		if row.LimitCategory != 0 {
			groups[row.LimitCategory] = true
		}
	}
	for _, g := range catalog.LimitGroups {
		if groups[g.Id] {
			pool.LimitGroups = append(pool.LimitGroups, g)
		}
	}
	return pool
}
