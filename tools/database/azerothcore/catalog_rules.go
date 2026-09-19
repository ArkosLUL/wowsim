package azerothcore

import (
	"math"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// Progression tiers are mod-individual-progression's ProgressionState values: a character at tier
// N has rewarded quests 66000..66000+N. See docs/guide/azerothcore-server.md#progression-tiers.

// tierUnknown marks a source (or node) that doesn't resolve to any tier.
const tierUnknown int32 = math.MaxInt32

const progressionQuestBase = 66000

// Maps the module gates explicitly (IndividualProgressionPlayer.cpp's OnPlayerBeforeTeleport).
// Every other map opens with its expansion: expansionTiers.
var mapTiers = map[int32]int32{
	469: 1,  // Blackwing Lair
	309: 3,  // Zul'Gurub, RequiredZulGurubProgression's default
	509: 4,  // Ruins of Ahn'Qiraj
	531: 4,  // Temple of Ahn'Qiraj
	548: 9,  // Serpentshrine Cavern
	550: 9,  // Tempest Keep
	534: 10, // Hyjal Summit
	564: 10, // Black Temple
	568: 12, // Zul'Aman, RequiredZulAmanProgression's default
	580: 12, // Sunwell Plateau
	585: 12, // Magisters' Terrace
	603: 14, // Ulduar
	649: 15, // Trial of the Crusader
	650: 15, // Trial of the Champion
	631: 16, // Icecrown Citadel
	632: 16, // Forge of Souls
	658: 16, // Pit of Saron, behind FoS
	668: 16, // Halls of Reflection, behind FoS
	724: 17, // Ruby Sanctum
}

// Map.dbc expansion -> tier: vanilla is open from the start, Outland at PRE_TBC, Northrend at 13.
var expansionTiers = []int32{0, 8, 13}

const (
	mapOnyxiasLair        = 249
	mapNaxxramas          = 533
	mapVaultOfArchavon    = 624
	onyxiaTier            = 15 // level-80 Onyxia has no gate; the user counts her as ToC-era
	naxx40Tier            = 6  // PROGRESSION_AQ opens Naxx40
	raidDifficultyVanilla = 2
)

// VoA phases its bosses in (IndividualProgression.cpp); its trash opens with Northrend.
var vaultBossTiers = map[int32]int32{
	31125: 13, // Archavon
	33993: 14, // Emalon
	35013: 15, // Koralon
	38433: 16, // Toravon
}

// mod-individual-progression's phase bits (ipp_aware_npcs.sql). A spawn outside the normal phase
// only shows up where checkIPPhasing casts the matching aura.
const (
	phaseNormal  = 1
	ippPhase     = 65536
	ippPhaseII   = 131072
	ippPhaseIII  = 262144
	mapNorthrend = 571
)

// phaseGate is the lowest tier that sees a spawn with this phase mask. In Northrend only the
// Argent Tournament grounds phase in, from 15; VoA phases in Emalon, Koralon and Toravon at 14, 15
// and 16. Every other phased area is older content, all below 13.
func phaseGate(mapID int32, phaseMask uint32) int32 {
	if phaseMask&phaseNormal != 0 {
		return 0
	}
	switch mapID {
	case mapNorthrend:
		if phaseMask&ippPhase != 0 {
			return 15
		}
	case mapVaultOfArchavon:
		switch {
		case phaseMask&ippPhase != 0:
			return 14
		case phaseMask&ippPhaseII != 0:
			return 15
		case phaseMask&ippPhaseIII != 0:
			return 16
		}
	}
	return 0
}

// Emblem tiers from wotlk_emblems.sql's drop conditions (docs/guide/azerothcore-server.md). They're
// floors: a few one-off sources hand emblems out earlier without opening the vendors that take
// them, e.g. Sartharion's Satchel of Spoils holds a Triumph at 13, and Usuri Brightcoin trades
// Triumph down to Conquest.
var currencyTiers = map[int32]int32{
	40752: 13, // Heroism
	40753: 13, // Valor
	45624: 14, // Conquest
	47241: 15, // Triumph
	49426: 16, // Frost
}

// mapTier is the tier a creature or gameobject on this map and difficulty opens at. boss is the
// base creature entry, 0 for gameobjects.
func mapTier(maps map[int32]CatalogMapRow, mapID, difficulty, boss int32) int32 {
	switch mapID {
	case mapOnyxiasLair:
		// the module runs vanilla (40-man) Onyxia as difficulty 2
		if difficulty >= raidDifficultyVanilla {
			return 0
		}
		return onyxiaTier
	case mapNaxxramas:
		if difficulty >= raidDifficultyVanilla {
			return naxx40Tier
		}
	case mapVaultOfArchavon:
		if tier, ok := vaultBossTiers[boss]; ok {
			return tier
		}
	}
	if tier, ok := mapTiers[mapID]; ok {
		return tier
	}
	if m, ok := maps[mapID]; ok && m.Expansion >= 0 && int(m.Expansion) < len(expansionTiers) {
		return expansionTiers[m.Expansion]
	}
	return 0
}

// scriptGates: CanBeSeen scripts in IndividualProgressionAwareness.cpp, IndividualProgressionPvP.cpp
// and npc_archmage_timear.cpp, by ScriptName, with the lowest tier that sees the NPC or object.
// Scripts that only hide something from a tier on (npc_ipp_pre_*, npc_archmage_timear) set no
// lower bound, so they aren't listed.
var scriptGates = map[string]int32{
	"npc_ipp_preaq":              3,
	"npc_ipp_zg":                 3,
	"npc_ipp_we":                 3,
	"npc_ipp_aqwewar":            3,
	"npc_ipp_ds2":                3,
	"npc_ipp_aqwar":              4,
	"npc_ipp_aq":                 5,
	"npc_ipp_si":                 6,
	"npc_ipp_naxx40":             6,
	"npc_ipp_tbc":                8,
	"npc_ipp_tbc_pvp":            8,
	"npc_ipp_tbc_S1":             8,
	"npc_ipp_tbc_S2":             8,
	"npc_ipp_tbc_S3":             8,
	"npc_ipp_tbc_S4":             8,
	"npc_ipp_tbc_t3":             10,
	"npc_ipp_za":                 12,
	"npc_ipp_wotlk":              13,
	"npc_ipp_wotlk_S5":           13,
	"npc_ipp_wotlk_S6":           13,
	"npc_ipp_wotlk_S7":           13,
	"npc_ipp_wotlk_S8":           13,
	"npc_wg_queue":               13,
	"npc_ipp_wotlk_ulduar":       14,
	"npc_ipp_wotlk_totc":         15,
	"npc_archmage_landalock_3_3": 15,
	"npc_ipp_wotlk_icc":          16,
	"npc_archmage_landalock":     16,
	"gobject_ipp_preaq":          3,
	"gobject_ipp_aqwar":          4,
	"gobject_ipp_si":             6,
	"gobject_ipp_naxx40":         6,
	"gobject_ipp_tbc":            8,
	"gobject_ipp_tbc_t4":         12,
	"gobject_ipp_wotlk":          13,
}

// knownScriptSummons places creatures and gameobjects that only C++ scripts summon, so no spawn,
// encounter or summon row does. Entries and difficulties come from the instance headers in [ac]
// src/server/scripts.
type scriptSummon struct {
	GameObject   bool
	Entry        int32
	Map          int32
	Difficulties []int32
}

var knownScriptSummons = []scriptSummon{
	// Naxxramas: GO_HORSEMEN_CHEST_10/_25
	{true, 181366, 533, []int32{0}},
	{true, 193426, 533, []int32{1}},
	// Eye of Eternity: ALEXSTRASZA_GIFT, HEART_OF_MAGIC are DUNGEON_MODE(10, 25)
	{true, 193905, 616, []int32{0}},
	{true, 193967, 616, []int32{1}},
	{true, 194158, 616, []int32{0}},
	{true, 194159, 616, []int32{1}},
	// Ulduar, 10 then 25, hard modes after: Kologarn, Thorim, Mimiron, Algalon (ulduar.h)
	{true, 195046, 603, []int32{0}},
	{true, 195047, 603, []int32{1}},
	{true, 194312, 603, []int32{0}},
	{true, 194313, 603, []int32{0}},
	{true, 194314, 603, []int32{1}},
	{true, 194315, 603, []int32{1}},
	{true, 194789, 603, []int32{0}},
	{true, 194957, 603, []int32{0}},
	{true, 194956, 603, []int32{1}},
	{true, 194958, 603, []int32{1}},
	{true, 194821, 603, []int32{0}},
	{true, 194822, 603, []int32{1}},
	// Freya's Gift per elders left alive; only case labels in instance_ulduar.cpp, so the size is
	// read off the loot: 10-man chests top out at item level 219/232, 25-man at 232/239
	{true, 194324, 603, []int32{0}},
	{true, 194326, 603, []int32{0}},
	{true, 194328, 603, []int32{0}},
	{true, 194330, 603, []int32{0}},
	{true, 194325, 603, []int32{1}},
	{true, 194327, 603, []int32{1}},
	{true, 194329, 603, []int32{1}},
	{true, 194331, 603, []int32{1}},
	// Trial of the Crusader: Fjola carries the Twin Val'kyr loot but Eydis gets the credit;
	// GO_CRUSADERS_CACHE_10/_25/_10_H/_25_H; GO_TRIBUTE_CHEST_10H_*/_25H_*
	{false, 34497, 649, []int32{0, 1, 2, 3}},
	{true, 195631, 649, []int32{0}},
	{true, 195632, 649, []int32{1}},
	{true, 195633, 649, []int32{2}},
	{true, 195635, 649, []int32{3}},
	{true, 195665, 649, []int32{2}},
	{true, 195666, 649, []int32{2}},
	{true, 195667, 649, []int32{2}},
	{true, 195668, 649, []int32{2}},
	{true, 195669, 649, []int32{3}},
	{true, 195670, 649, []int32{3}},
	{true, 195671, 649, []int32{3}},
	{true, 195672, 649, []int32{3}},
	// Trial of the Champion: the Black Knight; GO_CHAMPIONS_LOOT, GO_EADRIC_LOOT, GO_PALETRESS_LOOT (_H)
	{false, 35451, 650, []int32{0, 1}},
	{true, 195709, 650, []int32{0}},
	{true, 195710, 650, []int32{1}},
	{true, 195374, 650, []int32{0}},
	{true, 195375, 650, []int32{1}},
	{true, 195323, 650, []int32{0}},
	{true, 195324, 650, []int32{1}},
	// Icecrown Citadel: GO_CACHE_OF_THE_DREAMWALKER_10N/_25N/_10H/_25H
	{true, 201959, 631, []int32{0}},
	{true, 202339, 631, []int32{1}},
	{true, 202338, 631, []int32{2}},
	{true, 202340, 631, []int32{3}},
	// Halls of Reflection: The Captain's Chest, normal then heroic (item level 219, 232)
	{true, 202212, 668, []int32{0}},
	{true, 202337, 668, []int32{1}},
	// Culling of Stratholme: GO_MALGANIS_CHEST_N/_H
	{true, 190663, 595, []int32{0}},
	{true, 193597, 595, []int32{1}},
	// holiday bosses: Ahune's Ice Chests in the Slave Pens, the Headless Horseman in Scarlet Monastery
	{true, 187892, 547, []int32{0}},
	{true, 188192, 547, []int32{1}},
	{false, 23682, 189, []int32{0}},
}

// PvP currencies: anything bought with them is a PvP source.
var pvpCurrencies = map[int32]bool{
	20558: true, // Warsong Gulch Mark of Honor
	20559: true, // Arathi Basin Mark of Honor
	20560: true, // Alterac Valley Mark of Honor
	29024: true, // Eye of the Storm Mark of Honor
	42425: true, // Strand of the Ancients Mark of Honor
	47395: true, // Isle of Conquest Mark of Honor
	43589: true, // Wintergrasp Mark of Honor
	43228: true, // Stone Keeper's Shard
	24579: true, // Mark of Honor Hold
	24581: true, // Mark of Thrallmar
	26045: true, // Halaa Battle Token
	26044: true, // Halaa Research Token
	28558: true, // Spirit Shard
	37836: true, // Venture Coin, from Venture Bay in Grizzly Hills
}

// Map.dbc map_type.
const (
	mapTypeContinent    = 0
	mapTypeDungeon      = 1
	mapTypeRaid         = 2
	mapTypeBattleground = 3
	mapTypeArena        = 4
)

func isPvPMap(mapType int32) bool {
	return mapType == mapTypeBattleground || mapType == mapTypeArena
}

// sourceKind labels a drop by where it happens. maxPlayers is MapDifficulty.dbc's, 0 when unknown.
func sourceKind(mapType, difficulty, maxPlayers int32) proto.CatalogSourceKind {
	switch mapType {
	case mapTypeDungeon:
		if difficulty > 0 {
			return proto.CatalogSourceKind_CatalogSourceDungeonHeroic
		}
		return proto.CatalogSourceKind_CatalogSourceDungeon
	case mapTypeRaid:
		small := maxPlayers > 0 && maxPlayers <= 10 || maxPlayers == 0 && difficulty%2 == 0
		// 40-man vanilla modes sit on difficulty 2 but aren't heroics
		heroic := difficulty >= 2 && maxPlayers <= 25
		switch {
		case small && heroic:
			return proto.CatalogSourceKind_CatalogSourceRaid10Heroic
		case small:
			return proto.CatalogSourceKind_CatalogSourceRaid10
		case heroic:
			return proto.CatalogSourceKind_CatalogSourceRaid25Heroic
		default:
			return proto.CatalogSourceKind_CatalogSourceRaid25
		}
	}
	return proto.CatalogSourceKind_CatalogSourceWorldDrop
}

// Primary profession skill lines (SkillLine.dbc).
var skillProfessions = map[int32]proto.Profession{
	164: proto.Profession_Blacksmithing,
	165: proto.Profession_Leatherworking,
	171: proto.Profession_Alchemy,
	182: proto.Profession_Herbalism,
	186: proto.Profession_Mining,
	197: proto.Profession_Tailoring,
	202: proto.Profession_Engineering,
	333: proto.Profession_Enchanting,
	393: proto.Profession_Skinning,
	755: proto.Profession_Jewelcrafting,
	773: proto.Profession_Inscription,
}

// Sim-equippable inventory types: armor, jewelry, weapons, off-hands, ranged and relics. Shirts,
// tabards, bags, ammo and quivers stay out.
var catalogInventoryTypes = map[int32]bool{
	1: true, 2: true, 3: true, 5: true, 6: true, 7: true, 8: true, 9: true, 10: true, 11: true,
	12: true, 13: true, 14: true, 15: true, 16: true, 17: true, 20: true, 21: true, 22: true,
	23: true, 25: true, 26: true, 28: true,
}

const (
	// item_template.spelltrigger_N: the slot holds the spell a recipe teaches
	itemSpellTriggerLearnSpell = 6
	// Spell.dbc SPELL_EFFECT_LEARN_SPELL: teaches EffectTriggerSpell, or with 0 there (spell 483,
	// "Learning"), the spell in the casting item's learn slot
	spellEffectLearnSpell = 36

	itemClassGem           = 3
	itemFlagUniqueEquipped = 0x80000
	itemFlag2HordeOnly     = 0x1
	itemFlag2AllianceOnly  = 0x2
	itemModResilience      = 35
	raceMaskAlliance       = 1 | 4 | 8 | 64 | 1024   // human, dwarf, night elf, gnome, draenei
	raceMaskHorde          = 2 | 16 | 32 | 128 | 512 // orc, undead, tauren, troll, blood elf
	conditionTeamAlliance  = 469
	conditionTeamHorde     = 67
)

func isCatalogItem(row *CatalogItemRow) bool {
	return catalogInventoryTypes[row.InventoryType] || isGem(row)
}

func isGem(row *CatalogItemRow) bool {
	return row.Class == itemClassGem && row.GemProperties != 0
}

// itemFaction reads the faction-only flags, then AllowableRace.
func itemFaction(row *CatalogItemRow) proto.Faction {
	switch {
	case row.FlagsExtra&itemFlag2HordeOnly != 0:
		return proto.Faction_Horde
	case row.FlagsExtra&itemFlag2AllianceOnly != 0:
		return proto.Faction_Alliance
	}
	return raceMaskFaction(row.AllowableRace)
}

func raceMaskFaction(mask int32) proto.Faction {
	playable := mask & (raceMaskAlliance | raceMaskHorde)
	switch {
	case mask <= 0 || playable == 0:
		return proto.Faction_Unknown
	case playable&raceMaskHorde == 0:
		return proto.Faction_Alliance
	case playable&raceMaskAlliance == 0:
		return proto.Faction_Horde
	}
	return proto.Faction_Unknown
}

// Reputation thresholds (ReputationMgr::PointsInRank), for quests that gate on a value.
var reputationRankMins = []int32{-42000, -6000, -3000, 0, 3000, 9000, 21000, 42000}

func reputationRank(value int32) int32 {
	rank := int32(0)
	for i, min := range reputationRankMins {
		if value >= min {
			rank = int32(i)
		}
	}
	return rank
}
