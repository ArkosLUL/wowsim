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
