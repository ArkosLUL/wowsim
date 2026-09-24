package sim

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/assets/database"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

type wearer struct {
	player    *proto.Player
	dualWield bool
}

// without a ranged weapon a hunter Auto Shoots at a 0 s speed, so the sim never ends
const wearerBowID = 9000002

var wearerDatabase = &proto.SimDatabase{
	Items: []*proto.SimItem{{
		Id:               wearerBowID,
		Type:             proto.ItemType_ItemTypeRanged,
		RangedWeaponType: proto.RangedWeaponType_RangedWeaponTypeBow,
		WeaponDamageMin:  100,
		WeaponDamageMax:  200,
		WeaponSpeed:      3,
	}},
}

// no talents, actions or gear but a hunter's bow: the items under test are all a player has
var wearers = []wearer{
	{&proto.Player{Name: "Death Knight", Class: proto.Class_ClassDeathknight, Race: proto.Race_RaceOrc,
		Spec: &proto.Player_Deathknight{Deathknight: &proto.Deathknight{Options: &proto.Deathknight_Options{}}}}, true},
	{&proto.Player{Name: "Druid", Class: proto.Class_ClassDruid, Race: proto.Race_RaceTauren,
		Spec: &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{}}}}, false},
	{&proto.Player{Name: "Hunter", Class: proto.Class_ClassHunter, Race: proto.Race_RaceOrc,
		Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{}}}}, true},
	{&proto.Player{Name: "Mage", Class: proto.Class_ClassMage, Race: proto.Race_RaceTroll,
		Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{}}}}, false},
	{&proto.Player{Name: "Paladin", Class: proto.Class_ClassPaladin, Race: proto.Race_RaceBloodElf,
		Spec: &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.RetributionPaladin_Options{}}}}, false},
	{&proto.Player{Name: "Priest", Class: proto.Class_ClassPriest, Race: proto.Race_RaceUndead,
		Spec: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{}}}}, false},
	{&proto.Player{Name: "Rogue", Class: proto.Class_ClassRogue, Race: proto.Race_RaceOrc,
		Spec: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.Rogue_Options{}}}}, true},
	{&proto.Player{Name: "Shaman", Class: proto.Class_ClassShaman, Race: proto.Race_RaceTroll,
		Spec: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{Options: &proto.EnhancementShaman_Options{
			Totems: &proto.ShamanTotems{},
		}}}}, true},
	{&proto.Player{Name: "Warlock", Class: proto.Class_ClassWarlock, Race: proto.Race_RaceOrc,
		Spec: &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.Warlock_Options{}}}}, false},
	{&proto.Player{Name: "Warrior", Class: proto.Class_ClassWarrior, Race: proto.Race_RaceOrc,
		Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}}}, true},
}

// what each class can equip at 80, allowlists aside
var maxArmorType = map[proto.Class]proto.ArmorType{
	proto.Class_ClassDeathknight: proto.ArmorType_ArmorTypePlate,
	proto.Class_ClassDruid:       proto.ArmorType_ArmorTypeLeather,
	proto.Class_ClassHunter:      proto.ArmorType_ArmorTypeMail,
	proto.Class_ClassMage:        proto.ArmorType_ArmorTypeCloth,
	proto.Class_ClassPaladin:     proto.ArmorType_ArmorTypePlate,
	proto.Class_ClassPriest:      proto.ArmorType_ArmorTypeCloth,
	proto.Class_ClassRogue:       proto.ArmorType_ArmorTypeLeather,
	proto.Class_ClassShaman:      proto.ArmorType_ArmorTypeMail,
	proto.Class_ClassWarlock:     proto.ArmorType_ArmorTypeCloth,
	proto.Class_ClassWarrior:     proto.ArmorType_ArmorTypePlate,
}

var rangedWeaponTypes = map[proto.Class][]proto.RangedWeaponType{
	proto.Class_ClassDeathknight: {proto.RangedWeaponType_RangedWeaponTypeSigil},
	proto.Class_ClassDruid:       {proto.RangedWeaponType_RangedWeaponTypeIdol},
	proto.Class_ClassHunter:      {proto.RangedWeaponType_RangedWeaponTypeBow, proto.RangedWeaponType_RangedWeaponTypeCrossbow, proto.RangedWeaponType_RangedWeaponTypeGun},
	proto.Class_ClassMage:        {proto.RangedWeaponType_RangedWeaponTypeWand},
	proto.Class_ClassPaladin:     {proto.RangedWeaponType_RangedWeaponTypeLibram},
	proto.Class_ClassPriest:      {proto.RangedWeaponType_RangedWeaponTypeWand},
	proto.Class_ClassRogue:       {proto.RangedWeaponType_RangedWeaponTypeBow, proto.RangedWeaponType_RangedWeaponTypeCrossbow, proto.RangedWeaponType_RangedWeaponTypeGun, proto.RangedWeaponType_RangedWeaponTypeThrown},
	proto.Class_ClassShaman:      {proto.RangedWeaponType_RangedWeaponTypeTotem},
	proto.Class_ClassWarlock:     {proto.RangedWeaponType_RangedWeaponTypeWand},
	proto.Class_ClassWarrior:     {proto.RangedWeaponType_RangedWeaponTypeBow, proto.RangedWeaponType_RangedWeaponTypeCrossbow, proto.RangedWeaponType_RangedWeaponTypeGun, proto.RangedWeaponType_RangedWeaponTypeThrown},
}

// each weapon type a class can use, and whether two-handed
var weaponTypes = map[proto.Class]map[proto.WeaponType]bool{
	proto.Class_ClassDeathknight: {proto.WeaponType_WeaponTypeAxe: true, proto.WeaponType_WeaponTypeMace: true, proto.WeaponType_WeaponTypePolearm: true, proto.WeaponType_WeaponTypeSword: true},
	proto.Class_ClassDruid: {proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeFist: false, proto.WeaponType_WeaponTypeMace: true,
		proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypeStaff: true, proto.WeaponType_WeaponTypePolearm: true},
	proto.Class_ClassHunter: {proto.WeaponType_WeaponTypeAxe: true, proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeFist: false,
		proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypePolearm: true, proto.WeaponType_WeaponTypeSword: true, proto.WeaponType_WeaponTypeStaff: true},
	proto.Class_ClassMage: {proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypeStaff: true, proto.WeaponType_WeaponTypeSword: false},
	proto.Class_ClassPaladin: {proto.WeaponType_WeaponTypeAxe: true, proto.WeaponType_WeaponTypeMace: true, proto.WeaponType_WeaponTypeOffHand: false,
		proto.WeaponType_WeaponTypePolearm: true, proto.WeaponType_WeaponTypeShield: false, proto.WeaponType_WeaponTypeSword: true},
	proto.Class_ClassPriest: {proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeMace: false, proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypeStaff: true},
	proto.Class_ClassRogue: {proto.WeaponType_WeaponTypeAxe: false, proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeFist: false,
		proto.WeaponType_WeaponTypeMace: false, proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypeSword: false},
	proto.Class_ClassShaman: {proto.WeaponType_WeaponTypeAxe: true, proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeFist: false,
		proto.WeaponType_WeaponTypeMace: true, proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypeShield: false, proto.WeaponType_WeaponTypeStaff: true},
	proto.Class_ClassWarlock: {proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypeStaff: true, proto.WeaponType_WeaponTypeSword: false},
	proto.Class_ClassWarrior: {proto.WeaponType_WeaponTypeAxe: true, proto.WeaponType_WeaponTypeDagger: false, proto.WeaponType_WeaponTypeFist: false,
		proto.WeaponType_WeaponTypeMace: true, proto.WeaponType_WeaponTypeOffHand: false, proto.WeaponType_WeaponTypePolearm: true,
		proto.WeaponType_WeaponTypeShield: false, proto.WeaponType_WeaponTypeStaff: true, proto.WeaponType_WeaponTypeSword: true},
}

func (w wearer) canWear(item core.Item) bool {
	class := w.player.Class
	switch item.Type {
	case proto.ItemType_ItemTypeFinger, proto.ItemType_ItemTypeTrinket:
		return true
	case proto.ItemType_ItemTypeWeapon:
		twoHand, ok := weaponTypes[class][item.WeaponType]
		switch {
		case !ok:
			return false
		case item.HandType == proto.HandType_HandTypeTwoHand:
			return twoHand
		case item.HandType == proto.HandType_HandTypeOffHand:
			return w.dualWield || item.WeaponType == proto.WeaponType_WeaponTypeShield || item.WeaponType == proto.WeaponType_WeaponTypeOffHand
		}
		return true
	case proto.ItemType_ItemTypeRanged:
		return slices.Contains(rangedWeaponTypes[class], item.RangedWeaponType)
	}
	return item.ArmorType <= maxArmorType[class]
}

// registeredByClass tells a class package's item effect from one of sim/common's by the function
// registered for it: core.ClassEffect's wrapper, or a class package's own closure.
func registeredByClass(effect core.ApplyEffect) bool {
	name := runtime.FuncForPC(reflect.ValueOf(effect).Pointer()).Name()
	if strings.HasPrefix(name, "github.com/wowsims/wotlk/sim/core.ClassEffect[") {
		return true
	}
	return !strings.HasPrefix(name, "github.com/wowsims/wotlk/sim/core.") && !strings.HasPrefix(name, "github.com/wowsims/wotlk/sim/common/")
}

type wearerCase struct {
	name  string
	items []core.Item
}

// Every set, and every item effect a class package registers, on each class that can equip the
// items, allowlist aside: another class must get a harmless no-op. The allowlisted class is left to
// its golden suite, which has the talents and rotation the effects may need.
func TestSetsAndClassItemEffectsOnEveryWearer(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("needs the item database (-tags=with_db)")
	}

	allowlists := map[int32][]proto.Class{}
	for _, item := range database.Load().Items {
		allowlists[item.Id] = item.ClassAllowlist
	}

	var cases []wearerCase
	for _, set := range (&core.ItemFilter{}).FindAllSets() {
		cases = append(cases, wearerCase{set.Name, set.Items()})
	}
	core.ForEachItem(func(item core.Item) {
		if effect, ok := core.ItemEffect(item.ID); ok && registeredByClass(effect) {
			cases = append(cases, wearerCase{fmt.Sprintf("%s-%d", item.Name, item.ID), []core.Item{item}})
		}
	})
	slices.SortFunc(cases, func(a, b wearerCase) int { return strings.Compare(a.name, b.name) })

	sims := 0
	for _, c := range cases {
		for _, w := range wearers {
			if !slices.ContainsFunc(c.items, func(item core.Item) bool { return !w.canWear(item) }) &&
				!slices.ContainsFunc(c.items, func(item core.Item) bool { return slices.Contains(allowlists[item.ID], w.player.Class) }) {
				sims++
				t.Run(strings.ReplaceAll(c.name, " ", "")+"-"+w.player.Class.String(), func(t *testing.T) {
					t.Parallel()
					runWearerSim(t, w, c.items)
				})
			}
		}
	}
	t.Logf("%d sets and class item effects, %d sims", len(cases), sims)
}

func runWearerSim(t *testing.T, w wearer, items []core.Item) {
	var equipment core.Equipment
	if w.player.Class == proto.Class_ClassHunter {
		equipment[proto.ItemSlot_ItemSlotRanged].ID = wearerBowID
	}
	for _, item := range items {
		equipment.EquipItem(item)
	}
	player := googleProto.Clone(w.player).(*proto.Player)
	player.Equipment = equipment.ToEquipmentSpecProto()
	player.Rotation = &proto.APLRotation{}
	player.Database = wearerDatabase

	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter: &proto.Encounter{
			Duration: 20,
			Targets:  []*proto.Target{StandardTarget},
		},
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true},
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
}
