package azerothcore

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// Field indices follow AzerothCore's DBCStructure.h and DBCfmt.h.
const (
	limitCategoryFieldID       = 0
	limitCategoryFieldName     = 1
	limitCategoryFieldMaxCount = 18
	limitCategoryFieldMode     = 19

	extendedCostFieldID          = 0
	extendedCostFieldHonorPoints = 1
	extendedCostFieldArenaPoints = 2
	extendedCostFieldItem        = 4
	extendedCostItemCount        = 5
	extendedCostFieldArenaRating = 14

	encounterFieldMap        = 1
	encounterFieldDifficulty = 2

	mapFieldID        = 0
	mapFieldType      = 2
	mapFieldName      = 5
	mapFieldExpansion = 63

	mapDifficultyFieldMap        = 1
	mapDifficultyFieldDifficulty = 2
	mapDifficultyFieldMaxPlayers = 21

	skillAbilityFieldSkillLine = 1
	skillAbilityFieldSpell     = 2

	spellFieldReagent      = 52
	spellFieldReagentCount = 60
	spellReagentSlots      = 8

	achievementFieldID   = 0
	achievementFieldMap  = 2
	achievementFieldName = 4

	areaFieldID  = 0
	areaFieldMap = 1

	taxiNodeFieldPath = 1
	taxiNodeFieldMap  = 3

	// SPELL_EFFECT_CREATE_RANDOM_ITEM rolls spell_loot_template.
	spellEffectCreateRandomItem = 59
)

// catalogDBCFiles are the files LoadCatalogRows reads besides DBCFileNames, with the highest field
// index each is read at.
var catalogDBCFiles = []struct {
	name      string
	lastField int
}{
	{"ItemLimitCategory.dbc", limitCategoryFieldMode},
	{"ItemExtendedCost.dbc", extendedCostFieldArenaRating},
	{"DungeonEncounter.dbc", encounterFieldDifficulty},
	{"Map.dbc", mapFieldExpansion},
	{"MapDifficulty.dbc", mapDifficultyFieldMaxPlayers},
	{"SkillLineAbility.dbc", skillAbilityFieldSpell},
	{"Achievement.dbc", achievementFieldName},
	{"AreaTable.dbc", areaFieldMap},
	{"TaxiPathNode.dbc", taxiNodeFieldMap},
}

// CatalogDBCFileNames lists every DBC LoadCatalogRows needs.
func CatalogDBCFileNames() []string {
	names := append([]string{}, DBCFileNames...)
	for _, f := range catalogDBCFiles {
		names = append(names, f.name)
	}
	return names
}

// catalogDBC holds the DBC records the catalog reads, before the DB's *_dbc overrides.
type catalogDBC struct {
	limitCategories map[int32]LimitCategoryRow
	extendedCosts   map[int32]ExtendedCostRow
	encounters      map[int32]EncounterRow // by DungeonEncounter id; CreditEntry unset
	maps            map[int32]CatalogMapRow
	mapDifficulties map[int32]MapDifficultyRow // by MapDifficulty id
	skillAbilities  map[int32]int32            // spell -> skill line
	spells          map[int32]CreateSpellRow
	achievements    map[int32]AchievementRewardRow // name and map only
	areas           map[int32]int32                // area -> map
	taxiPathMaps    map[int32]int32                // taxi path -> map of its first node
}

func readCatalogDBC(files map[string]*DBCFile) (*catalogDBC, error) {
	for _, f := range catalogDBCFiles {
		if fields := files[f.name].FieldCount; fields <= f.lastField {
			return nil, fmt.Errorf("%s has %d fields, need more than %d", f.name, fields, f.lastField)
		}
	}
	return &catalogDBC{
		limitCategories: readLimitCategories(files["ItemLimitCategory.dbc"]),
		extendedCosts:   readExtendedCosts(files["ItemExtendedCost.dbc"]),
		encounters:      readEncounters(files["DungeonEncounter.dbc"]),
		maps:            readMaps(files["Map.dbc"]),
		mapDifficulties: readMapDifficulties(files["MapDifficulty.dbc"]),
		skillAbilities:  readSkillAbilities(files["SkillLineAbility.dbc"]),
		spells:          readCreateSpells(files["Spell.dbc"]),
		achievements:    readAchievements(files["Achievement.dbc"]),
		areas:           readAreas(files["AreaTable.dbc"]),
		taxiPathMaps:    readTaxiPathMaps(files["TaxiPathNode.dbc"]),
	}, nil
}

func readLimitCategories(f *DBCFile) map[int32]LimitCategoryRow {
	out := make(map[int32]LimitCategoryRow, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		lc := LimitCategoryRow{
			ID:       f.Int32(row, limitCategoryFieldID),
			Name:     f.String(row, limitCategoryFieldName),
			MaxCount: f.Int32(row, limitCategoryFieldMaxCount),
			Mode:     f.Int32(row, limitCategoryFieldMode),
		}
		out[lc.ID] = lc
	}
	return out
}

func readExtendedCosts(f *DBCFile) map[int32]ExtendedCostRow {
	out := make(map[int32]ExtendedCostRow, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		cost := ExtendedCostRow{
			ID:          f.Int32(row, extendedCostFieldID),
			HonorPoints: f.Int32(row, extendedCostFieldHonorPoints),
			ArenaPoints: f.Int32(row, extendedCostFieldArenaPoints),
			ArenaRating: f.Int32(row, extendedCostFieldArenaRating),
		}
		for i := 0; i < extendedCostItemCount; i++ {
			if id := f.Int32(row, extendedCostFieldItem+i); id != 0 {
				cost.Items = append(cost.Items, id)
			}
		}
		out[cost.ID] = cost
	}
	return out
}

func readEncounters(f *DBCFile) map[int32]EncounterRow {
	out := make(map[int32]EncounterRow, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		out[f.Int32(row, 0)] = EncounterRow{Map: f.Int32(row, encounterFieldMap), Difficulty: f.Int32(row, encounterFieldDifficulty)}
	}
	return out
}

func readMaps(f *DBCFile) map[int32]CatalogMapRow {
	out := make(map[int32]CatalogMapRow, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		m := CatalogMapRow{
			ID:        f.Int32(row, mapFieldID),
			Type:      f.Int32(row, mapFieldType),
			Name:      f.String(row, mapFieldName),
			Expansion: f.Int32(row, mapFieldExpansion),
		}
		out[m.ID] = m
	}
	return out
}

func readMapDifficulties(f *DBCFile) map[int32]MapDifficultyRow {
	out := make(map[int32]MapDifficultyRow, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		out[f.Int32(row, 0)] = MapDifficultyRow{
			Map:        f.Int32(row, mapDifficultyFieldMap),
			Difficulty: f.Int32(row, mapDifficultyFieldDifficulty),
			MaxPlayers: f.Int32(row, mapDifficultyFieldMaxPlayers),
		}
	}
	return out
}

// readSkillAbilities keeps primary professions only; a spell under several lines keeps the first.
func readSkillAbilities(f *DBCFile) map[int32]int32 {
	out := map[int32]int32{}
	for row := 0; row < f.RecordCount; row++ {
		skill, spell := f.Int32(row, skillAbilityFieldSkillLine), f.Int32(row, skillAbilityFieldSpell)
		if _, ok := skillProfessions[skill]; !ok {
			continue
		}
		if _, ok := out[spell]; !ok {
			out[spell] = skill
		}
	}
	return out
}

// readCreateSpells keeps spells that create an item or roll spell loot.
func readCreateSpells(f *DBCFile) map[int32]CreateSpellRow {
	out := map[int32]CreateSpellRow{}
	for row := 0; row < f.RecordCount; row++ {
		var effects, itemTypes [3]int32
		for i := 0; i < 3; i++ {
			effects[i] = f.Int32(row, spellFieldEffect+i)
			itemTypes[i] = f.Int32(row, spellFieldEffectItemType+i)
		}
		var reagents, counts [spellReagentSlots]int32
		for i := 0; i < spellReagentSlots; i++ {
			reagents[i] = f.Int32(row, spellFieldReagent+i)
			counts[i] = f.Int32(row, spellFieldReagentCount+i)
		}
		if s, ok := createSpell(f.Int32(row, spellFieldID), f.String(row, spellFieldName), effects, itemTypes, reagents, counts); ok {
			out[s.ID] = s
		}
	}
	return out
}

// createSpell builds a CreateSpellRow from Spell.dbc (or spell_dbc) columns, or reports that the
// spell creates nothing.
func createSpell(id int32, name string, effects, itemTypes [3]int32, reagents, counts [spellReagentSlots]int32) (CreateSpellRow, bool) {
	s := CreateSpellRow{ID: id, Name: name}
	for i := 0; i < 3; i++ {
		switch effects[i] {
		case SpellEffectCreateItem, SpellEffectCreateItem2:
			if itemTypes[i] > 0 {
				s.Creates = append(s.Creates, itemTypes[i])
			} else {
				s.SpellLoot = true
			}
		case spellEffectCreateRandomItem:
			s.SpellLoot = true
		}
	}
	if len(s.Creates) == 0 && !s.SpellLoot {
		return s, false
	}
	for i := 0; i < spellReagentSlots; i++ {
		if reagents[i] > 0 && counts[i] > 0 {
			s.Reagents = append(s.Reagents, reagents[i])
		}
	}
	return s, true
}

func readAchievements(f *DBCFile) map[int32]AchievementRewardRow {
	out := make(map[int32]AchievementRewardRow, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		a := AchievementRewardRow{
			ID:   f.Int32(row, achievementFieldID),
			Map:  f.Int32(row, achievementFieldMap),
			Name: f.String(row, achievementFieldName),
		}
		out[a.ID] = a
	}
	return out
}

func readAreas(f *DBCFile) map[int32]int32 {
	out := make(map[int32]int32, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		out[f.Int32(row, areaFieldID)] = f.Int32(row, areaFieldMap)
	}
	return out
}

// readTaxiPathMaps keeps the map of each path's first node; transports never change maps.
func readTaxiPathMaps(f *DBCFile) map[int32]int32 {
	out := map[int32]int32{}
	for row := 0; row < f.RecordCount; row++ {
		path := f.Int32(row, taxiNodeFieldPath)
		if _, ok := out[path]; !ok {
			out[path] = f.Int32(row, taxiNodeFieldMap)
		}
	}
	return out
}

// createSpellProfession looks a spell up in the SkillLineAbility map.
func createSpellProfession(skillAbilities map[int32]int32, spell int32) proto.Profession {
	return skillProfessions[skillAbilities[spell]]
}
