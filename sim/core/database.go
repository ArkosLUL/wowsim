package core

import (
	"fmt"
	"sync"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"google.golang.org/protobuf/encoding/protojson"
)

var WITH_DB = false

// Every request can add to these (NewCharacter does), so they're only ever reached through
// AddToDatabase and the lookup functions below, which hold dbMu.
var itemsByID = map[int32]Item{}
var gemsByID = map[int32]Gem{}
var enchantsByEffectID = map[int32]Enchant{}

// dbMu guards the three maps above.
var dbMu sync.RWMutex

// AddToDatabase adds the items, enchants and gems the database doesn't have yet, and replaces a
// cached item without ServerStats with a later copy that has them: without them it can't be
// reforged. Lookups return copies, so a value looked up before a replacement stays valid.
func AddToDatabase(newDB *proto.SimDatabase) {
	dbMu.Lock()
	defer dbMu.Unlock()

	for _, v := range newDB.GetItems() {
		if existing, ok := itemsByID[v.Id]; !ok || (len(existing.ServerStats) == 0 && len(v.ServerStats) > 0) {
			itemsByID[v.Id] = ItemFromProto(v)
		}
	}

	for _, v := range newDB.GetEnchants() {
		if _, ok := enchantsByEffectID[v.EffectId]; !ok {
			enchantsByEffectID[v.EffectId] = EnchantFromProto(v)
		}
	}

	for _, v := range newDB.GetGems() {
		if _, ok := gemsByID[v.Id]; !ok {
			gemsByID[v.Id] = GemFromProto(v)
		}
	}
}

func LookupItem(id int32) (Item, bool) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	item, ok := itemsByID[id]
	return item, ok
}

func LookupGem(id int32) (Gem, bool) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	gem, ok := gemsByID[id]
	return gem, ok
}

func LookupEnchant(effectID int32) (Enchant, bool) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	enchant, ok := enchantsByEffectID[effectID]
	return enchant, ok
}

// ForEachItem calls f with every item in the database, in no particular order. f mustn't call
// AddToDatabase: the read lock is held throughout, so that would deadlock.
func ForEachItem(f func(Item)) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	for _, item := range itemsByID {
		f(item)
	}
}

// ForEachGem is ForEachItem for gems.
func ForEachGem(f func(Gem)) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	for _, gem := range gemsByID {
		f(gem)
	}
}

type Item struct {
	ID        int32
	Type      proto.ItemType
	ArmorType proto.ArmorType
	// Weapon Stats
	WeaponType       proto.WeaponType
	HandType         proto.HandType
	RangedWeaponType proto.RangedWeaponType
	WeaponDamageMin  float64
	WeaponDamageMax  float64
	SwingSpeed       float64

	Name    string
	Stats   stats.Stats // Stats applied to wearer
	Quality proto.ItemQuality
	SetName string // Empty string if not part of a set.

	// The item_template stat rows the server reforges from, empty for an item the item database
	// has no server row for.
	ServerStats []ItemStat

	GemSockets  []proto.GemColor
	SocketBonus stats.Stats

	// Modified for each instance of the item.
	Gems    []Gem
	Enchant Enchant
	// only set when valid, its stat change is already added to Stats
	Reforge *proto.ItemReforge

	//Internal use
	TempEnchant int32
}

// ItemStat is one item_template stat_type/stat_value pair; see proto.ItemStat.
type ItemStat struct {
	Type  int32
	Value int32
}

func ItemFromProto(pData *proto.SimItem) Item {
	return Item{
		ID:               pData.Id,
		Name:             pData.Name,
		Type:             pData.Type,
		ArmorType:        pData.ArmorType,
		WeaponType:       pData.WeaponType,
		HandType:         pData.HandType,
		RangedWeaponType: pData.RangedWeaponType,
		WeaponDamageMin:  pData.WeaponDamageMin,
		WeaponDamageMax:  pData.WeaponDamageMax,
		SwingSpeed:       pData.WeaponSpeed,
		Stats:            stats.FromFloatArray(pData.Stats),
		GemSockets:       pData.GemSockets,
		SocketBonus:      stats.FromFloatArray(pData.SocketBonus),
		SetName:          pData.SetName,
		ServerStats: MapSlice(pData.ServerStats, func(stat *proto.ItemStat) ItemStat {
			return ItemStat{Type: stat.StatType, Value: stat.Value}
		}),
	}
}

func (item *Item) ToItemSpecProto() *proto.ItemSpec {
	return &proto.ItemSpec{
		Id:      item.ID,
		Enchant: item.Enchant.EffectID,
		Gems:    MapSlice(item.Gems, func(gem Gem) int32 { return gem.ID }),
		Reforge: item.Reforge,
	}
}

type Enchant struct {
	EffectID int32 // Used by UI to apply effect to tooltip
	Stats    stats.Stats
}

func EnchantFromProto(pData *proto.SimEnchant) Enchant {
	return Enchant{
		EffectID: pData.EffectId,
		Stats:    stats.FromFloatArray(pData.Stats),
	}
}

type Gem struct {
	ID    int32
	Name  string
	Stats stats.Stats
	Color proto.GemColor
}

func GemFromProto(pData *proto.SimGem) Gem {
	return Gem{
		ID:    pData.Id,
		Name:  pData.Name,
		Stats: stats.FromFloatArray(pData.Stats),
		Color: pData.Color,
	}
}

type ItemSpec struct {
	ID      int32
	Enchant int32
	Gems    []int32
	Reforge *proto.ItemReforge
}

type Equipment [proto.ItemSlot_ItemSlotRanged + 1]Item

func (equipment *Equipment) MainHand() *Item {
	return &equipment[proto.ItemSlot_ItemSlotMainHand]
}

func (equipment *Equipment) OffHand() *Item {
	return &equipment[proto.ItemSlot_ItemSlotOffHand]
}

func (equipment *Equipment) Ranged() *Item {
	return &equipment[proto.ItemSlot_ItemSlotRanged]
}

func (equipment *Equipment) Head() *Item {
	return &equipment[proto.ItemSlot_ItemSlotHead]
}

func (equipment *Equipment) Hands() *Item {
	return &equipment[proto.ItemSlot_ItemSlotHands]
}

func (equipment *Equipment) Neck() *Item {
	return &equipment[proto.ItemSlot_ItemSlotNeck]
}

func (equipment *Equipment) Trinket1() *Item {
	return &equipment[proto.ItemSlot_ItemSlotTrinket1]
}

func (equipment *Equipment) Trinket2() *Item {
	return &equipment[proto.ItemSlot_ItemSlotTrinket2]
}

func (equipment *Equipment) Finger1() *Item {
	return &equipment[proto.ItemSlot_ItemSlotFinger1]
}

func (equipment *Equipment) Finger2() *Item {
	return &equipment[proto.ItemSlot_ItemSlotFinger2]
}

func (equipment *Equipment) EquipItem(item Item) {
	if item.Type == proto.ItemType_ItemTypeFinger {
		if equipment.Finger1().ID == 0 {
			*equipment.Finger1() = item
		} else {
			*equipment.Finger2() = item
		}
	} else if item.Type == proto.ItemType_ItemTypeTrinket {
		if equipment.Trinket1().ID == 0 {
			*equipment.Trinket1() = item
		} else {
			*equipment.Trinket2() = item
		}
	} else if item.Type == proto.ItemType_ItemTypeWeapon {
		if item.WeaponType == proto.WeaponType_WeaponTypeShield && equipment.MainHand().HandType != proto.HandType_HandTypeTwoHand {
			*equipment.OffHand() = item
		} else if item.HandType == proto.HandType_HandTypeMainHand || item.HandType == proto.HandType_HandTypeUnknown {
			*equipment.MainHand() = item
		} else if item.HandType == proto.HandType_HandTypeOffHand {
			*equipment.OffHand() = item
		} else if item.HandType == proto.HandType_HandTypeOneHand || item.HandType == proto.HandType_HandTypeTwoHand {
			if equipment.MainHand().ID == 0 {
				*equipment.MainHand() = item
			} else if equipment.OffHand().ID == 0 {
				*equipment.OffHand() = item
			}
		}
	} else {
		equipment[ItemTypeToSlot(item.Type)] = item
	}
}

func (equipment *Equipment) ToEquipmentSpecProto() *proto.EquipmentSpec {
	return &proto.EquipmentSpec{
		Items: MapSlice(equipment[:], func(item Item) *proto.ItemSpec {
			return item.ToItemSpecProto()
		}),
	}
}

// Structs used for looking up items/gems/enchants
type EquipmentSpec [proto.ItemSlot_ItemSlotRanged + 1]ItemSpec

func ProtoToEquipmentSpec(es *proto.EquipmentSpec) EquipmentSpec {
	var coreEquip EquipmentSpec
	for i, item := range es.Items {
		coreEquip[i] = ItemSpec{
			ID:      item.Id,
			Enchant: item.Enchant,
			Gems:    item.Gems,
			Reforge: item.Reforge,
		}
	}
	return coreEquip
}

// NewItem builds the item the spec names, with its enchant, gems and reforge. reforging is the
// raid's mod-reforging config, from Character.Server(); nil means the live server's, which is what
// callers outside a sim run want.
func NewItem(itemSpec ItemSpec, reforging *Reforging) Item {
	item := lookupItemSpec(itemSpec)
	if reforgeStats := ReforgeStats(&item, itemSpec.Reforge, reforging); reforgeStats != (stats.Stats{}) {
		item.Reforge = itemSpec.Reforge
		item.Stats = item.Stats.Add(reforgeStats)
	}
	return item
}

// Unlocks via defer so its panics don't leave the read lock held: sim runners recover from them,
// and a stuck lock would block every later AddToDatabase.
func lookupItemSpec(itemSpec ItemSpec) Item {
	dbMu.RLock()
	defer dbMu.RUnlock()

	item := Item{}
	if foundItem, ok := itemsByID[itemSpec.ID]; ok {
		item = foundItem
	} else {
		panic(fmt.Sprintf("No item with id: %d", itemSpec.ID))
	}

	if itemSpec.Enchant != 0 {
		if enchant, ok := enchantsByEffectID[itemSpec.Enchant]; ok {
			item.Enchant = enchant
		}
		// else {
		// 	panic(fmt.Sprintf("No enchant with id: %d", itemSpec.Enchant))
		// }
	}

	if len(itemSpec.Gems) > 0 {
		// Need to do this to account for possible extra gem sockets.
		numGems := len(item.GemSockets)
		if len(itemSpec.Gems) > numGems {
			numGems = len(itemSpec.Gems)
		}

		item.Gems = make([]Gem, numGems)
		for gemIdx, gemID := range itemSpec.Gems {
			if gem, ok := gemsByID[gemID]; ok {
				item.Gems[gemIdx] = gem
			} else {
				if gemID != 0 {
					panic(fmt.Sprintf("When parsing item %d, socket %d had gem with id: %d\nThis gem is not in the database.", itemSpec.ID, gemIdx, gemID))
				}
			}
		}
	}
	return item
}

func NewEquipmentSet(equipSpec EquipmentSpec, reforging *Reforging) Equipment {
	equipment := Equipment{}
	for _, itemSpec := range equipSpec {
		if itemSpec.ID != 0 {
			equipment.EquipItem(NewItem(itemSpec, reforging))
		}
	}
	return equipment
}

func ProtoToEquipment(es *proto.EquipmentSpec, reforging *Reforging) Equipment {
	return NewEquipmentSet(ProtoToEquipmentSpec(es), reforging)
}

// Like ItemSpec, but uses names for reference instead of ID.
type ItemStringSpec struct {
	Name    string
	Enchant string
	Gems    []string
}

func EquipmentSpecFromJsonString(jsonString string) *proto.EquipmentSpec {
	es := &proto.EquipmentSpec{}

	data := []byte(jsonString)
	if err := protojson.Unmarshal(data, es); err != nil {
		panic(err)
	}
	return es
}

// Adds in place, here and in TotalStats: every EquipStats call runs these, and a by-value Stats is a
// few hundred bytes. Don't reorder the additions, the float sums depend on the order.
func (equipment *Equipment) Stats() stats.Stats {
	equipStats := stats.Stats{}
	for i := range equipment {
		itemStats := equipment[i].TotalStats()
		equipStats.AddInplace(&itemStats)
	}
	return equipStats
}

// TotalStats returns the stats the item gives when worn: its own, enchant, gems and socket bonus.
func (item *Item) TotalStats() stats.Stats {
	itemStats := item.Stats
	itemStats.AddInplace(&item.Enchant.Stats)

	for i := range item.Gems {
		itemStats.AddInplace(&item.Gems[i].Stats)
	}

	// Check socket bonus
	if len(item.GemSockets) > 0 && len(item.Gems) >= len(item.GemSockets) {
		allMatch := true
		for gemIndex, socketColor := range item.GemSockets {
			if !ColorIntersects(socketColor, item.Gems[gemIndex].Color) {
				allMatch = false
				break
			}
		}

		if allMatch {
			itemStats.AddInplace(&item.SocketBonus)
		}
	}
	return itemStats
}

func ItemTypeToSlot(it proto.ItemType) proto.ItemSlot {
	switch it {
	case proto.ItemType_ItemTypeHead:
		return proto.ItemSlot_ItemSlotHead
	case proto.ItemType_ItemTypeNeck:
		return proto.ItemSlot_ItemSlotNeck
	case proto.ItemType_ItemTypeShoulder:
		return proto.ItemSlot_ItemSlotShoulder
	case proto.ItemType_ItemTypeBack:
		return proto.ItemSlot_ItemSlotBack
	case proto.ItemType_ItemTypeChest:
		return proto.ItemSlot_ItemSlotChest
	case proto.ItemType_ItemTypeWrist:
		return proto.ItemSlot_ItemSlotWrist
	case proto.ItemType_ItemTypeHands:
		return proto.ItemSlot_ItemSlotHands
	case proto.ItemType_ItemTypeWaist:
		return proto.ItemSlot_ItemSlotWaist
	case proto.ItemType_ItemTypeLegs:
		return proto.ItemSlot_ItemSlotLegs
	case proto.ItemType_ItemTypeFeet:
		return proto.ItemSlot_ItemSlotFeet
	case proto.ItemType_ItemTypeFinger:
		return proto.ItemSlot_ItemSlotFinger1
	case proto.ItemType_ItemTypeTrinket:
		return proto.ItemSlot_ItemSlotTrinket1
	case proto.ItemType_ItemTypeWeapon:
		return proto.ItemSlot_ItemSlotMainHand
	case proto.ItemType_ItemTypeRanged:
		return proto.ItemSlot_ItemSlotRanged
	}

	return 255
}

// See getEligibleItemSlots in proto_utils/utils.ts.
var itemTypeToSlotsMap = map[proto.ItemType][]proto.ItemSlot{
	proto.ItemType_ItemTypeHead:     {proto.ItemSlot_ItemSlotHead},
	proto.ItemType_ItemTypeNeck:     {proto.ItemSlot_ItemSlotNeck},
	proto.ItemType_ItemTypeShoulder: {proto.ItemSlot_ItemSlotShoulder},
	proto.ItemType_ItemTypeBack:     {proto.ItemSlot_ItemSlotBack},
	proto.ItemType_ItemTypeChest:    {proto.ItemSlot_ItemSlotChest},
	proto.ItemType_ItemTypeWrist:    {proto.ItemSlot_ItemSlotWrist},
	proto.ItemType_ItemTypeHands:    {proto.ItemSlot_ItemSlotHands},
	proto.ItemType_ItemTypeWaist:    {proto.ItemSlot_ItemSlotWaist},
	proto.ItemType_ItemTypeLegs:     {proto.ItemSlot_ItemSlotLegs},
	proto.ItemType_ItemTypeFeet:     {proto.ItemSlot_ItemSlotFeet},
	proto.ItemType_ItemTypeFinger:   {proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2},
	proto.ItemType_ItemTypeTrinket:  {proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2},
	proto.ItemType_ItemTypeRanged:   {proto.ItemSlot_ItemSlotRanged},
	// ItemType_ItemTypeWeapon is excluded intentionally - the slot cannot be decided based on type alone for weapons.
}

// EligibleSlotsForItem lists the slots an item can go in by its type and hand type alone. The slice
// can be shared, so don't modify it.
func EligibleSlotsForItem(item Item) []proto.ItemSlot {
	if slots, ok := itemTypeToSlotsMap[item.Type]; ok {
		return slots
	}

	if item.Type == proto.ItemType_ItemTypeWeapon {
		switch item.HandType {
		case proto.HandType_HandTypeTwoHand, proto.HandType_HandTypeMainHand:
			return []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand}
		case proto.HandType_HandTypeOffHand:
			return []proto.ItemSlot{proto.ItemSlot_ItemSlotOffHand}
		case proto.HandType_HandTypeOneHand:
			return []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand}
		}
	}

	return nil
}

func ColorIntersects(g proto.GemColor, o proto.GemColor) bool {
	if g == o {
		return true
	}
	if g == proto.GemColor_GemColorPrismatic || o == proto.GemColor_GemColorPrismatic {
		return true
	}
	if g == proto.GemColor_GemColorMeta {
		return o == proto.GemColor_GemColorMeta
	}
	if g == proto.GemColor_GemColorRed {
		return o == proto.GemColor_GemColorOrange || o == proto.GemColor_GemColorPurple
	}
	if g == proto.GemColor_GemColorBlue {
		return o == proto.GemColor_GemColorGreen || o == proto.GemColor_GemColorPurple
	}
	if g == proto.GemColor_GemColorYellow {
		return o == proto.GemColor_GemColorGreen || o == proto.GemColor_GemColorOrange
	}
	if g == proto.GemColor_GemColorOrange {
		return o == proto.GemColor_GemColorYellow || o == proto.GemColor_GemColorRed
	}
	if g == proto.GemColor_GemColorGreen {
		return o == proto.GemColor_GemColorYellow || o == proto.GemColor_GemColorBlue
	}
	if g == proto.GemColor_GemColorPurple {
		return o == proto.GemColor_GemColorBlue || o == proto.GemColor_GemColorRed
	}

	return false // dunno what else could be.
}
