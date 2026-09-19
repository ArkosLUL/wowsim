package azerothcore

import (
	"slices"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// catalogFixture builds CatalogRows by hand: a few real maps with their difficulties, plus
// whatever items, holders and sources a test adds.
type catalogFixture struct {
	rows CatalogRows
}

const (
	mapEasternKingdoms = 0
	mapOutland         = 530
	mapUlduar          = 603
	mapToC             = 649
	mapICC             = 631
	mapRubySanctum     = 724
	mapUtgardeKeep     = 574
	mapAlteracValley   = 30
	mapSkybreaker      = 672
)

func newCatalogFixture() *catalogFixture {
	f := &catalogFixture{}
	f.rows.ClassicPhases = map[int32]int32{}
	for _, m := range []CatalogMapRow{
		{ID: mapEasternKingdoms, Type: mapTypeContinent, Name: "Eastern Kingdoms", Expansion: 0},
		{ID: mapOutland, Type: mapTypeContinent, Name: "Outland", Expansion: 1},
		{ID: mapNorthrend, Type: mapTypeContinent, Name: "Northrend", Expansion: 2},
		{ID: mapNaxxramas, Type: mapTypeRaid, Name: "Naxxramas", Expansion: 2},
		{ID: mapUlduar, Type: mapTypeRaid, Name: "Ulduar", Expansion: 2},
		{ID: mapToC, Type: mapTypeRaid, Name: "Trial of the Crusader", Expansion: 2},
		{ID: mapICC, Type: mapTypeRaid, Name: "Icecrown Citadel", Expansion: 2},
		{ID: mapRubySanctum, Type: mapTypeRaid, Name: "The Ruby Sanctum", Expansion: 2},
		{ID: mapOnyxiasLair, Type: mapTypeRaid, Name: "Onyxia's Lair", Expansion: 0},
		{ID: mapVaultOfArchavon, Type: mapTypeRaid, Name: "Vault of Archavon", Expansion: 2},
		{ID: mapUtgardeKeep, Type: mapTypeDungeon, Name: "Utgarde Keep", Expansion: 2},
		{ID: mapAlteracValley, Type: mapTypeBattleground, Name: "Alterac Valley", Expansion: 0},
		{ID: mapSkybreaker, Type: mapTypeContinent, Name: "Transport: The Skybreaker", Expansion: 2},
	} {
		f.rows.Maps = append(f.rows.Maps, m)
	}
	for _, m := range []int32{mapToC, mapICC, mapRubySanctum} {
		for d, players := range []int32{10, 25, 10, 25} {
			f.rows.MapDifficulties = append(f.rows.MapDifficulties, MapDifficultyRow{Map: m, Difficulty: int32(d), MaxPlayers: players})
		}
	}
	for _, m := range []int32{mapNaxxramas, mapUlduar, mapOnyxiasLair, mapVaultOfArchavon} {
		f.rows.MapDifficulties = append(f.rows.MapDifficulties,
			MapDifficultyRow{Map: m, Difficulty: 0, MaxPlayers: 10}, MapDifficultyRow{Map: m, Difficulty: 1, MaxPlayers: 25})
	}
	// mod-individual-progression's 40-man modes, from mapdifficulty_dbc
	f.rows.MapDifficulties = append(f.rows.MapDifficulties,
		MapDifficultyRow{Map: mapNaxxramas, Difficulty: 2, MaxPlayers: 40}, MapDifficultyRow{Map: mapOnyxiasLair, Difficulty: 2, MaxPlayers: 40},
		MapDifficultyRow{Map: mapUtgardeKeep, Difficulty: 0, MaxPlayers: 5}, MapDifficultyRow{Map: mapUtgardeKeep, Difficulty: 1, MaxPlayers: 5})
	return f
}

// gear adds an epic head piece the catalog covers.
func (f *catalogFixture) gear(id int32) *CatalogItemRow {
	f.rows.Items = append(f.rows.Items, CatalogItemRow{Entry: id, Name: "Gear", Class: 4, Quality: 4, ItemLevel: 232, InventoryType: 1})
	return &f.rows.Items[len(f.rows.Items)-1]
}

// material adds an item the catalog leaves out: tokens, currencies, reagents, ores.
func (f *catalogFixture) material(id int32) {
	f.rows.Items = append(f.rows.Items, CatalogItemRow{Entry: id, Name: "Material", Class: 15})
}

func (f *catalogFixture) creature(entry, lootID int32, difficultyEntries ...int32) {
	c := CatalogCreatureRow{Entry: entry, Name: "Boss", LootID: lootID}
	copy(c.DifficultyEntries[:], difficultyEntries)
	f.rows.Creatures = append(f.rows.Creatures, c)
}

func (f *catalogFixture) spawn(entry, mapID, spawnMask int32) {
	f.rows.CreatureSpawns = append(f.rows.CreatureSpawns, SpawnRow{Entry: entry, Map: mapID, SpawnMask: spawnMask, PhaseMask: phaseNormal})
}

func (f *catalogFixture) loot(store LootStore, entry int32, items ...int32) {
	for _, id := range items {
		f.rows.Loot = append(f.rows.Loot, LootRow{Store: store, Entry: entry, Item: id})
	}
}

// drop places a boss with its own loot table on a map.
func (f *catalogFixture) drop(boss, mapID, spawnMask int32, items ...int32) {
	f.creature(boss, boss)
	f.spawn(boss, mapID, spawnMask)
	f.loot(LootCreature, boss, items...)
}

func (f *catalogFixture) vendor(npc, mapID, item, extendedCost int32) {
	if !slices.ContainsFunc(f.rows.Creatures, func(c CatalogCreatureRow) bool { return c.Entry == npc }) {
		f.creature(npc, 0)
		f.spawn(npc, mapID, 1)
	}
	f.rows.Vendors = append(f.rows.Vendors, VendorRow{Entry: npc, Item: item, ExtendedCost: extendedCost})
}

func (f *catalogFixture) resolve(t *testing.T) (map[int32]*proto.CatalogItem, *proto.ServerCatalog, *CatalogStats) {
	t.Helper()
	catalog, stats := ResolveCatalog(&f.rows)
	items := map[int32]*proto.CatalogItem{}
	for _, it := range catalog.Items {
		items[it.Id] = it
	}
	return items, catalog, stats
}

func sourceKinds(item *proto.CatalogItem) []proto.CatalogSourceKind {
	var kinds []proto.CatalogSourceKind
	for _, s := range item.Sources {
		kinds = append(kinds, s.Kind)
	}
	return kinds
}

func TestResolveCatalogMapTiers(t *testing.T) {
	const loot = 9000
	tests := []struct {
		name      string
		mapID     int32
		spawnMask int32
		phaseMask uint32
		boss      int32
		wantTier  int32
		wantKind  proto.CatalogSourceKind
		wantName  string
	}{
		{"Naxxramas 10", mapNaxxramas, 1, phaseNormal, 1, 13, proto.CatalogSourceKind_CatalogSourceRaid10, "Naxxramas 10: Boss"},
		{"Naxx40 on difficulty 2", mapNaxxramas, 4, phaseNormal, 1, naxx40Tier, proto.CatalogSourceKind_CatalogSourceRaid25, "Naxxramas 40: Boss"},
		{"Ulduar 10", mapUlduar, 1, phaseNormal, 1, 14, proto.CatalogSourceKind_CatalogSourceRaid10, "Ulduar 10: Boss"},
		{"ToC 25 heroic", mapToC, 8, phaseNormal, 1, 15, proto.CatalogSourceKind_CatalogSourceRaid25Heroic, "Trial of the Crusader 25 Heroic: Boss"},
		{"ICC 10 heroic", mapICC, 4, phaseNormal, 1, 16, proto.CatalogSourceKind_CatalogSourceRaid10Heroic, "Icecrown Citadel 10 Heroic: Boss"},
		{"Ruby Sanctum", mapRubySanctum, 2, phaseNormal, 1, 17, proto.CatalogSourceKind_CatalogSourceRaid25, "The Ruby Sanctum 25: Boss"},
		{"level-80 Onyxia", mapOnyxiasLair, 2, phaseNormal, 1, onyxiaTier, proto.CatalogSourceKind_CatalogSourceRaid25, "Onyxia's Lair 25: Boss"},
		{"vanilla Onyxia", mapOnyxiasLair, 4, phaseNormal, 1, 0, proto.CatalogSourceKind_CatalogSourceRaid25, "Onyxia's Lair 40: Boss"},
		{"Emalon", mapVaultOfArchavon, 1, phaseNormal, 33993, 14, proto.CatalogSourceKind_CatalogSourceRaid10, "Vault of Archavon 10: Boss"},
		{"VoA trash", mapVaultOfArchavon, 1, phaseNormal, 1, 13, proto.CatalogSourceKind_CatalogSourceRaid10, "Vault of Archavon 10: Boss"},
		{"Koralon's phase", mapVaultOfArchavon, 1, ippPhaseII, 1, 15, proto.CatalogSourceKind_CatalogSourceRaid10, "Vault of Archavon 10: Boss"},
		{"heroic dungeon", mapUtgardeKeep, 2, phaseNormal, 1, 13, proto.CatalogSourceKind_CatalogSourceDungeonHeroic, "Utgarde Keep Heroic: Boss"},
		{"Northrend world drop", mapNorthrend, 1, phaseNormal, 1, 13, proto.CatalogSourceKind_CatalogSourceWorldDrop, "Northrend: Boss"},
		{"Argent Tournament phase", mapNorthrend, 1, ippPhase, 1, 15, proto.CatalogSourceKind_CatalogSourceWorldDrop, "Northrend: Boss"},
		{"Outland", mapOutland, 1, phaseNormal, 1, 8, proto.CatalogSourceKind_CatalogSourceWorldDrop, "Outland: Boss"},
		{"old world", mapEasternKingdoms, 1, phaseNormal, 1, 0, proto.CatalogSourceKind_CatalogSourceWorldDrop, "Eastern Kingdoms: Boss"},
		{"gunship transport flies in ICC", mapSkybreaker, 8, phaseNormal, 1, 16, proto.CatalogSourceKind_CatalogSourceRaid25Heroic, "Icecrown Citadel 25 Heroic: Boss"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCatalogFixture()
			f.rows.Transports = []TransportRow{{Map: mapSkybreaker, RouteMap: mapICC}}
			f.gear(100)
			f.creature(tt.boss, loot)
			f.rows.CreatureSpawns = append(f.rows.CreatureSpawns, SpawnRow{Entry: tt.boss, Map: tt.mapID, SpawnMask: tt.spawnMask, PhaseMask: tt.phaseMask})
			f.loot(LootCreature, loot, 100)

			items, _, _ := f.resolve(t)
			item := items[100]
			if item == nil {
				t.Fatal("item missing from the catalog")
			}
			if item.ProgressionTier != tt.wantTier {
				t.Errorf("tier = %d, want %d", item.ProgressionTier, tt.wantTier)
			}
			if len(item.Sources) != 1 {
				t.Fatalf("sources = %v", item.Sources)
			}
			s := item.Sources[0]
			if s.Kind != tt.wantKind || s.Name != tt.wantName || s.NpcId != tt.boss || s.MapId == 0 && tt.mapID != 0 {
				t.Errorf("source = %v", s)
			}
		})
	}
}

// Difficulty entries hold the 25-man and heroic loot; missing ones fall back like
// Creature::InitEntry, heroics to their normal size.
func TestResolveCatalogDifficultyEntries(t *testing.T) {
	f := newCatalogFixture()
	f.gear(10)
	f.gear(25)
	f.gear(11)
	f.creature(1, 1, 2, 3, 0)
	f.creature(2, 2)
	f.creature(3, 3)
	f.spawn(1, mapToC, 15)
	f.loot(LootCreature, 1, 10)
	f.loot(LootCreature, 2, 25)
	f.loot(LootCreature, 3, 11)

	items, _, _ := f.resolve(t)
	if got := sourceKinds(items[10]); !slices.Equal(got, []proto.CatalogSourceKind{proto.CatalogSourceKind_CatalogSourceRaid10}) {
		t.Errorf("10N loot kinds = %v", got)
	}
	// 25H has no entry of its own, so it drops the 25N loot
	if got := sourceKinds(items[25]); !slices.Equal(got, []proto.CatalogSourceKind{proto.CatalogSourceKind_CatalogSourceRaid25, proto.CatalogSourceKind_CatalogSourceRaid25Heroic}) {
		t.Errorf("25N loot kinds = %v", got)
	}
	if got := sourceKinds(items[11]); !slices.Equal(got, []proto.CatalogSourceKind{proto.CatalogSourceKind_CatalogSourceRaid10Heroic}) {
		t.Errorf("10H loot kinds = %v", got)
	}
}

// Bosses without a spawn row: encounter credits, summons, and the known script summons.
func TestResolveCatalogPlacesSummonedHolders(t *testing.T) {
	f := newCatalogFixture()
	for _, id := range []int32{1, 2, 3, 4} {
		f.gear(id)
	}
	f.creature(100, 100) // credited in DungeonEncounter
	f.loot(LootCreature, 100, 1)
	f.rows.Encounters = []EncounterRow{{CreditEntry: 100, Map: mapICC, Difficulty: 1}}

	f.creature(200, 0) // a spawned controller that summons the boss
	f.spawn(200, mapUlduar, 3)
	f.creature(201, 201)
	f.loot(LootCreature, 201, 2)
	f.rows.Summons = []SummonRow{{SummonerKind: SummonerCreature, SummonerID: 200, Entry: 201}}

	// GO_CRUSADERS_CACHE_10, which only the ToC script summons
	f.rows.GameObjects = []CatalogGameObjectRow{{Entry: 195631, Type: 3, Name: "Champions' Cache", LootID: 50}, {Entry: 999, Type: 3, Name: "Lost Chest", LootID: 51}}
	f.loot(LootGameObject, 50, 3)
	f.loot(LootGameObject, 51, 4)

	items, _, stats := f.resolve(t)
	if it := items[1]; it.ProgressionTier != 16 || it.Sources[0].Kind != proto.CatalogSourceKind_CatalogSourceRaid25 {
		t.Errorf("encounter credit = %v", it)
	}
	if it := items[2]; it.ProgressionTier != 14 || len(it.Sources) != 2 {
		t.Errorf("summoned boss = %v", it)
	}
	if it := items[3]; it.ProgressionTier != 15 || it.Sources[0].GameobjectId != 195631 || it.Sources[0].Kind != proto.CatalogSourceKind_CatalogSourceRaid10 {
		t.Errorf("known script summon = %v", it)
	}
	if items[4] != nil || !slices.Contains(stats.Unresolved, 4) {
		t.Errorf("an unplaced chest's loot without a Classic phase should be left out as unresolved")
	}
	if len(stats.UnplacedHolders) != 1 || stats.UnplacedHolders[0].Entry != 999 || !stats.UnplacedHolders[0].GameObject {
		t.Errorf("unplaced holders = %v", stats.UnplacedHolders)
	}
}

// DungeonEncounter.dbc lists ICC and RS encounters under 0 and 1 only, but a summoned boss like
// Sindragosa fights in the heroics too, with her difficulty entries' loot.
func TestResolveCatalogEncounterCreditsCoverHeroics(t *testing.T) {
	const tenN, tenH, twentyFiveH = 1, 2, 3
	f := newCatalogFixture()
	for _, id := range []int32{tenN, tenH, twentyFiveH} {
		f.gear(id)
	}
	f.creature(36853, 36853, 38265, 38266, 38267)
	f.creature(38265, 38265)
	f.creature(38266, 38266)
	f.creature(38267, 38267)
	f.loot(LootCreature, 36853, tenN)
	f.loot(LootCreature, 38266, tenH)
	f.loot(LootCreature, 38267, twentyFiveH)
	f.rows.Encounters = []EncounterRow{{CreditEntry: 36853, Map: mapICC, Difficulty: 0}, {CreditEntry: 36853, Map: mapICC, Difficulty: 1}}

	items, _, _ := f.resolve(t)
	for id, want := range map[int32]proto.CatalogSourceKind{
		tenN:        proto.CatalogSourceKind_CatalogSourceRaid10,
		tenH:        proto.CatalogSourceKind_CatalogSourceRaid10Heroic,
		twentyFiveH: proto.CatalogSourceKind_CatalogSourceRaid25Heroic,
	} {
		if it := items[id]; it == nil || it.ProgressionTier != 16 || !slices.Equal(sourceKinds(it), []proto.CatalogSourceKind{want}) {
			t.Errorf("item %d = %v, want one %v source at 16", id, it, want)
		}
	}
}

// Loot on a difficulty entry nothing places is still awarded by a table, so it can fall back and
// gets reported, instead of counting as unobtainable.
func TestResolveCatalogUnplacedDifficultyEntry(t *testing.T) {
	const normal, heroic = 1, 2
	f := newCatalogFixture()
	f.gear(normal)
	f.gear(heroic)
	f.rows.ClassicPhases[heroic] = 4
	f.creature(10, 10, 11)
	f.creature(11, 11)
	f.spawn(10, mapUtgardeKeep, 1)
	f.loot(LootCreature, 10, normal)
	f.loot(LootCreature, 11, heroic)

	items, _, stats := f.resolve(t)
	if it := items[normal]; it == nil || it.ProgressionTier != 13 {
		t.Errorf("normal loot = %v", it)
	}
	if it := items[heroic]; it == nil || !it.FallbackTier || it.ProgressionTier != 16 || slices.Contains(stats.Unobtainable, heroic) {
		t.Errorf("unplaced heroic loot = %v, want a fallback", it)
	}
	if len(stats.UnplacedHolders) != 1 || stats.UnplacedHolders[0].Entry != 11 || stats.UnplacedHolders[0].GameObject {
		t.Errorf("unplaced holders = %v, want the heroic entry", stats.UnplacedHolders)
	}
}

func TestResolveCatalogConditions(t *testing.T) {
	rewarded := func(elseGroup, tier int32, negative bool) ConditionRow {
		return ConditionRow{SourceType: 1, SourceGroup: 1, SourceEntry: 100, ElseGroup: elseGroup, Type: conditionQuestRewarded,
			Value1: progressionQuestBase + tier, Negative: negative}
	}
	team := func(elseGroup, value int32) ConditionRow {
		return ConditionRow{SourceType: 1, SourceGroup: 1, SourceEntry: 100, ElseGroup: elseGroup, Type: conditionTeam, Value1: value}
	}
	tests := []struct {
		name        string
		conditions  []ConditionRow
		wantTier    int32
		wantFaction proto.Faction
		wantDropped bool
	}{
		{"none", nil, 13, proto.Faction_Unknown, false},
		{"needs tier 15", []ConditionRow{rewarded(0, 15, false)}, 15, proto.Faction_Unknown, false},
		{"only before 14 still opens at 13", []ConditionRow{rewarded(0, 14, true)}, 13, proto.Faction_Unknown, false},
		{"window 14 to 15", []ConditionRow{rewarded(0, 14, false), rewarded(0, 15, true)}, 14, proto.Faction_Unknown, false},
		{"else groups take the earliest", []ConditionRow{rewarded(0, 16, false), rewarded(1, 14, false)}, 14, proto.Faction_Unknown, false},
		{"empty window never drops", []ConditionRow{rewarded(0, 15, false), rewarded(0, 14, true)}, 0, proto.Faction_Unknown, true},
		{"horde only", []ConditionRow{team(0, conditionTeamHorde)}, 13, proto.Faction_Horde, false},
		{"alliance by negating horde", []ConditionRow{{SourceType: 1, SourceGroup: 1, SourceEntry: 100, Type: conditionTeam, Value1: conditionTeamHorde, Negative: true}}, 13, proto.Faction_Alliance, false},
		{"other conditions pass", []ConditionRow{{SourceType: 1, SourceGroup: 1, SourceEntry: 100, Type: 5, Value1: 1156}}, 13, proto.Faction_Unknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCatalogFixture()
			f.gear(100)
			f.drop(1, mapNaxxramas, 1, 100)
			f.rows.Conditions = tt.conditions
			items, _, stats := f.resolve(t)
			item := items[100]
			if tt.wantDropped {
				// the row can never drop, so nothing awards the item
				if item != nil || !slices.Contains(stats.Unobtainable, 100) {
					t.Errorf("item = %v, want it left out", item)
				}
				return
			}
			if item == nil || item.ProgressionTier != tt.wantTier || item.Faction != tt.wantFaction {
				t.Errorf("item = %v, want tier %d faction %v", item, tt.wantTier, tt.wantFaction)
			}
		})
	}
}

// Conditions on a reference row gate everything under it, together with the item's own.
func TestResolveCatalogReferenceConditions(t *testing.T) {
	f := newCatalogFixture()
	f.gear(100)
	f.creature(1, 1)
	f.spawn(1, mapNaxxramas, 1)
	f.rows.Loot = append(f.rows.Loot, LootRow{Store: LootCreature, Entry: 1, Item: 5, Reference: 34000},
		LootRow{Store: LootReference, Entry: 34000, Item: 100})
	f.rows.Conditions = []ConditionRow{
		{SourceType: 1, SourceGroup: 1, SourceEntry: 5, Type: conditionQuestRewarded, Value1: progressionQuestBase + 14},
		{SourceType: 10, SourceGroup: 34000, SourceEntry: 100, Type: conditionQuestRewarded, Value1: progressionQuestBase + 16},
	}
	items, _, _ := f.resolve(t)
	if got := items[100].ProgressionTier; got != 16 {
		t.Errorf("tier = %d, want 16", got)
	}
}

// A T10 piece: bought from an ICC-gated vendor with a Mark of Sanctification that drops in ICC 25.
func TestResolveCatalogTokenVendor(t *testing.T) {
	const piece, token, frost = 51125, 52025, 49426
	f := newCatalogFixture()
	f.gear(piece)
	f.material(token)
	f.material(frost)
	f.drop(1, mapICC, 2, token, frost)
	f.rows.ExtendedCosts = []ExtendedCostRow{{ID: 2795, Items: []int32{token, frost}}}
	f.vendor(35498, mapNorthrend, piece, 2795)

	items, _, stats := f.resolve(t)
	s := items[piece].Sources
	if items[piece].ProgressionTier != 16 || len(s) != 1 || s[0].Kind != proto.CatalogSourceKind_CatalogSourceVendor ||
		s[0].ViaItemId != token || s[0].ExtendedCostId != 2795 || s[0].NpcId != 35498 {
		t.Errorf("piece = %v", items[piece])
	}
	if items[token] != nil || stats.ItemTiers[token] != 16 {
		t.Errorf("tokens stay out of the catalog but keep a tier: %v, %d", items[token], stats.ItemTiers[token])
	}
}

// Emblem tiers are floors: a Triumph handed out at 13 doesn't open Triumph gear at 13.
func TestResolveCatalogCurrencyFloor(t *testing.T) {
	const triumph, satchel, gear = 47241, 43346, 100
	f := newCatalogFixture()
	f.gear(gear)
	f.material(triumph)
	f.material(satchel)
	f.drop(28860, mapNorthrend, 1, satchel)
	f.loot(LootItem, satchel, triumph)
	f.rows.ExtendedCosts = []ExtendedCostRow{{ID: 1, Items: []int32{triumph}}}
	f.vendor(35579, mapNorthrend, gear, 1)

	items, _, stats := f.resolve(t)
	if stats.ItemTiers[triumph] != 15 || items[gear].ProgressionTier != 15 {
		t.Errorf("Triumph = %d, gear = %v", stats.ItemTiers[triumph], items[gear])
	}
}

func TestResolveCatalogVendorGates(t *testing.T) {
	tests := []struct {
		name      string
		script    string
		condition *ConditionRow
		wantTier  int32
	}{
		{"open vendor", "", nil, 13},
		{"ToC emblem vendor", "npc_ipp_wotlk_totc", nil, 15},
		{"ICC emblem vendor", "npc_ipp_wotlk_icc", nil, 16},
		{"hidden before, no lower bound", "npc_ipp_pre_wotlk", nil, 13},
		{"vendor item condition", "", &ConditionRow{SourceType: conditionSourceVendor, SourceGroup: 32172, SourceEntry: 100,
			Type: conditionQuestRewarded, Value1: progressionQuestBase + 15}, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCatalogFixture()
			f.gear(100)
			f.vendor(32172, mapNorthrend, 100, 0)
			f.rows.Creatures[0].ScriptName = tt.script
			if tt.condition != nil {
				f.rows.Conditions = []ConditionRow{*tt.condition}
			}
			items, _, _ := f.resolve(t)
			if got := items[100].ProgressionTier; got != tt.wantTier {
				t.Errorf("tier = %d, want %d", got, tt.wantTier)
			}
		})
	}
}

// Crafted items take their latest reagent; epic gems come out of Titanium Ore at 13.
func TestResolveCatalogCrafted(t *testing.T) {
	const (
		ulduarReagent, northrendReagent, crafted = 45087, 36913, 100
		ore, rawGem, cutGem                      = 36910, 36919, 40111
	)
	f := newCatalogFixture()
	f.gear(crafted)
	f.material(ulduarReagent)
	f.material(northrendReagent)
	f.material(ore)
	f.material(rawGem)
	f.rows.Items = append(f.rows.Items, CatalogItemRow{Entry: cutGem, Name: "Bold Cardinal Ruby", Class: itemClassGem, Quality: 4, GemProperties: 1})
	f.drop(1, mapUlduar, 1, ulduarReagent)
	f.drop(2, mapNorthrend, 1, northrendReagent)
	f.rows.GameObjects = []CatalogGameObjectRow{{Entry: 189980, Type: 3, Name: "Titanium Vein", LootID: 60}}
	f.rows.GameObjectSpawns = []SpawnRow{{Entry: 189980, Map: mapNorthrend, SpawnMask: 1, PhaseMask: phaseNormal}}
	f.loot(LootGameObject, 60, ore)
	f.loot(LootProspecting, ore, rawGem)
	f.rows.Spells = []CreateSpellRow{
		{ID: 1, Name: "Crafted Belt", Creates: []int32{crafted}, Reagents: []int32{northrendReagent, ulduarReagent}, Profession: proto.Profession_Blacksmithing},
		{ID: 2, Name: "Bold Cardinal Ruby", Creates: []int32{cutGem}, Reagents: []int32{rawGem}, Profession: proto.Profession_Jewelcrafting},
		// not a profession spell, and nothing casts it
		{ID: 3, Name: "Test Spell", Creates: []int32{crafted}},
	}

	items, _, stats := f.resolve(t)
	belt := items[crafted]
	if belt.ProgressionTier != 14 || len(belt.Sources) != 1 {
		t.Fatalf("belt = %v", belt)
	}
	if s := belt.Sources[0]; s.Kind != proto.CatalogSourceKind_CatalogSourceCrafted || s.Profession != proto.Profession_Blacksmithing ||
		s.SpellId != 1 || s.ViaItemId != ulduarReagent || s.Name != "Blacksmithing: Crafted Belt" {
		t.Errorf("belt source = %v", s)
	}
	gem := items[cutGem]
	if gem == nil || !gem.IsGem || gem.ProgressionTier != 13 {
		t.Errorf("cut gem = %v", gem)
	}
	if stats.ItemTiers[rawGem] != 13 {
		t.Errorf("raw gem tier = %d", stats.ItemTiers[rawGem])
	}
}

// Transmutes and the like loop back on themselves; the loop can't pull a tier below its sources.
func TestResolveCatalogCycles(t *testing.T) {
	f := newCatalogFixture()
	f.material(1)
	f.material(2)
	f.gear(100)
	f.drop(10, mapUlduar, 1, 1)
	f.rows.Spells = []CreateSpellRow{
		{ID: 1, Creates: []int32{2}, Reagents: []int32{1}, Profession: proto.Profession_Alchemy},
		{ID: 2, Creates: []int32{1}, Reagents: []int32{2}, Profession: proto.Profession_Alchemy},
		{ID: 3, Creates: []int32{100}, Reagents: []int32{2}, Profession: proto.Profession_Tailoring},
		// a loop with no way in resolves to nothing
		{ID: 4, Creates: []int32{3}, Reagents: []int32{4}, Profession: proto.Profession_Alchemy},
		{ID: 5, Creates: []int32{4}, Reagents: []int32{3}, Profession: proto.Profession_Alchemy},
	}
	f.material(3)
	f.material(4)
	items, _, stats := f.resolve(t)
	if items[100].ProgressionTier != 14 || stats.ItemTiers[2] != 14 {
		t.Errorf("crafted = %v, reagent tier %d", items[100], stats.ItemTiers[2])
	}
	if _, ok := stats.ItemTiers[3]; ok {
		t.Error("a closed loop resolved to a tier")
	}
}

func TestResolveCatalogQuests(t *testing.T) {
	const (
		reward, chainReward = 100, 101
		iccDrop, tocDrop    = 200, 201
		provided, unsourced = 202, 203
		scriptDrop          = 204
		dalaranNPC          = 28776
	)
	f := newCatalogFixture()
	f.gear(reward)
	f.gear(chainReward)
	for _, id := range []int32{iccDrop, tocDrop, provided, unsourced, scriptDrop} {
		f.material(id)
	}
	f.drop(1, mapICC, 1, iccDrop)
	f.drop(2, mapToC, 1, tocDrop)
	f.creature(dalaranNPC, 0)
	f.spawn(dalaranNPC, mapNorthrend, 1)
	// scriptDrop only comes from an object a quest script summons, which nothing places
	f.rows.GameObjects = []CatalogGameObjectRow{{Entry: 201937, Type: 3, LootID: 70}}
	f.loot(LootGameObject, 70, scriptDrop)
	f.rows.Quests = []QuestRow{
		{ID: 1, Title: "Needs an ICC drop", RewardItems: []int32{reward},
			RequiredItems: []int32{iccDrop, provided, unsourced, scriptDrop}, ProvidedItems: []int32{provided}},
		{ID: 2, Title: "Needs a ToC drop", RequiredItems: []int32{tocDrop}},
		{ID: 3, Title: "Follow-up", PrevQuest: 2, RewardItems: []int32{chainReward}, RequiredRepFaction: 1156, RequiredRepValue: 21000},
		{ID: 4, Title: "Nobody starts it", RewardItems: []int32{reward}},
	}
	f.rows.QuestStarters = []QuestStarterRow{
		{Quest: 1, Kind: QuestStarterCreature, Entry: dalaranNPC},
		{Quest: 2, Kind: QuestStarterCreature, Entry: dalaranNPC},
		{Quest: 3, Kind: QuestStarterCreature, Entry: dalaranNPC},
	}

	items, _, _ := f.resolve(t)
	if it := items[reward]; it.ProgressionTier != 16 || len(it.Sources) != 1 || it.Sources[0].QuestId != 1 ||
		it.Sources[0].Name != "Quest: Needs an ICC drop" {
		t.Errorf("reward = %v", it)
	}
	s := items[chainReward].Sources
	if items[chainReward].ProgressionTier != 15 || s[0].Kind != proto.CatalogSourceKind_CatalogSourceReputation ||
		s[0].ReputationFactionId != 1156 || s[0].ReputationRank != 6 {
		t.Errorf("chain reward = %v", items[chainReward])
	}
}

// A quest item that's only unresolved because its own quest needs a script-given item still gates
// the quests that ask for it, once that script item stops counting.
func TestResolveCatalogQuestNeedsThroughUnresolvedChain(t *testing.T) {
	const (
		reward, questItem, scriptItem = 100, 200, 201
		iccNPC, dalaranNPC            = 1, 28776
	)
	f := newCatalogFixture()
	f.gear(reward)
	f.material(questItem)
	f.material(scriptItem)
	f.creature(iccNPC, 0)
	f.spawn(iccNPC, mapICC, 1)
	f.creature(dalaranNPC, 0)
	f.spawn(dalaranNPC, mapNorthrend, 1)
	f.rows.GameObjects = []CatalogGameObjectRow{{Entry: 201937, Type: 3, LootID: 70}}
	f.loot(LootGameObject, 70, scriptItem)
	f.rows.Quests = []QuestRow{
		{ID: 1, Title: "Hands out the quest item in ICC", RewardItems: []int32{questItem}, RequiredItems: []int32{scriptItem}},
		{ID: 2, Title: "Asks for the quest item", RewardItems: []int32{reward}, RequiredItems: []int32{questItem}},
	}
	f.rows.QuestStarters = []QuestStarterRow{
		{Quest: 1, Kind: QuestStarterCreature, Entry: iccNPC},
		{Quest: 2, Kind: QuestStarterCreature, Entry: dalaranNPC},
	}

	items, _, stats := f.resolve(t)
	if stats.ItemTiers[questItem] != 16 || items[reward].ProgressionTier != 16 {
		t.Errorf("quest item tier %d, reward = %v; want both at 16", stats.ItemTiers[questItem], items[reward])
	}
}

func TestResolveCatalogPvP(t *testing.T) {
	const honorOnly, resilient, both, markOnly = 100, 101, 102, 103
	f := newCatalogFixture()
	f.gear(honorOnly)
	f.gear(resilient).HasResilience = true
	f.gear(both)
	f.gear(markOnly)
	f.material(43589)
	f.drop(1, mapNorthrend, 1, 43589)
	f.rows.ExtendedCosts = []ExtendedCostRow{{ID: 1, HonorPoints: 1000}, {ID: 2, Items: []int32{43589}}}
	f.vendor(10, mapNorthrend, honorOnly, 1)
	f.vendor(10, mapNorthrend, both, 1)
	f.vendor(10, mapNorthrend, markOnly, 2)
	f.drop(2, mapUlduar, 1, resilient, both)

	items, _, _ := f.resolve(t)
	if it := items[honorOnly]; !it.Pvp || it.ProgressionTier != 13 {
		t.Errorf("honor-only item = %v", it)
	}
	if it := items[resilient]; !it.Pvp || it.ProgressionTier != 14 {
		t.Errorf("resilience item = %v", it)
	}
	if it := items[both]; it.Pvp || it.ProgressionTier != 14 || len(it.Sources) != 1 {
		t.Errorf("item with a PvE source = %v, want the PvP vendor left out", it)
	}
	if it := items[markOnly]; !it.Pvp {
		t.Errorf("Wintergrasp mark item = %v", it)
	}
}

func TestResolveCatalogBattlegroundLootIsPvP(t *testing.T) {
	f := newCatalogFixture()
	f.gear(100)
	f.drop(1, mapAlteracValley, 1, 100)
	items, _, _ := f.resolve(t)
	if !items[100].Pvp {
		t.Errorf("battleground loot = %v", items[100])
	}
}

func TestResolveCatalogFallbackAndGaps(t *testing.T) {
	f := newCatalogFixture()
	f.gear(100) // only in an unplaced chest, but Classic knows it
	f.gear(101) // nothing awards it
	f.rows.GameObjects = []CatalogGameObjectRow{{Entry: 999, Type: 3, Name: "Lost Chest", LootID: 1}}
	f.loot(LootGameObject, 1, 100)
	f.rows.ClassicPhases[100] = 4
	f.rows.ClassicPhases[101] = 2

	items, _, stats := f.resolve(t)
	it := items[100]
	if it == nil || !it.FallbackTier || it.ProgressionTier != 16 || len(it.Sources) != 1 || it.Sources[0].ProgressionTier != 16 {
		t.Errorf("fallback item = %v", it)
	}
	if !slices.Equal(stats.Fallback, []int32{100}) || !slices.Equal(stats.Unobtainable, []int32{101}) || items[101] != nil {
		t.Errorf("stats = fallback %v, unobtainable %v", stats.Fallback, stats.Unobtainable)
	}
}

func TestResolveCatalogItemFields(t *testing.T) {
	f := newCatalogFixture()
	dbw := f.gear(50363)
	dbw.MaxCount, dbw.ItemLimitCategory, dbw.StatsCount, dbw.HasEffect = 1, 47, 2, true
	unique := f.gear(2)
	unique.Flags = itemFlagUniqueEquipped
	horde := f.gear(3)
	horde.FlagsExtra = itemFlag2HordeOnly
	alliance := f.gear(4)
	alliance.AllowableRace = raceMaskAlliance
	f.rows.Items = append(f.rows.Items, CatalogItemRow{Entry: 42142, Name: "Bold Dragon's Eye", Class: itemClassGem, Quality: 4,
		GemProperties: 1, ItemLimitCategory: 2, RequiredSkill: 755, RequiredSkillRank: 350})
	f.drop(1, mapICC, 1, 50363, 2, 3, 4, 42142)
	f.rows.LimitCategories = []LimitCategoryRow{
		{ID: 2, Name: "Jeweler's Gems", MaxCount: 3, Mode: 1},
		{ID: 47, Name: "Deathbringer's Will", MaxCount: 1, Mode: 1},
		{ID: 99, Name: "Unused", MaxCount: 1},
	}

	items, catalog, _ := f.resolve(t)
	if it := items[50363]; it.MaxCount != 1 || it.LimitCategory != 47 || it.StatsCount != 2 || !it.HasEffect || it.UniqueEquipped {
		t.Errorf("DBW = %v", it)
	}
	if !items[2].UniqueEquipped || items[3].Faction != proto.Faction_Horde || items[4].Faction != proto.Faction_Alliance {
		t.Errorf("flags: unique %v, horde %v, alliance %v", items[2].UniqueEquipped, items[3].Faction, items[4].Faction)
	}
	eye := items[42142]
	if !eye.IsGem || eye.RequiredProfession != proto.Profession_Jewelcrafting || eye.RequiredSkillRank != 350 || eye.LimitCategory != 2 {
		t.Errorf("Dragon's Eye = %v", eye)
	}
	if len(catalog.LimitGroups) != 2 || catalog.LimitGroups[0].MaxEquipped != 3 || catalog.LimitGroups[1].Name != "Deathbringer's Will" {
		t.Errorf("limit groups = %v", catalog.LimitGroups)
	}
}

func TestResolveCatalogCollapsesSources(t *testing.T) {
	f := newCatalogFixture()
	f.gear(100)
	for npc := int32(1); npc <= 5; npc++ {
		f.creature(npc, 1)
		f.spawn(npc, mapNorthrend, 1)
	}
	f.loot(LootCreature, 1, 100)
	// more than maxSources ways to get it: they fold into one per kind and tier
	f.gear(200)
	for npc := int32(10); npc < 10+maxSources+1; npc++ {
		f.vendor(npc, mapNorthrend, 200, npc)
		f.rows.ExtendedCosts = append(f.rows.ExtendedCosts, ExtendedCostRow{ID: npc})
	}

	items, _, _ := f.resolve(t)
	if s := items[100].Sources; len(s) != 1 || s[0].NpcId != 0 || s[0].Name != "Northrend: 5 creatures" {
		t.Errorf("world drop sources = %v", s)
	}
	if s := items[200].Sources; len(s) != 1 || s[0].Name != "Northrend: 9 sources" || s[0].MapId != mapNorthrend {
		t.Errorf("vendor sources = %v", s)
	}
}

func TestResolveCatalogContainersAndTurnIns(t *testing.T) {
	const box, inner, gearA, gearB, token = 10, 11, 100, 101, 12
	f := newCatalogFixture()
	f.material(box)
	f.material(inner)
	f.material(token)
	f.gear(gearA)
	f.gear(gearB)
	f.drop(1, mapUlduar, 2, box, token)
	f.loot(LootItem, box, inner)
	f.loot(LootItem, inner, gearA)
	f.rows.TokenTurnIns = []TokenTurnInRow{{Token: token, Result: gearB}}

	items, _, _ := f.resolve(t)
	if s := items[gearA].Sources; items[gearA].ProgressionTier != 14 || s[0].ViaItemId != inner ||
		s[0].Kind != proto.CatalogSourceKind_CatalogSourceRaid25 {
		t.Errorf("nested container = %v", items[gearA])
	}
	if s := items[gearB].Sources; items[gearB].ProgressionTier != 14 || s[0].ViaItemId != token || s[0].Name != "Token turn-in: Material" {
		t.Errorf("token turn-in = %v", items[gearB])
	}
}

// Spell loot from an item's on-use: a caster nothing awards shouldn't be what the source points at.
func TestResolveCatalogSpellLootViaResolvedCaster(t *testing.T) {
	const lostBox, box, gear, spell = 10, 11, 100, 5000
	f := newCatalogFixture()
	f.material(lostBox)
	f.material(box)
	f.rows.Items[0].UseSpells = []int32{spell}
	f.rows.Items[1].UseSpells = []int32{spell}
	f.gear(gear)
	f.drop(1, mapUlduar, 2, box)
	f.rows.Spells = []CreateSpellRow{{ID: spell, Name: "Open", SpellLoot: true}}
	f.loot(LootSpell, spell, gear)

	items, _, _ := f.resolve(t)
	it := items[gear]
	if it == nil || it.ProgressionTier != 14 || len(it.Sources) != 1 {
		t.Fatalf("gear = %v", it)
	}
	if s := it.Sources[0]; s.ViaItemId != box || s.Kind != proto.CatalogSourceKind_CatalogSourceRaid25 {
		t.Errorf("source = %v, want it via the box that drops", s)
	}
}

func TestPhaseGate(t *testing.T) {
	tests := []struct {
		mapID int32
		mask  uint32
		want  int32
	}{
		{mapNorthrend, phaseNormal, 0},
		{mapNorthrend, 0xFFFFFFFF, 0},
		{mapNorthrend, ippPhase, 15},
		{mapVaultOfArchavon, ippPhase, 14},
		{mapVaultOfArchavon, ippPhaseIII, 16},
		{mapOutland, ippPhase, 0},
	}
	for _, tt := range tests {
		if got := phaseGate(tt.mapID, tt.mask); got != tt.want {
			t.Errorf("phaseGate(%d, %#x) = %d, want %d", tt.mapID, tt.mask, got, tt.want)
		}
	}
}

func TestRaceMaskFaction(t *testing.T) {
	tests := []struct {
		mask int32
		want proto.Faction
	}{
		{-1, proto.Faction_Unknown},
		{0, proto.Faction_Unknown},
		{32767, proto.Faction_Unknown},
		{raceMaskAlliance, proto.Faction_Alliance},
		{raceMaskHorde, proto.Faction_Horde},
		{2, proto.Faction_Horde},
	}
	for _, tt := range tests {
		if got := raceMaskFaction(tt.mask); got != tt.want {
			t.Errorf("raceMaskFaction(%d) = %v, want %v", tt.mask, got, tt.want)
		}
	}
}

func TestReputationRank(t *testing.T) {
	for value, want := range map[int32]int32{0: 3, 2999: 3, 3000: 4, 9000: 5, 21000: 6, 42000: 7, -5000: 1} {
		if got := reputationRank(value); got != want {
			t.Errorf("reputationRank(%d) = %d, want %d", value, got, want)
		}
	}
}
