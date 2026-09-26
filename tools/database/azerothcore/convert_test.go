package azerothcore

import (
	"slices"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
)

func TestConvertItemKeepsFormOnlyEquipSpellsOutOfStats(t *testing.T) {
	feralAP := applyAura(AuraModAttackPower, 0, 1059)
	feralAP.ID = 44916
	feralAP.Stances = 0x40000091 // cat, bear, dire bear, moonkin
	plainAP := applyAura(AuraModAttackPower, 0, 44)
	plainAP.ID = 15810
	dbc := &DBC{Spells: map[int32]*SpellEntry{feralAP.ID: &feralAP, plainAP.ID: &plainAP}}

	row := &ItemRow{Entry: 30883, SpellIDs: [5]int32{44916, 15810}, SpellTriggers: [5]int32{ItemSpellTriggerOnEquip, ItemSpellTriggerOnEquip}}
	converted := ConvertItem(row, dbc)

	if got := converted.Item.Stats[proto.Stat_StatAttackPower]; got != 44 {
		t.Errorf("attack power = %v, want 44 from the plain aura only", got)
	}
	if len(converted.EffectSpells) != 1 || converted.EffectSpells[0].SpellID != 44916 {
		t.Errorf("effect spells = %+v, want the form-only spell", converted.EffectSpells)
	}
}

func TestConvertItemRelicClass(t *testing.T) {
	for _, tc := range []struct {
		name           string
		subclass       int32
		allowableClass int32
		want           []proto.Class
	}{
		{"open idol", 8, -1, []proto.Class{proto.Class_ClassDruid}},
		{"open sigil", 10, 32767, []proto.Class{proto.Class_ClassDeathknight}},
		{"restricted libram", 7, 2, []proto.Class{proto.Class_ClassPaladin}},
	} {
		row := &ItemRow{Class: 4, Subclass: tc.subclass, InventoryType: inventoryTypeRelic, AllowableClass: tc.allowableClass}
		if got := ConvertItem(row, &DBC{}).Item.ClassAllowlist; !slices.Equal(got, tc.want) {
			t.Errorf("%s: class allowlist = %v, want %v", tc.name, got, tc.want)
		}
	}

	shield := &ItemRow{Class: 4, Subclass: 6, InventoryType: 14, AllowableClass: -1}
	if got := ConvertItem(shield, &DBC{}).Item.ClassAllowlist; got != nil {
		t.Errorf("open shield got class allowlist %v", got)
	}
}

func TestSimSetName(t *testing.T) {
	for _, tc := range []struct {
		dbcName, itemName, want string
	}{
		{"Gladiator's Pursuit", "Vengeful Gladiator's Chain Armor", "Vengeful Gladiator's Pursuit"},
		{"Gladiator's Pursuit", "Brutal Gladiator's Chain Armor", "Brutal Gladiator's Pursuit"},
		{"Gladiator's Pursuit", "Merciless Gladiator's Chain Armor", "Merciless Gladiator's Pursuit"},
		{"Gladiator's Pursuit", "Gladiator's Chain Armor", "Gladiator's Pursuit"},
		{"Gladiator's Pursuit", "Furious Gladiator's Chain Armor", "Gladiator's Pursuit"},
		{"Kirin Tor Garb", "Valorous Kirin Tor Hood", "Kirin'dor Garb"},
		{"Kirin Tor Garb", "Conqueror's Kirin Tor Hood", "Kirin Tor Garb"},
		{"Conqueror's Scourgeborne Battlegear", "Conqueror's Scourgeborne Helmet", "Scourgeborne Battlegear"},
	} {
		if got := simSetName(tc.dbcName, tc.itemName); got != tc.want {
			t.Errorf("simSetName(%q, %q) = %q, want %q", tc.dbcName, tc.itemName, got, tc.want)
		}
	}
}

// The stats mod-reforging reads: item_template's rows in order, zero values dropped, so the count
// is the StatsCount the worldserver builds. A negative value still counts.
func TestConvertItemServerStats(t *testing.T) {
	row := &ItemRow{Entry: 1}
	row.StatTypes = [10]int32{7, 3, 32, 31, 6}
	row.StatValues = [10]int32{50, 0, 83, 40, -5}

	got := ConvertItem(row, &DBC{}).Item.ServerStats
	want := []*proto.ItemStat{{StatType: 7, Value: 50}, {StatType: 32, Value: 83}, {StatType: 31, Value: 40}, {StatType: 6, Value: -5}}
	if len(got) != len(want) {
		t.Fatalf("server stats = %v, want %v", got, want)
	}
	for i, stat := range want {
		if got[i].StatType != stat.StatType || got[i].Value != stat.Value {
			t.Errorf("server stat %d = type %d value %d, want type %d value %d",
				i, got[i].StatType, got[i].Value, stat.StatType, stat.Value)
		}
	}
}

func TestConvertItemHeirloomStats(t *testing.T) {
	dbc := &DBC{
		ScalingStatDistributions: map[int32]*ScalingStatDistributionEntry{
			5: {ID: 5, StatMod: [10]int32{7, 4, -1, -1, -1, -1, -1, -1, -1, -1}, Modifier: [10]int32{100, 60}, MaxLevel: 80},
		},
		ScalingStatValues: map[int32]*ScalingStatValuesEntry{
			80: {ID: 1, Level: 80, SSDMultiplierCols: [4]int32{5000}, ArmorModCols: [4]int32{200}, SpellPower: 50},
		},
	}
	// shoulder stat multiplier (0x1) + cloth shoulder armor (0x20) + spell power bonus (0x8000)
	row := &ItemRow{Entry: 1, ScalingStatDistribution: 5, ScalingStatValue: 0x1 | 0x20 | 0x8000}
	converted := ConvertItem(row, dbc)

	if converted.NotComparable != "" {
		t.Fatalf("NotComparable = %q, want a real heirloom to be comparable", converted.NotComparable)
	}
	stats := converted.Item.Stats
	if got := stats[proto.Stat_StatStamina]; got != 50 { // 5000 * 100 / 10000
		t.Errorf("stamina = %v, want 50", got)
	}
	if got := stats[proto.Stat_StatStrength]; got != 30 { // 5000 * 60 / 10000
		t.Errorf("strength = %v, want 30", got)
	}
	if got := stats[proto.Stat_StatSpellPower]; got != 50 {
		t.Errorf("spell power = %v, want the flat ScalingStatValues bonus (50)", got)
	}
	if got := stats[proto.Stat_StatArmor]; got != 200 {
		t.Errorf("armor = %v, want 200", got)
	}
}

// Player::_ApplyItemBonuses only builds a ScalingStatValuesEntry when ScalingStatValue is set, so a
// ScalingStatDistribution item with ScalingStatValue 0 never scales: it falls back to its own
// stat_type/value columns, same as any other item.
func TestConvertItemSkipsScalingWhenScalingStatValueZero(t *testing.T) {
	dbc := &DBC{ScalingStatDistributions: map[int32]*ScalingStatDistributionEntry{5: {ID: 5, MaxLevel: 80}}}
	row := &ItemRow{Entry: 1, ScalingStatDistribution: 5, ScalingStatValue: 0, StatTypes: [10]int32{7}, StatValues: [10]int32{20}}
	converted := ConvertItem(row, dbc)

	if converted.NotComparable != "" {
		t.Fatalf("NotComparable = %q, want comparable via the raw stat columns", converted.NotComparable)
	}
	if got := converted.Item.Stats[proto.Stat_StatStamina]; got != 20 {
		t.Errorf("stamina = %v, want 20 from stat_value1", got)
	}
}

func TestConvertItemHeirloomCapsLevel(t *testing.T) {
	dbc := &DBC{
		ScalingStatDistributions: map[int32]*ScalingStatDistributionEntry{
			5: {ID: 5, StatMod: [10]int32{7, -1, -1, -1, -1, -1, -1, -1, -1, -1}, Modifier: [10]int32{100}, MaxLevel: 70},
		},
		ScalingStatValues: map[int32]*ScalingStatValuesEntry{
			70: {ID: 1, Level: 70, SSDMultiplierCols: [4]int32{1000}},
			80: {ID: 2, Level: 80, SSDMultiplierCols: [4]int32{5000}},
		},
	}
	row := &ItemRow{Entry: 1, ScalingStatDistribution: 5, ScalingStatValue: 0x1}
	converted := ConvertItem(row, dbc)
	if got := converted.Item.Stats[proto.Stat_StatStamina]; got != 10 { // 1000 * 100 / 10000, level capped at 70
		t.Errorf("stamina = %v, want 10 (MaxLevel 70, not the level 80 row)", got)
	}
}

// _ApplyItemBonuses caps ssd_level unconditionally, even to a MaxLevel of 0.
func TestConvertItemHeirloomZeroMaxLevelHasNoScalingStatValuesRow(t *testing.T) {
	dbc := &DBC{ScalingStatDistributions: map[int32]*ScalingStatDistributionEntry{5: {ID: 5, MaxLevel: 0}}}
	row := &ItemRow{Entry: 1, ScalingStatDistribution: 5, ScalingStatValue: 0x1}
	converted := ConvertItem(row, dbc)
	if converted.NotComparable != "scaling stats" {
		t.Errorf("NotComparable = %q, want %q", converted.NotComparable, "scaling stats")
	}
}

func TestConvertItemHeirloomWeaponDamage(t *testing.T) {
	dbc := &DBC{
		ScalingStatDistributions: map[int32]*ScalingStatDistributionEntry{5: {ID: 5, MaxLevel: 80}},
		ScalingStatValues:        map[int32]*ScalingStatValuesEntry{80: {ID: 1, Level: 80, DPSModCols: [6]int32{0, 100}}},
	}
	row := &ItemRow{Entry: 1, ScalingStatDistribution: 5, ScalingStatValue: 0x400, Delay: 2800} // 2H weapon
	item := ConvertItem(row, dbc).Item

	// average = 100 * 2800ms / 1000; a 2H weapon spreads +-20% around it (1H spreads +-30%)
	if item.WeaponDamageMin != 224 || item.WeaponDamageMax != 336 {
		t.Errorf("weapon damage = %v-%v, want 224-336", item.WeaponDamageMin, item.WeaponDamageMax)
	}
}

func TestConvertItemHeirloomNotComparableWithoutScalingStatValuesRow(t *testing.T) {
	dbc := &DBC{ScalingStatDistributions: map[int32]*ScalingStatDistributionEntry{5: {ID: 5, MaxLevel: 80}}}
	row := &ItemRow{Entry: 1, ScalingStatDistribution: 5, ScalingStatValue: 1}
	converted := ConvertItem(row, dbc)
	if converted.NotComparable != "scaling stats" {
		t.Errorf("NotComparable = %q, want %q", converted.NotComparable, "scaling stats")
	}
}

func TestApplyToOverwritesZeroValuesAndLists(t *testing.T) {
	var wowheadStats database.Stats
	wowheadStats[proto.Stat_StatAttackPower] = 94
	item := &proto.UIItem{
		Id:              30883,
		Name:            "Pillar of Ferocity",
		Phase:           2,
		Ilvl:            141,
		Quality:         proto.ItemQuality_ItemQualityEpic,
		Stats:           wowheadStats[:],
		GemSockets:      []proto.GemColor{proto.GemColor_GemColorRed},
		SocketBonus:     wowheadStats[:],
		WeaponDamageMin: 313,
		WeaponDamageMax: 470,
		WeaponSpeed:     3,
		Heroic:          true,
		ClassAllowlist:  []proto.Class{proto.Class_ClassDruid},
		SetName:         "Wowhead Set",
		ServerStats:     []*proto.ItemStat{{StatType: 7, Value: 1}},
	}

	var serverStats database.Stats
	serverStats[proto.Stat_StatStrength] = 47
	server := &ConvertedItem{Item: &proto.UIItem{
		Id:      30883,
		Name:    "server name",
		Ilvl:    141,
		Quality: proto.ItemQuality_ItemQualityEpic,
		Stats:   serverStats[:],
		// zero or empty everywhere else
		SocketBonus: make([]float64, len(database.Stats{})),
	}}
	server.ApplyTo(item)

	if item.Name != "Pillar of Ferocity" || item.Phase != 2 {
		t.Errorf("fields the server doesn't own changed: name %q, phase %d", item.Name, item.Phase)
	}
	if item.Stats[proto.Stat_StatStrength] != 47 || item.Stats[proto.Stat_StatAttackPower] != 0 {
		t.Errorf("stats not replaced: %v", item.Stats)
	}
	if item.GemSockets != nil || item.ClassAllowlist != nil || item.ServerStats != nil {
		t.Errorf("lists not cleared: sockets %v, classes %v, server stats %v", item.GemSockets, item.ClassAllowlist, item.ServerStats)
	}
	if slices.ContainsFunc(item.SocketBonus, func(v float64) bool { return v != 0 }) {
		t.Errorf("socket bonus not cleared: %v", item.SocketBonus)
	}
	if item.WeaponDamageMin != 0 || item.WeaponDamageMax != 0 || item.WeaponSpeed != 0 || item.Heroic || item.SetName != "" {
		t.Errorf("zero values not applied: %+v", item)
	}

	item.Stats[proto.Stat_StatStrength] = 1
	if server.Item.Stats[proto.Stat_StatStrength] != 47 {
		t.Error("item shares its stats slice with the converted item")
	}
}
