package azerothcore

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// LoadCatalogRows reads everything ResolveCatalog needs from the world DB (SELECTs only) and the
// server's DBC files in dbcDir, applying the DB's *_dbc overrides. ClassicPhases is left for the
// caller.
func LoadCatalogRows(db *sql.DB, dbcDir string) (*CatalogRows, error) {
	files, err := readDBCFiles(dbcDir, CatalogDBCFileNames())
	if err != nil {
		return nil, err
	}
	dbc := dbcFromFiles(files)
	if _, err := ApplySpellDBCOverrides(db, dbc); err != nil {
		return nil, fmt.Errorf("spell_dbc: %w", err)
	}
	cdbc, err := readCatalogDBC(files)
	if err != nil {
		return nil, err
	}
	if err := applyCatalogDBCOverrides(db, cdbc); err != nil {
		return nil, err
	}

	rows := &CatalogRows{}
	steps := []struct {
		name string
		load func() error
	}{
		{"items", func() error { return loadCatalogItems(db, dbc, rows) }},
		{"creatures", func() error { return loadCreatures(db, rows) }},
		{"gameobjects", func() error { return loadGameObjects(db, cdbc, rows) }},
		{"encounters", func() error { return loadEncounters(db, cdbc, rows) }},
		{"summons", func() error { return loadSummons(db, rows) }},
		{"loot", func() error { return loadLoot(db, rows) }},
		{"conditions", func() error { return loadConditions(db, rows) }},
		{"vendors", func() error { return loadVendors(db, rows) }},
		{"token turn-ins", func() error { return loadTokenTurnIns(db, rows) }},
		{"quests", func() error { return loadQuests(db, rows) }},
		{"trainer spells", func() error { return loadTrainerSpells(db, dbc, rows) }},
		{"achievements", func() error { return loadAchievements(db, cdbc, rows) }},
	}
	for _, step := range steps {
		if err := step.load(); err != nil {
			return nil, fmt.Errorf("%s: %w", step.name, err)
		}
	}

	for _, id := range sortedKeys(cdbc.maps) {
		rows.Maps = append(rows.Maps, cdbc.maps[id])
	}
	for _, id := range sortedKeys(cdbc.mapDifficulties) {
		rows.MapDifficulties = append(rows.MapDifficulties, cdbc.mapDifficulties[id])
	}
	for _, id := range sortedKeys(cdbc.extendedCosts) {
		rows.ExtendedCosts = append(rows.ExtendedCosts, cdbc.extendedCosts[id])
	}
	for _, id := range sortedKeys(cdbc.limitCategories) {
		rows.LimitCategories = append(rows.LimitCategories, cdbc.limitCategories[id])
	}
	for _, id := range sortedKeys(cdbc.areas) {
		rows.Areas = append(rows.Areas, AreaRow{ID: id, Map: cdbc.areas[id]})
	}
	for _, id := range sortedKeys(cdbc.spells) {
		s := cdbc.spells[id]
		s.Profession = createSpellProfession(cdbc.skillAbilities, id)
		rows.Spells = append(rows.Spells, s)
	}
	return rows, nil
}

func scanRows(db *sql.DB, query string, scan func(*sql.Rows) error) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// missingTable reports MySQL error 1146, which a module that isn't installed raises.
func missingTable(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1146
}

func columnList(prefix string, from, to int) []string {
	var cols []string
	for i := from; i <= to; i++ {
		cols = append(cols, fmt.Sprintf("%s%d", prefix, i))
	}
	return cols
}

// String columns the catalog reads; everything else is numeric.
var catalogStringColumns = map[string]bool{
	"name": true, "ScriptName": true, "Name_Lang_enUS": true, "MapName_Lang_enUS": true, "Title_Lang_enUS": true,
}

// selectFrom reads columns with NULLs as zero values, since module rows leave some NULL.
func selectFrom(table string, columns ...string) string {
	parts := make([]string, len(columns))
	for i, column := range columns {
		zero := "0"
		if catalogStringColumns[column] {
			zero = "''"
		}
		parts[i] = fmt.Sprintf("COALESCE(`%s`, %s)", column, zero)
	}
	return "SELECT " + strings.Join(parts, ", ") + " FROM " + table
}

// applyCatalogDBCOverrides applies the acore_world *_dbc tables, which replace same-id DBC records.
func applyCatalogDBCOverrides(db *sql.DB, c *catalogDBC) error {
	overrides := []struct {
		name  string
		query string
		scan  func(*sql.Rows) error
	}{
		{"itemlimitcategory_dbc", selectFrom("itemlimitcategory_dbc", "ID", "Name_Lang_enUS", "Quantity", "Flags"), func(r *sql.Rows) error {
			var lc LimitCategoryRow
			err := r.Scan(&lc.ID, &lc.Name, &lc.MaxCount, &lc.Mode)
			c.limitCategories[lc.ID] = lc
			return err
		}},
		{"itemextendedcost_dbc", selectFrom("itemextendedcost_dbc", append([]string{"ID", "HonorPoints", "ArenaPoints", "RequiredArenaRating"},
			columnList("ItemID_", 1, extendedCostItemCount)...)...), func(r *sql.Rows) error {
			var cost ExtendedCostRow
			var items [extendedCostItemCount]int32
			err := r.Scan(&cost.ID, &cost.HonorPoints, &cost.ArenaPoints, &cost.ArenaRating, &items[0], &items[1], &items[2], &items[3], &items[4])
			for _, id := range items {
				if id != 0 {
					cost.Items = append(cost.Items, id)
				}
			}
			c.extendedCosts[cost.ID] = cost
			return err
		}},
		{"dungeonencounter_dbc", selectFrom("dungeonencounter_dbc", "ID", "MapID", "Difficulty"), func(r *sql.Rows) error {
			var id int32
			var e EncounterRow
			err := r.Scan(&id, &e.Map, &e.Difficulty)
			c.encounters[id] = e
			return err
		}},
		{"map_dbc", selectFrom("map_dbc", "ID", "InstanceType", "MapName_Lang_enUS", "ExpansionID"), func(r *sql.Rows) error {
			var m CatalogMapRow
			err := r.Scan(&m.ID, &m.Type, &m.Name, &m.Expansion)
			c.maps[m.ID] = m
			return err
		}},
		{"mapdifficulty_dbc", selectFrom("mapdifficulty_dbc", "ID", "MapID", "Difficulty", "MaxPlayers"), func(r *sql.Rows) error {
			var id int32
			var d MapDifficultyRow
			err := r.Scan(&id, &d.Map, &d.Difficulty, &d.MaxPlayers)
			c.mapDifficulties[id] = d
			return err
		}},
		{"skilllineability_dbc", selectFrom("skilllineability_dbc", "SkillLine", "Spell"), func(r *sql.Rows) error {
			var skill, spell int32
			err := r.Scan(&skill, &spell)
			if _, ok := skillProfessions[skill]; ok {
				c.skillAbilities[spell] = skill
			}
			return err
		}},
		{"achievement_dbc", selectFrom("achievement_dbc", "ID", "Instance_Id", "Title_Lang_enUS"), func(r *sql.Rows) error {
			var a AchievementRewardRow
			err := r.Scan(&a.ID, &a.Map, &a.Name)
			c.achievements[a.ID] = a
			return err
		}},
		{"areatable_dbc", selectFrom("areatable_dbc", "ID", "ContinentID"), func(r *sql.Rows) error {
			var id, mapID int32
			err := r.Scan(&id, &mapID)
			c.areas[id] = mapID
			return err
		}},
		{"spell_dbc", selectFrom("spell_dbc", append(append(append(append([]string{"ID", "Name_Lang_enUS"},
			columnList("Effect_", 1, 3)...), columnList("EffectItemType_", 1, 3)...),
			columnList("Reagent_", 1, spellReagentSlots)...), columnList("ReagentCount_", 1, spellReagentSlots)...)...), func(r *sql.Rows) error {
			var id int32
			var name string
			var effects, itemTypes [3]int32
			var reagents, counts [spellReagentSlots]int32
			dest := []any{&id, &name}
			for i := range effects {
				dest = append(dest, &effects[i])
			}
			for i := range itemTypes {
				dest = append(dest, &itemTypes[i])
			}
			for i := range reagents {
				dest = append(dest, &reagents[i])
			}
			for i := range counts {
				dest = append(dest, &counts[i])
			}
			if err := r.Scan(dest...); err != nil {
				return err
			}
			if name == "" {
				name = c.spells[id].Name
			}
			if s, ok := createSpell(id, name, effects, itemTypes, reagents, counts); ok {
				c.spells[id] = s
			} else {
				delete(c.spells, id)
			}
			return nil
		}},
	}
	for _, o := range overrides {
		if err := scanRows(db, o.query, o.scan); err != nil {
			if missingTable(err) {
				log.Printf("warning: %s is missing, using the DBC as is", o.name)
				continue
			}
			return fmt.Errorf("%s: %w", o.name, err)
		}
	}
	return nil
}

type itemExtras struct {
	flagsExtra                             uint32
	allowableRace, maxCount, limitCategory int32
	requiredSkill, requiredSkillRank       int32
	repFaction, repRank                    int32
	startQuest, disenchantID               int32
}

func loadCatalogItems(db *sql.DB, dbc *DBC, rows *CatalogRows) error {
	items, err := LoadItems(db)
	if err != nil {
		return err
	}
	extras := map[int32]itemExtras{}
	query := selectFrom("item_template", "entry", "FlagsExtra", "AllowableRace", "maxcount", "ItemLimitCategory", "RequiredSkill",
		"RequiredSkillRank", "RequiredReputationFaction", "RequiredReputationRank", "startquest", "DisenchantID")
	err = scanRows(db, query, func(r *sql.Rows) error {
		var id int32
		var e itemExtras
		err := r.Scan(&id, &e.flagsExtra, &e.allowableRace, &e.maxCount, &e.limitCategory, &e.requiredSkill, &e.requiredSkillRank,
			&e.repFaction, &e.repRank, &e.startQuest, &e.disenchantID)
		extras[id] = e
		return err
	})
	if err != nil {
		return err
	}

	for _, id := range sortedKeys(items) {
		item, e := items[id], extras[id]
		row := CatalogItemRow{
			Entry:                     id,
			Name:                      item.Name,
			Class:                     item.Class,
			Quality:                   item.Quality,
			ItemLevel:                 item.ItemLevel,
			InventoryType:             item.InventoryType,
			Flags:                     item.Flags,
			FlagsExtra:                e.flagsExtra,
			AllowableRace:             e.allowableRace,
			MaxCount:                  e.maxCount,
			ItemLimitCategory:         e.limitCategory,
			GemProperties:             item.GemProperties,
			RequiredSkill:             e.requiredSkill,
			RequiredSkillRank:         e.requiredSkillRank,
			RequiredReputationFaction: e.repFaction,
			RequiredReputationRank:    e.repRank,
			StartQuest:                e.startQuest,
			DisenchantID:              e.disenchantID,
		}
		for i, value := range item.StatValues {
			if value == 0 {
				continue
			}
			row.StatsCount++
			if item.StatTypes[i] == itemModResilience {
				row.HasResilience = true
			}
		}
		for i, spell := range item.SpellIDs {
			switch {
			case spell <= 0:
			case item.SpellTriggers[i] == itemSpellTriggerLearnSpell:
				row.Teaches = append(row.Teaches, spell)
			case item.SpellTriggers[i] == ItemSpellTriggerOnUse:
				row.UseSpells = append(row.UseSpells, spell)
				row.Teaches = append(row.Teaches, learnedSpells(dbc, spell)...)
			}
		}
		if isGem(&row) {
			_, spells := ConvertGem(item, dbc)
			row.HasEffect = len(spells) > 0
		} else if isCatalogItem(&row) {
			row.HasEffect = len(ConvertItem(item, dbc).EffectSpells) > 0
		}
		rows.Items = append(rows.Items, row)
		if row.StartQuest != 0 {
			rows.QuestStarters = append(rows.QuestStarters, QuestStarterRow{Quest: row.StartQuest, Kind: QuestStarterItem, Entry: id})
		}
	}
	return nil
}

// learnedSpells is what a spell's learn effects teach. 483 ("Learning") names none: the item that
// casts it carries the spell in a learn slot.
func learnedSpells(dbc *DBC, spellID int32) []int32 {
	spell := dbc.Spells[spellID]
	if spell == nil {
		return nil
	}
	var out []int32
	for i, effect := range spell.Effect {
		if effect == spellEffectLearnSpell && spell.EffectTriggerSpell[i] > 0 {
			out = append(out, spell.EffectTriggerSpell[i])
		}
	}
	return out
}

// loadTrainerSpells follows Trainer::TeachSpell: a trainer spell with a learn effect is cast, which
// teaches what the effect names; any other is learned as is.
func loadTrainerSpells(db *sql.DB, dbc *DBC, rows *CatalogRows) error {
	taught := map[int32]bool{}
	err := scanRows(db, selectFrom("trainer_spell", "SpellId"), func(r *sql.Rows) error {
		var spell int32
		if err := r.Scan(&spell); err != nil {
			return err
		}
		learned := learnedSpells(dbc, spell)
		if len(learned) == 0 {
			learned = []int32{spell}
		}
		for _, id := range learned {
			taught[id] = true
		}
		return nil
	})
	if missingTable(err) {
		log.Printf("warning: trainer_spell is missing, so every recipe gates its spell")
		return nil
	}
	if err != nil {
		return err
	}
	rows.TrainerSpells = sortedKeys(taught)
	return nil
}

func loadCreatures(db *sql.DB, rows *CatalogRows) error {
	// ordered, so a difficulty entry two templates share always goes to the same one
	err := scanRows(db, selectFrom("creature_template", "entry", "name", "difficulty_entry_1", "difficulty_entry_2", "difficulty_entry_3",
		"lootid", "skinloot", "pickpocketloot", "ScriptName")+" ORDER BY entry", func(r *sql.Rows) error {
		var c CatalogCreatureRow
		err := r.Scan(&c.Entry, &c.Name, &c.DifficultyEntries[0], &c.DifficultyEntries[1], &c.DifficultyEntries[2],
			&c.LootID, &c.SkinLootID, &c.PickpocketLootID, &c.ScriptName)
		rows.Creatures = append(rows.Creatures, c)
		return err
	})
	if err != nil {
		return err
	}
	return scanRows(db, selectFrom("creature", "guid", "id", "map", "spawnMask", "phaseMask", "ScriptName"), func(r *sql.Rows) error {
		var s SpawnRow
		err := r.Scan(&s.GUID, &s.Entry, &s.Map, &s.SpawnMask, &s.PhaseMask, &s.ScriptName)
		rows.CreatureSpawns = append(rows.CreatureSpawns, s)
		return err
	})
}

// GAMEOBJECT_TYPE_MO_TRANSPORT: Data0 is its TaxiPath, Data6 its own map.
const gameObjectTypeTransport = 15

func loadGameObjects(db *sql.DB, c *catalogDBC, rows *CatalogRows) error {
	err := scanRows(db, selectFrom("gameobject_template", "entry", "type", "name", "Data0", "Data1", "Data6", "ScriptName")+" ORDER BY entry", func(r *sql.Rows) error {
		var g CatalogGameObjectRow
		var path, transportMap int32
		err := r.Scan(&g.Entry, &g.Type, &g.Name, &path, &g.LootID, &transportMap, &g.ScriptName)
		if route, ok := c.taxiPathMaps[path]; ok && g.Type == gameObjectTypeTransport && transportMap != 0 {
			rows.Transports = append(rows.Transports, TransportRow{Map: transportMap, RouteMap: route})
		}
		rows.GameObjects = append(rows.GameObjects, g)
		return err
	})
	if err != nil {
		return err
	}
	return scanRows(db, selectFrom("gameobject", "guid", "id", "map", "spawnMask", "phaseMask", "ScriptName"), func(r *sql.Rows) error {
		var s SpawnRow
		err := r.Scan(&s.GUID, &s.Entry, &s.Map, &s.SpawnMask, &s.PhaseMask, &s.ScriptName)
		rows.GameObjectSpawns = append(rows.GameObjectSpawns, s)
		return err
	})
}

// loadEncounters joins creature credits (creditType 0) to DungeonEncounter.dbc.
func loadEncounters(db *sql.DB, c *catalogDBC, rows *CatalogRows) error {
	return scanRows(db, selectFrom("instance_encounters", "entry", "creditType", "creditEntry"), func(r *sql.Rows) error {
		var id, creditType, credit int32
		if err := r.Scan(&id, &creditType, &credit); err != nil {
			return err
		}
		if e, ok := c.encounters[id]; ok && creditType == 0 && credit != 0 {
			e.CreditEntry = credit
			rows.Encounters = append(rows.Encounters, e)
		}
		return nil
	})
}

// SmartAI action types that summon, or call an action list that might.
const (
	smartActionSummonCreature        = 12
	smartActionSummonGameObject      = 50
	smartActionCallTimedActionList   = 80
	smartActionCallRandomActionList  = 87
	smartActionCallRandomRangeAction = 88

	smartSourceCreature   = 0
	smartSourceGameObject = 1
	smartSourceActionList = 9
)

func loadSummons(db *sql.DB, rows *CatalogRows) error {
	err := scanRows(db, selectFrom("creature_summon_groups", "summonerId", "summonerType", "entry"), func(r *sql.Rows) error {
		var summoner, summonerType, entry int32
		if err := r.Scan(&summoner, &summonerType, &entry); err != nil {
			return err
		}
		// TempSummonType's SUMMONER_TYPE_CREATURE/GAMEOBJECT/MAP are 0/1/2
		if summonerType >= 0 && summonerType <= 2 {
			rows.Summons = append(rows.Summons, SummonRow{SummonerKind: SummonerKind(summonerType + 1), SummonerID: summoner, Entry: entry})
		}
		return nil
	})
	if err != nil {
		return err
	}

	creatureGUIDs, goGUIDs := map[int32]int32{}, map[int32]int32{}
	for _, s := range rows.CreatureSpawns {
		creatureGUIDs[s.GUID] = s.Entry
	}
	for _, s := range rows.GameObjectSpawns {
		goGUIDs[s.GUID] = s.Entry
	}

	type smartRow struct {
		owner      int32
		sourceType int32
		action     int32
		params     [6]int32
	}
	var smart []smartRow
	query := selectFrom("smart_scripts", append([]string{"entryorguid", "source_type", "action_type"}, columnList("action_param", 1, 6)...)...) +
		fmt.Sprintf(" WHERE action_type IN (%d, %d, %d, %d, %d)", smartActionSummonCreature, smartActionSummonGameObject,
			smartActionCallTimedActionList, smartActionCallRandomActionList, smartActionCallRandomRangeAction)
	err = scanRows(db, query, func(r *sql.Rows) error {
		var s smartRow
		err := r.Scan(&s.owner, &s.sourceType, &s.action, &s.params[0], &s.params[1], &s.params[2], &s.params[3], &s.params[4], &s.params[5])
		smart = append(smart, s)
		return err
	})
	if err != nil {
		return err
	}

	// who runs a script: a negative entryorguid is a spawn guid, and action lists run for whoever
	// called them (or, by convention, creature entry = list id / 100)
	type summoner struct {
		kind  SummonerKind
		entry int32
	}
	resolveOwner := func(sourceType, owner int32) (summoner, bool) {
		switch sourceType {
		case smartSourceCreature:
			if owner < 0 {
				owner = creatureGUIDs[-owner]
			}
			return summoner{SummonerCreature, owner}, owner != 0
		case smartSourceGameObject:
			if owner < 0 {
				owner = goGUIDs[-owner]
			}
			return summoner{SummonerGameObject, owner}, owner != 0
		}
		return summoner{}, false
	}
	listCallers := map[int32][]summoner{}
	for _, s := range smart {
		caller, ok := resolveOwner(s.sourceType, s.owner)
		if !ok {
			continue
		}
		var lists []int32
		switch s.action {
		case smartActionCallTimedActionList:
			lists = []int32{s.params[0]}
		case smartActionCallRandomActionList:
			lists = s.params[:]
		case smartActionCallRandomRangeAction:
			for id := s.params[0]; id <= s.params[1] && id-s.params[0] < 100; id++ {
				lists = append(lists, id)
			}
		}
		for _, id := range lists {
			if id > 0 {
				listCallers[id] = append(listCallers[id], caller)
			}
		}
	}

	for _, s := range smart {
		if s.action != smartActionSummonCreature && s.action != smartActionSummonGameObject || s.params[0] <= 0 {
			continue
		}
		var owners []summoner
		if s.sourceType == smartSourceActionList {
			owners = listCallers[s.owner]
			if len(owners) == 0 {
				owners = []summoner{{SummonerCreature, s.owner / 100}}
			}
		} else if o, ok := resolveOwner(s.sourceType, s.owner); ok {
			owners = []summoner{o}
		}
		for _, o := range owners {
			rows.Summons = append(rows.Summons, SummonRow{SummonerKind: o.kind, SummonerID: o.entry,
				GameObject: s.action == smartActionSummonGameObject, Entry: s.params[0]})
		}
	}
	return nil
}

var lootTables = []struct {
	store LootStore
	table string
}{
	{LootCreature, "creature_loot_template"},
	{LootGameObject, "gameobject_loot_template"},
	{LootItem, "item_loot_template"},
	{LootReference, "reference_loot_template"},
	{LootDisenchant, "disenchant_loot_template"},
	{LootProspecting, "prospecting_loot_template"},
	{LootMilling, "milling_loot_template"},
	{LootPickpocketing, "pickpocketing_loot_template"},
	{LootSkinning, "skinning_loot_template"},
	{LootFishing, "fishing_loot_template"},
	{LootMail, "mail_loot_template"},
	{LootSpell, "spell_loot_template"},
}

func loadLoot(db *sql.DB, rows *CatalogRows) error {
	for _, t := range lootTables {
		err := scanRows(db, selectFrom(t.table, "Entry", "Item", "Reference"), func(r *sql.Rows) error {
			l := LootRow{Store: t.store}
			err := r.Scan(&l.Entry, &l.Item, &l.Reference)
			rows.Loot = append(rows.Loot, l)
			return err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", t.table, err)
		}
	}
	return nil
}

// loadConditions reads every condition on the sources the catalog looks at, whatever its type:
// dropping rows would drop else groups that pass.
func loadConditions(db *sql.DB, rows *CatalogRows) error {
	query := selectFrom("conditions", "SourceTypeOrReferenceId", "SourceGroup", "SourceEntry", "SourceId", "ElseGroup",
		"ConditionTypeOrReference", "ConditionValue1", "ConditionValue2", "NegativeCondition") +
		fmt.Sprintf(" WHERE SourceTypeOrReferenceId BETWEEN 1 AND 12 OR SourceTypeOrReferenceId IN (%d, %d)",
			conditionSourceQuestAvailable, conditionSourceVendor)
	return scanRows(db, query, func(r *sql.Rows) error {
		var c ConditionRow
		var negative int32
		err := r.Scan(&c.SourceType, &c.SourceGroup, &c.SourceEntry, &c.SourceID, &c.ElseGroup, &c.Type, &c.Value1, &c.Value2, &negative)
		c.Negative = negative != 0
		rows.Conditions = append(rows.Conditions, c)
		return err
	})
}

// loadVendors expands reference vendors (a negative item includes another vendor's list) and
// resolves event vendors' spawns to their creature.
func loadVendors(db *sql.DB, rows *CatalogRows) error {
	lists := map[int32][]VendorRow{}
	err := scanRows(db, selectFrom("npc_vendor", "entry", "item", "ExtendedCost"), func(r *sql.Rows) error {
		var v VendorRow
		err := r.Scan(&v.Entry, &v.Item, &v.ExtendedCost)
		lists[v.Entry] = append(lists[v.Entry], v)
		return err
	})
	if err != nil {
		return err
	}
	var expand func(vendor, list int32, seen map[int32]bool)
	expand = func(vendor, list int32, seen map[int32]bool) {
		if seen[list] {
			return
		}
		seen[list] = true
		for _, v := range lists[list] {
			if v.Item < 0 {
				expand(vendor, -v.Item, seen)
				continue
			}
			rows.Vendors = append(rows.Vendors, VendorRow{Entry: vendor, Item: v.Item, ExtendedCost: v.ExtendedCost})
		}
	}
	for _, vendor := range sortedKeys(lists) {
		expand(vendor, vendor, map[int32]bool{})
	}

	spawns := map[int32]int32{}
	for _, s := range rows.CreatureSpawns {
		spawns[s.GUID] = s.Entry
	}
	return scanRows(db, selectFrom("game_event_npc_vendor", "guid", "item", "ExtendedCost"), func(r *sql.Rows) error {
		var guid int32
		var v VendorRow
		if err := r.Scan(&guid, &v.Item, &v.ExtendedCost); err != nil {
			return err
		}
		if v.Entry = spawns[guid]; v.Entry != 0 && v.Item > 0 {
			rows.Vendors = append(rows.Vendors, v)
		}
		return nil
	})
}

func loadTokenTurnIns(db *sql.DB, rows *CatalogRows) error {
	err := scanRows(db, selectFrom("mod_token_turnin_tokens", "token_entry", "secondary_item_entry", "result_item_entry"), func(r *sql.Rows) error {
		var t TokenTurnInRow
		err := r.Scan(&t.Token, &t.Secondary, &t.Result)
		rows.TokenTurnIns = append(rows.TokenTurnIns, t)
		return err
	})
	if missingTable(err) {
		log.Printf("warning: mod_token_turnin_tokens is missing, so no token turn-ins")
		return nil
	}
	return err
}

func loadQuests(db *sql.DB, rows *CatalogRows) error {
	columns := []string{"q.ID", "q.LogTitle", "q.RewardNextQuest", "q.AllowableRaces", "q.StartItem",
		"a.PrevQuestID", "a.RewardMailTemplateID", "a.RequiredMinRepFaction", "a.RequiredMinRepValue"}
	columns = append(columns, columnList("q.RewardItem", 1, 4)...)
	columns = append(columns, columnList("q.RewardChoiceItemID", 1, 6)...)
	columns = append(columns, columnList("q.RequiredItemId", 1, 6)...)
	columns = append(columns, columnList("q.RequiredNpcOrGo", 1, 4)...)
	columns = append(columns, columnList("q.ItemDrop", 1, 4)...)
	quoted := make([]string, len(columns))
	for i, c := range columns {
		table, name, _ := strings.Cut(c, ".")
		zero := "0"
		if name == "LogTitle" {
			zero = "''"
		}
		quoted[i] = fmt.Sprintf("COALESCE(%s.`%s`, %s)", table, name, zero)
	}
	query := "SELECT " + strings.Join(quoted, ", ") + " FROM quest_template q LEFT JOIN quest_template_addon a ON a.ID = q.ID"
	err := scanRows(db, query, func(r *sql.Rows) error {
		var q QuestRow
		var startItem int32
		var rewards [10]int32
		var required [6]int32
		var targets [4]int32
		var drops [4]int32
		dest := []any{&q.ID, &q.Title, &q.RewardNextQuest, &q.AllowableRaces, &startItem,
			&q.PrevQuest, &q.RewardMailTemplateID, &q.RequiredRepFaction, &q.RequiredRepValue}
		for i := range rewards {
			dest = append(dest, &rewards[i])
		}
		for i := range required {
			dest = append(dest, &required[i])
		}
		for i := range targets {
			dest = append(dest, &targets[i])
		}
		for i := range drops {
			dest = append(dest, &drops[i])
		}
		if err := r.Scan(dest...); err != nil {
			return err
		}
		q.RewardItems = nonZero(rewards[:])
		q.RequiredItems = nonZero(required[:])
		q.RequiredNpcOrGo = nonZero(targets[:])
		q.ProvidedItems = nonZero(append([]int32{startItem}, drops[:]...))
		rows.Quests = append(rows.Quests, q)
		return nil
	})
	if err != nil {
		return err
	}
	for _, s := range []struct {
		table string
		kind  QuestStarterKind
	}{
		{"creature_queststarter", QuestStarterCreature},
		{"gameobject_queststarter", QuestStarterGameObject},
	} {
		err := scanRows(db, selectFrom(s.table, "id", "quest"), func(r *sql.Rows) error {
			q := QuestStarterRow{Kind: s.kind}
			err := r.Scan(&q.Entry, &q.Quest)
			rows.QuestStarters = append(rows.QuestStarters, q)
			return err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", s.table, err)
		}
	}
	return nil
}

func nonZero(ids []int32) []int32 {
	var out []int32
	for _, id := range ids {
		if id != 0 {
			out = append(out, id)
		}
	}
	return out
}

func loadAchievements(db *sql.DB, c *catalogDBC, rows *CatalogRows) error {
	return scanRows(db, selectFrom("achievement_reward", "ID", "ItemID", "MailTemplateID"), func(r *sql.Rows) error {
		var a AchievementRewardRow
		if err := r.Scan(&a.ID, &a.Item, &a.MailTemplateID); err != nil {
			return err
		}
		a.Map = -1
		if dbcRow, ok := c.achievements[a.ID]; ok {
			a.Map, a.Name = dbcRow.Map, dbcRow.Name
		}
		rows.Achievements = append(rows.Achievements, a)
		return nil
	})
}
