package azerothcore

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// DefaultDSN matches the stock AzerothCore docker-compose setup.
const DefaultDSN = "root:password@tcp(127.0.0.1:3306)/acore_world"

func OpenWorldDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type ItemRow struct {
	Entry          int32
	Name           string
	Class          int32
	Subclass       int32
	Quality        int32
	Flags          uint32
	InventoryType  int32
	AllowableClass int32
	ItemLevel      int32

	StatTypes  [10]int32
	StatValues [10]int32

	ScalingStatDistribution int32
	DmgMin                  float64
	DmgMax                  float64
	Delay                   int32 // ms

	Armor               int32 // total armor, bonus armor included
	ArmorDamageModifier float64
	HolyRes             int32
	FireRes             int32
	NatureRes           int32
	FrostRes            int32
	ShadowRes           int32
	ArcaneRes           int32
	Block               int32

	SpellIDs               [5]int32
	SpellTriggers          [5]int32
	SpellPPMRates          [5]float64
	SpellCooldowns         [5]int32 // ms, -1 when unset
	SpellCategoryCooldowns [5]int32 // ms, -1 when unset

	SocketColors  [3]int32
	SocketBonus   int32
	GemProperties int32
	ItemSet       int32

	RandomProperty int32
	RandomSuffix   int32
}

func LoadItems(db *sql.DB) (map[int32]*ItemRow, error) {
	columns := []string{"entry", "name", "class", "subclass", "Quality", "Flags", "InventoryType", "AllowableClass", "ItemLevel"}
	for i := 1; i <= 10; i++ {
		columns = append(columns, fmt.Sprintf("stat_type%d", i), fmt.Sprintf("stat_value%d", i))
	}
	columns = append(columns, "ScalingStatDistribution", "dmg_min1", "dmg_max1", "delay",
		"armor", "ArmorDamageModifier", "holy_res", "fire_res", "nature_res", "frost_res", "shadow_res", "arcane_res", "block")
	for i := 1; i <= 5; i++ {
		columns = append(columns, fmt.Sprintf("spellid_%d", i), fmt.Sprintf("spelltrigger_%d", i), fmt.Sprintf("spellppmRate_%d", i),
			fmt.Sprintf("spellcooldown_%d", i), fmt.Sprintf("spellcategorycooldown_%d", i))
	}
	columns = append(columns, "socketColor_1", "socketColor_2", "socketColor_3", "socketBonus", "GemProperties", "itemset",
		"RandomProperty", "RandomSuffix")

	rows, err := db.Query("SELECT " + coalesced(columns) + " FROM item_template")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make(map[int32]*ItemRow)
	for rows.Next() {
		item := &ItemRow{}
		dest := []any{&item.Entry, &item.Name, &item.Class, &item.Subclass, &item.Quality, &item.Flags, &item.InventoryType,
			&item.AllowableClass, &item.ItemLevel}
		for i := 0; i < 10; i++ {
			dest = append(dest, &item.StatTypes[i], &item.StatValues[i])
		}
		dest = append(dest, &item.ScalingStatDistribution, &item.DmgMin, &item.DmgMax, &item.Delay,
			&item.Armor, &item.ArmorDamageModifier, &item.HolyRes, &item.FireRes, &item.NatureRes, &item.FrostRes,
			&item.ShadowRes, &item.ArcaneRes, &item.Block)
		for i := 0; i < 5; i++ {
			dest = append(dest, &item.SpellIDs[i], &item.SpellTriggers[i], &item.SpellPPMRates[i],
				&item.SpellCooldowns[i], &item.SpellCategoryCooldowns[i])
		}
		dest = append(dest, &item.SocketColors[0], &item.SocketColors[1], &item.SocketColors[2], &item.SocketBonus,
			&item.GemProperties, &item.ItemSet, &item.RandomProperty, &item.RandomSuffix)

		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		items[item.Entry] = item
	}
	return items, rows.Err()
}

// coalesced guards against NULLs, which custom and module rows sometimes leave in numeric columns.
func coalesced(columns []string) string {
	parts := make([]string, len(columns))
	for i, column := range columns {
		zero := "0"
		switch {
		case column == "name" || strings.HasPrefix(column, "Name_") || strings.HasPrefix(column, "Description_") || strings.HasPrefix(column, "AuraDescription_"):
			zero = "''"
		case strings.HasPrefix(column, "spellcooldown_") || strings.HasPrefix(column, "spellcategorycooldown_"):
			zero = "-1" // unset; 0 would mean no cooldown
		}
		parts[i] = fmt.Sprintf("COALESCE(`%s`, %s)", column, zero)
	}
	return strings.Join(parts, ", ")
}

type SpellProc struct {
	Chance         float64
	ProcsPerMinute float64
	CooldownMs     int32
}

// LoadSpellProcs keys by absolute spell ID; negative IDs in spell_proc apply to every rank.
func LoadSpellProcs(db *sql.DB) (map[int32]SpellProc, error) {
	rows, err := db.Query("SELECT " + coalesced([]string{"SpellId", "Chance", "ProcsPerMinute", "Cooldown"}) + " FROM spell_proc")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	procs := make(map[int32]SpellProc)
	for rows.Next() {
		var id int32
		var proc SpellProc
		if err := rows.Scan(&id, &proc.Chance, &proc.ProcsPerMinute, &proc.CooldownMs); err != nil {
			return nil, err
		}
		if id < 0 {
			id = -id
		}
		procs[id] = proc
	}
	return procs, rows.Err()
}

// ApplySpellDBCOverrides merges acore_world.spell_dbc into the DBC spells. The worldserver lets
// those rows replace client DBC entries with the same ID and adds server-side spells.
func ApplySpellDBCOverrides(db *sql.DB, dbc *DBC) (int, error) {
	columns := []string{"ID", "RecoveryTime", "CategoryRecoveryTime", "ProcChance", "DurationIndex"}
	for _, prefix := range []string{"Effect", "EffectDieSides", "EffectBasePoints", "EffectAura", "EffectItemType", "EffectMiscValue", "EffectTriggerSpell"} {
		for i := 1; i <= 3; i++ {
			columns = append(columns, fmt.Sprintf("%s_%d", prefix, i))
		}
	}
	columns = append(columns, "Name_Lang_enUS", "Description_Lang_enUS", "AuraDescription_Lang_enUS")

	rows, err := db.Query("SELECT " + coalesced(columns) + " FROM spell_dbc")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		spell := &SpellEntry{}
		dest := []any{&spell.ID, &spell.RecoveryTime, &spell.CategoryRecoveryTime, &spell.ProcChance, &spell.DurationIndex}
		for _, arr := range []*[3]int32{&spell.Effect, &spell.EffectDieSides, &spell.EffectBasePoints, &spell.EffectApplyAuraName,
			&spell.EffectItemType, &spell.EffectMiscValue, &spell.EffectTriggerSpell} {
			dest = append(dest, &arr[0], &arr[1], &arr[2])
		}
		dest = append(dest, &spell.Name, &spell.Description, &spell.AuraDescription)

		if err := rows.Scan(dest...); err != nil {
			return 0, err
		}
		dbc.Spells[spell.ID] = overrideSpell(dbc.Spells[spell.ID], spell)
		count++
	}
	return count, rows.Err()
}

// overrideSpell applies a spell_dbc row over the client spell with the same ID, if any. Like
// DBCDatabaseLoader, it keeps the client's text where the row's string column is empty.
func overrideSpell(client, row *SpellEntry) *SpellEntry {
	if client == nil {
		return row
	}
	if row.Name == "" {
		row.Name = client.Name
	}
	if row.Description == "" {
		row.Description = client.Description
	}
	if row.AuraDescription == "" {
		row.AuraDescription = client.AuraDescription
	}
	return row
}

// ApplySpellCooldownOverrides applies acore_world.spell_cooldown_overrides, which the worldserver
// uses to replace spell cooldowns after loading spell_dbc.
func ApplySpellCooldownOverrides(db *sql.DB, dbc *DBC) (int, error) {
	rows, err := db.Query("SELECT " + coalesced([]string{"Id", "RecoveryTime", "CategoryRecoveryTime"}) + " FROM spell_cooldown_overrides")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, recovery, categoryRecovery int32
		if err := rows.Scan(&id, &recovery, &categoryRecovery); err != nil {
			return 0, err
		}
		if spell := dbc.Spells[id]; spell != nil {
			spell.RecoveryTime, spell.CategoryRecoveryTime = recovery, categoryRecovery
			count++
		}
	}
	return count, rows.Err()
}

type EnchantProc struct {
	CustomChance float64
	PPM          float64
}

// LoadSpellEnchantProcs reads spell_enchant_proc_data, which overrides the proc chance of enchant
// combat spells, keyed by enchant ID.
func LoadSpellEnchantProcs(db *sql.DB) (map[int32]EnchantProc, error) {
	rows, err := db.Query("SELECT " + coalesced([]string{"entry", "customChance", "PPMChance"}) + " FROM spell_enchant_proc_data")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	procs := make(map[int32]EnchantProc)
	for rows.Next() {
		var id int32
		var proc EnchantProc
		if err := rows.Scan(&id, &proc.CustomChance, &proc.PPM); err != nil {
			return nil, err
		}
		procs[id] = proc
	}
	return procs, rows.Err()
}

// LoadObtainableItemIDs maps each item a player can get on the server to the kinds of sources
// that provide it: loot tables, vendors, quest and achievement rewards, and create-item spells.
func LoadObtainableItemIDs(db *sql.DB, dbc *DBC) (map[int32][]string, error) {
	type source struct{ kind, query string }
	var queries []source
	for _, table := range []string{"creature", "gameobject", "reference", "item", "mail", "spell", "fishing", "skinning",
		"pickpocketing", "disenchant", "prospecting", "milling"} {
		queries = append(queries, source{table + "_loot", fmt.Sprintf("SELECT Item FROM %s_loot_template WHERE Reference = 0", table)})
	}
	queries = append(queries,
		source{"vendor", "SELECT item FROM npc_vendor WHERE item > 0"},
		source{"event_vendor", "SELECT item FROM game_event_npc_vendor WHERE item > 0"},
		source{"achievement_reward", "SELECT ItemID FROM achievement_reward"},
		source{"quest_reward", `SELECT RewardItem1 FROM quest_template UNION SELECT RewardItem2 FROM quest_template
			UNION SELECT RewardItem3 FROM quest_template UNION SELECT RewardItem4 FROM quest_template
			UNION SELECT RewardChoiceItemID1 FROM quest_template UNION SELECT RewardChoiceItemID2 FROM quest_template
			UNION SELECT RewardChoiceItemID3 FROM quest_template UNION SELECT RewardChoiceItemID4 FROM quest_template
			UNION SELECT RewardChoiceItemID5 FROM quest_template UNION SELECT RewardChoiceItemID6 FROM quest_template`},
	)

	sources := make(map[int32][]string)
	add := func(itemID int32, kind string) {
		if itemID <= 0 {
			return
		}
		for _, existing := range sources[itemID] {
			if existing == kind {
				return
			}
		}
		sources[itemID] = append(sources[itemID], kind)
	}

	for _, src := range queries {
		ids, err := queryIDs(db, src.query)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src.kind, err)
		}
		for _, id := range ids {
			add(id, src.kind)
		}
	}

	for _, spell := range dbc.Spells {
		for i := 0; i < 3; i++ {
			if spell.Effect[i] == SpellEffectCreateItem || spell.Effect[i] == SpellEffectCreateItem2 {
				add(spell.EffectItemType[i], "create_item_spell")
			}
		}
	}
	return sources, nil
}

func queryIDs(db *sql.DB, query string) ([]int32, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int32
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
