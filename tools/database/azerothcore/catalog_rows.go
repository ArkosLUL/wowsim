package azerothcore

import "github.com/wowsims/wotlk/sim/core/proto"

// CatalogRows is everything ResolveCatalog reads: world DB rows and DBC records, already
// joined where the join is trivial. LoadCatalogRows fills it from a live server; tests build it by
// hand.
type CatalogRows struct {
	Items []CatalogItemRow
	// Classic (db.json) phases of items and gems, the flagged fallback when no server source
	// resolves to a tier.
	ClassicPhases map[int32]int32

	Maps             []CatalogMapRow
	MapDifficulties  []MapDifficultyRow
	Creatures        []CatalogCreatureRow
	CreatureSpawns   []SpawnRow
	GameObjects      []CatalogGameObjectRow
	GameObjectSpawns []SpawnRow
	Encounters       []EncounterRow
	Summons          []SummonRow
	Transports       []TransportRow

	Loot       []LootRow
	Conditions []ConditionRow

	Vendors       []VendorRow
	ExtendedCosts []ExtendedCostRow
	TokenTurnIns  []TokenTurnInRow

	Quests        []QuestRow
	QuestStarters []QuestStarterRow

	Spells       []CreateSpellRow
	Achievements []AchievementRewardRow
	Areas        []AreaRow

	LimitCategories []LimitCategoryRow
}

// CatalogItemRow is the part of item_template the catalog needs, plus facts the loader derives
// from the DBCs (HasEffect).
type CatalogItemRow struct {
	Entry         int32
	Name          string
	Class         int32
	Quality       int32
	ItemLevel     int32
	InventoryType int32
	Flags         uint32
	FlagsExtra    uint32
	AllowableRace int32

	MaxCount          int32
	ItemLimitCategory int32
	// Non-zero stat slots, which is how the worldserver fills ItemTemplate::StatsCount.
	StatsCount    int32
	HasResilience bool
	HasEffect     bool
	GemProperties int32

	RequiredSkill             int32
	RequiredSkillRank         int32
	RequiredReputationFaction int32
	RequiredReputationRank    int32

	StartQuest   int32
	DisenchantID int32
	// On-use spells (spelltrigger 0): a create-item or spell-loot spell here makes the item a
	// source of what that spell creates.
	UseSpells []int32
}

type CatalogMapRow struct {
	ID int32
	// Map.dbc map_type: 0 continent, 1 dungeon, 2 raid, 3 battleground, 4 arena.
	Type      int32
	Name      string
	Expansion int32
}

type MapDifficultyRow struct {
	Map        int32
	Difficulty int32
	MaxPlayers int32
}

type CatalogCreatureRow struct {
	Entry             int32
	Name              string
	DifficultyEntries [3]int32
	LootID            int32
	SkinLootID        int32
	PickpocketLootID  int32
	ScriptName        string
}

type CatalogGameObjectRow struct {
	Entry int32
	Type  int32
	Name  string
	// Data1, for chests (type 3) and fishing holes (type 25).
	LootID     int32
	ScriptName string
}

// SpawnRow is a creature or gameobject spawn.
type SpawnRow struct {
	GUID       int32
	Entry      int32
	Map        int32
	SpawnMask  int32
	PhaseMask  uint32
	ScriptName string
}

// TransportRow: a transport's own map and the map it travels in (its TaxiPathNode.dbc route),
// e.g. an ICC gunship.
type TransportRow struct {
	Map      int32
	RouteMap int32
}

// EncounterRow is an instance_encounters creature credit joined to its DungeonEncounter.dbc
// record, which places script-summoned bosses that have no spawn row.
type EncounterRow struct {
	CreditEntry int32
	Map         int32
	Difficulty  int32
}

type SummonerKind int32

const (
	SummonerCreature SummonerKind = iota + 1
	SummonerGameObject
	SummonerMap
)

// SummonRow: a creature or gameobject that the summoner brings into the world, from
// creature_summon_groups or smart_scripts.
type SummonRow struct {
	SummonerKind SummonerKind
	SummonerID   int32
	GameObject   bool
	Entry        int32
}

type LootStore int32

const (
	LootCreature LootStore = iota + 1
	LootGameObject
	LootItem
	LootReference
	LootDisenchant
	LootProspecting
	LootMilling
	LootPickpocketing
	LootSkinning
	LootFishing
	LootMail
	LootSpell
)

// LootRow is a *_loot_template row. For reference rows, Item is whatever the Item column holds;
// conditions attach to it.
type LootRow struct {
	Store     LootStore
	Entry     int32
	Item      int32
	Reference int32
}

type ConditionRow struct {
	SourceType  int32
	SourceGroup int32
	SourceEntry int32
	SourceID    int32
	ElseGroup   int32
	Type        int32
	Value1      int32
	Value2      int32
	Negative    bool
}

// VendorRow is an npc_vendor row, or a game_event_npc_vendor row with its spawn resolved to the
// creature entry.
type VendorRow struct {
	Entry        int32
	Item         int32
	ExtendedCost int32
}

// ExtendedCostRow is an ItemExtendedCost.dbc record, with itemextendedcost_dbc applied.
type ExtendedCostRow struct {
	ID          int32
	HonorPoints int32
	ArenaPoints int32
	ArenaRating int32
	Items       []int32
}

// TokenTurnInRow is a mod-token-turnin conversion: Token (plus Secondary, when set) becomes Result.
type TokenTurnInRow struct {
	Token     int32
	Secondary int32
	Result    int32
}

type QuestRow struct {
	ID    int32
	Title string
	// RewardItem* and RewardChoiceItemID*.
	RewardItems   []int32
	RequiredItems []int32
	// RequiredNpcOrGo*: a positive id is a creature, a negative one a gameobject.
	RequiredNpcOrGo []int32
	// StartItem and ItemDrop*, which the quest hands out itself.
	ProvidedItems []int32

	PrevQuest            int32
	RewardNextQuest      int32
	RewardMailTemplateID int32
	AllowableRaces       int32
	RequiredRepFaction   int32
	RequiredRepValue     int32
}

type QuestStarterKind int32

const (
	QuestStarterCreature QuestStarterKind = iota + 1
	QuestStarterGameObject
	QuestStarterItem
)

type QuestStarterRow struct {
	Quest int32
	Kind  QuestStarterKind
	Entry int32
}

// CreateSpellRow is a spell that creates items, or rolls spell_loot_template (SpellLoot).
type CreateSpellRow struct {
	ID        int32
	Name      string
	Creates   []int32
	SpellLoot bool
	Reagents  []int32
	// Set when SkillLineAbility.dbc lists the spell under a primary profession.
	Profession proto.Profession
}

type AchievementRewardRow struct {
	ID             int32
	Name           string
	Map            int32 // -1 when not tied to a map
	Item           int32
	MailTemplateID int32
}

type AreaRow struct {
	ID  int32
	Map int32
}

type LimitCategoryRow struct {
	ID       int32
	Name     string
	MaxCount int32
	// 0: limits how many you can have; 1: how many you can have equipped.
	Mode int32
}
